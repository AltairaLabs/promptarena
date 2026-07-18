package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"log"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	arenaaudio "github.com/AltairaLabs/promptarena/arena/audio"
	"github.com/AltairaLabs/promptarena/arena/engine"
	"github.com/AltairaLabs/promptarena/arena/statestore"
	"github.com/AltairaLabs/promptarena/arena/web"
)

const (
	serverReadTimeout = 30 * time.Second
	defaultServePort  = 8080
	// servePortScanAttempts bounds how many consecutive ports we try before
	// giving up when the requested one is busy.
	servePortScanAttempts = 20
)

// serveAddr returns the IPv4 loopback listen address for the given port.
func serveAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

// firstFreeLoopbackPort finds the first port >= start (scanning up to `attempts`
// consecutive ports) that is bindable on BOTH the IPv4 (127.0.0.1) and IPv6
// (::1) loopback, returning an open listener for each. If the host has no IPv6
// loopback at all, it falls back to IPv4-only (v6 == nil).
//
// Checking both families is the fix for a silent failure: binding only IPv4 lets
// a process holding the port on the IPv6 loopback (e.g. another app on
// [::]:8080) slip through — our bind succeeds, but a browser resolving
// "localhost" to ::1 connects to that other process instead of us. By refusing
// any port occupied on either family and serving on both, "localhost" always
// reaches us. The caller owns closing the returned listeners.
func firstFreeLoopbackPort(start, attempts int) (v4, v6 net.Listener, port int, err error) {
	ipv6OK := false
	if probe, probeErr := net.Listen("tcp6", "[::1]:0"); probeErr == nil {
		ipv6OK = true
		_ = probe.Close()
	}
	var lastErr error
	for p := start; p < start+attempts; p++ {
		l4, e4 := net.Listen("tcp4", serveAddr(p))
		if e4 != nil {
			lastErr = e4
			continue
		}
		if !ipv6OK {
			return l4, nil, p, nil
		}
		l6, e6 := net.Listen("tcp6", fmt.Sprintf("[::1]:%d", p))
		if e6 != nil {
			_ = l4.Close()
			lastErr = e6
			continue
		}
		return l4, l6, p, nil
	}
	return nil, nil, 0, fmt.Errorf("no free port in range %d-%d: %w", start, start+attempts-1, lastErr)
}

var serveCmd = &cobra.Command{
	Use:   "serve [config-path]",
	Short: "Start the Arena web UI with live run streaming",
	Long: `Starts a local web server with the Arena UI. Streams run events
to the browser via SSE and provides a REST API for starting runs
and viewing results.

If config-path is a directory, looks for config.arena.yaml inside it.

Examples:
  promptarena serve                    # Load config.arena.yaml from current dir
  promptarena serve ./my-scenario      # Load from specific directory
  promptarena serve -p 3000            # Serve on port 3000
  promptarena serve --open             # Open browser automatically`,
	Args: cobra.MaximumNArgs(1),
	RunE: runServe,
}

var (
	servePort int
	serveOpen bool
)

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", defaultServePort, "Port to serve on")
	serveCmd.Flags().BoolVarP(&serveOpen, "open", "o", false, "Open browser automatically")
	serveCmd.Flags().String("audio-monitor", string(arenaaudio.ModeAuto),
		"Audio monitoring: auto, on, off")
	serveCmd.Flags().Int("audio-rate", arenaaudio.Rate24k,
		"Audio canonical sample rate: 16000, 24000, or 48000")
	serveCmd.Flags().Bool("mock-provider", false,
		"Replace ALL configured providers with mocks (assistant, self-play user, "+
			"and any others) so no real API calls are made. Useful for free demos. "+
			"Pair with --mock-config to use canned scenario responses.")
	serveCmd.Flags().String("mock-config", "",
		"Path to a mock responses YAML file. Only meaningful with --mock-provider.")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	configPath := "."
	if len(args) > 0 {
		configPath = args[0]
	}

	// Resolve config file path
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
		absPath = filepath.Join(absPath, "config.arena.yaml")
	}
	if _, statErr := os.Stat(absPath); statErr != nil {
		return fmt.Errorf("config not found: %s", absPath)
	}

	// Load config and create engine
	cfg, err := arenaconfig.LoadConfig(absPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	eng, err := engine.NewEngineFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create engine: %w", err)
	}

	// --mock-provider replaces every provider in the registry with a
	// generic mock, including the self-play user role and any TTS path
	// that resolves through providers. Without this, picking the
	// "mock-duplex" assistant in the UI still leaves self-play and TTS
	// hitting real APIs for $$$ — surprising for a demo.
	mockAll, _ := cmd.Flags().GetBool("mock-provider")
	mockConfig, _ := cmd.Flags().GetString("mock-config")
	if mockAll {
		if mockErr := eng.EnableMockProviderMode(mockConfig); mockErr != nil {
			return fmt.Errorf("failed to enable mock provider mode: %w", mockErr)
		}
		log.Printf("Mock provider mode: ALL providers replaced with mocks (no API calls)")
	}

	// Wire audio monitor
	mode, err := cmd.Flags().GetString("audio-monitor")
	if err != nil {
		return fmt.Errorf("failed to get audio-monitor flag: %w", err)
	}
	rate, err := cmd.Flags().GetInt("audio-rate")
	if err != nil {
		return fmt.Errorf("failed to get audio-rate flag: %w", err)
	}
	if monitorErr := eng.EnableAudioMonitor(arenaaudio.Options{
		Mode:        arenaaudio.MonitorMode(mode),
		Rate:        rate,
		LocalSink:   true,
		SSEPlayback: true,
		LevelMeter:  true,
	}); monitorErr != nil {
		return fmt.Errorf("audio monitor: %w", monitorErr)
	}

	// Wire event bus
	eventBus := events.NewEventBus()
	eng.SetEventBus(eventBus, engine.WithMessageEvents())

	// Create web adapter and subscribe to bus
	adapter := web.NewEventAdapter()
	adapter.Subscribe(eventBus)

	// Get state store for results API
	arenaStore, _ := eng.GetStateStore().(*statestore.ArenaStateStore)

	// Resolve output directory
	outDir := cfg.Defaults.Output.Dir
	if outDir == "" {
		outDir = "out"
	}
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(cfg.Defaults.ConfigDir, outDir)
	}

	// Load existing results from the output directory (if any)
	if arenaStore != nil {
		if n := web.LoadResultsIntoStore(outDir, arenaStore); n > 0 {
			log.Printf("Loaded %d existing run result(s) from %s", n, outDir)
		}
	}

	// Create web server
	srv := web.NewServer(adapter, eng, arenaStore, outDir)

	// Reserve a port free on BOTH loopback families and auto-advance if the
	// requested one is busy, so the user never lands on some other process that
	// happens to hold the port on the family "localhost" resolves to.
	l4, l6, actualPort, err := firstFreeLoopbackPort(servePort, servePortScanAttempts)
	if err != nil {
		return fmt.Errorf("could not find a free port starting at %d (try -p): %w", servePort, err)
	}
	defer func() { _ = l4.Close() }()
	if l6 != nil {
		defer func() { _ = l6.Close() }()
	}
	if actualPort != servePort {
		fmt.Printf("Port %d is in use — serving on %d instead\n", servePort, actualPort)
	}

	url := fmt.Sprintf("http://localhost:%d", actualPort)
	fmt.Printf("Arena Web UI: %s\n", url)
	fmt.Printf("Config: %s\n", absPath)
	fmt.Println("Press Ctrl+C to stop")

	if serveOpen {
		go openBrowser(url)
	}

	// NOSONAR: TLS not required - local development tool, binds to loopback only
	httpServer := &http.Server{
		Handler:     srv.Handler(),
		ReadTimeout: serverReadTimeout,
		// WriteTimeout intentionally omitted: SSE requires long-lived connections;
		// a non-zero write timeout would kill active SSE streams.
	}
	// Serve both loopback families so "localhost" reaches us however it resolves.
	// The IPv6 listener (if present) runs in the background; the IPv4 listener
	// blocks. Serve closes each listener on return.
	if l6 != nil {
		go func() {
			if serveErr := httpServer.Serve(l6); serveErr != nil && serveErr != http.ErrServerClosed {
				log.Printf("IPv6 listener stopped: %v", serveErr)
			}
		}()
	}
	if serveErr := httpServer.Serve(l4); serveErr != nil && serveErr != http.ErrServerClosed {
		return serveErr
	}
	return nil
}

// openBrowser opens the default browser to the given URL.
//
//nolint:noctx // Dev tool - context not needed for opening browser
func openBrowser(url string) {
	var cmd *exec.Cmd
	// url is internally generated (a localhost URL), never user input, so command
	// injection is not a concern. The OS launcher name (open/xdg-open/rundll32) is
	// resolved via $PATH by design — these are fixed system utilities and a dev
	// tool cannot hardcode their absolute paths across platforms.
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) //nolint:gosec // G204 fixed launcher, localhost URL (NOSONAR S4036)
	case "linux":
		cmd = exec.Command("xdg-open", url) //nolint:gosec // G204 fixed launcher, localhost URL (NOSONAR S4036)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // G204 (NOSONAR S4036)
	default:
		fmt.Printf("Open %s in your browser\n", url)
		return
	}
	_ = cmd.Run()
}
