package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/promptarena/arena/tui/app"

	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"
)

var rootCmd = &cobra.Command{
	Use:           "promptarena",
	Short:         "PromptKit Arena - Multi-turn conversation simulation and testing tool",
	Version:       GetVersion(),
	SilenceUsage:  true,  // Don't print usage on error
	SilenceErrors: false, // Do print errors
	Long: `PromptKit Arena is a testing framework for running multi-turn conversation
simulations across multiple LLM providers and system prompts.

It evaluates tone consistency, content quality, prompt adherence, and validates
conversation flows for various task types (customer support, code assistance, etc.).`,
	// RunE fires only when no sub-command is provided (bare `promptarena`).
	// Sub-commands registered via rootCmd.AddCommand() take precedence and
	// RunE is NOT called for them, so existing commands are unaffected.
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not determine working directory: %w", err)
		}
		ctx := buildHubContext(cwd, os.Stderr)
		return runHub(ctx, app.NewHome(ctx, app.DefaultMenu(ctx)))
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize logger based on verbose flag if present
		// This runs before all subcommands
		if cmd.Flags().Changed("verbose") {
			verbose, err := cmd.Flags().GetBool("verbose")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting verbose flag: %v\n", err)
				return
			}
			logger.SetVerbose(verbose)
		}
	},
}

// runHub starts the interactive hub. It is a variable rather than a direct call
// to app.Run so the bare-`promptarena` path is reachable from a test: app.Run
// hands control to a Bubble Tea program that owns the terminal and does not
// return, which no test can call.
var runHub = app.Run

// buildHubContext assembles the AppContext for a bare `promptarena` hub launch:
// version, a default results directory, and whatever config is discoverable
// from cwd. Split out of RunE so it is reachable from a test — RunE's remaining
// statement hands control to the Bubble Tea program, which never returns under
// `go test`.
//
// A config that is present but unloadable is a warning, not a failure: the hub
// is how you find out what is wrong with your kit, so refusing to open it over
// a malformed config is the least useful thing it could do.
func buildHubContext(cwd string, warnTo io.Writer) *app.AppContext {
	ctx := &app.AppContext{
		Version:    GetVersion(),
		ResultsDir: filepath.Join(cwd, "out"),
	}

	cfgPath, found := app.DiscoverConfig(cwd)
	if !found {
		return ctx
	}

	if loadErr := ctx.LoadConfig(cfgPath); loadErr != nil {
		_, _ = fmt.Fprintf(warnTo, "warning: could not load config %s: %v\n", cfgPath, loadErr)
		return ctx
	}

	// LoadConfig sets ResultsDir relative to the config file; override only if
	// it is still empty (defensive fallback — LoadConfig always sets it in
	// practice, but guard against future refactors).
	if ctx.ResultsDir == "" {
		ctx.ResultsDir = filepath.Join(cwd, "out")
	}
	return ctx
}

// setupVersion configures the version display
func setupVersion() {
	// Set custom version template to show detailed version info
	rootCmd.SetVersionTemplate(GetVersionInfo() + "\n")
}

// normalizeHelpArgs returns a copy of args with every standalone "-?" token
// replaced by "--help". Cobra reserves -h/--help internally and does not allow
// registering "?" as a shorthand on the help flag, so we translate the alias
// before cobra parses the arguments. Only the exact token "-?" is rewritten;
// "--help", "-h", and unrelated arguments pass through unchanged.
func normalizeHelpArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == "-?" {
			out[i] = "--help"
			continue
		}
		out[i] = a
	}
	return out
}

func Execute() {
	setupVersion()
	rootCmd.SetArgs(normalizeHelpArgs(os.Args[1:]))
	err := rootCmd.Execute()
	if err != nil {
		// Error already printed by cobra
		os.Exit(1)
	}
}

// installCtrlCEscapeHatch is a last-resort guard against a wedged TUI
// (e.g. Bubble Tea blocked on a stuck pipeline goroutine) leaving the
// terminal unusable. It does NOT replace whatever signal handling the
// TUI / cobra commands already have — it sits behind them as a safety
// net.
//
// Behavior:
//   - First Ctrl-C: print a hint and start a 5-second watchdog. The
//     existing handlers (cobra context cancel, Bubble Tea quit) are
//     still notified by the runtime — signal.Notify does not consume
//     the signal, it only adds us as another listener — so they keep
//     trying to shut down cleanly.
//   - Second Ctrl-C OR 5s without exit: restore the terminal (cursor,
//     primary buffer, attributes) and force-exit 130.
//
// The terminal restore sequences cover the 99% case of a Bubble Tea
// program that left the alternate screen + cursor hidden + raw mode.
// If the terminal is hosed worse than that, the user can run `reset`
// or `stty sane` afterwards.
const (
	ctrlCWatchdogTimeout = 5 * time.Second
	// sigChanBuf is the buffer depth for the SIGINT/SIGTERM channel — large
	// enough to hold a duplicate "second Ctrl-C" arriving before the
	// goroutine drains the first.
	sigChanBuf = 2
	// exitCodeSIGINT is the standard "terminated by SIGINT" exit code (128 + 2).
	exitCodeSIGINT = 130
)

// terminalRestoreSeq shows the cursor, leaves the alternate screen, leaves
// bracketed paste mode and resets attributes — the usual things a TUI puts the
// terminal into and the only ones worth undoing blind.
const terminalRestoreSeq = "\x1b[?25h\x1b[?1049l\x1b[?2004l\x1b[0m"

// awaitForceExit blocks until the escape hatch should fire, writing its running
// commentary to out, and returns without exiting. Split from
// installCtrlCEscapeHatch so both arms — a second signal, and the watchdog
// expiring — are reachable from a test; the os.Exit stays in the caller, which
// is the one statement that cannot run under `go test`.
func awaitForceExit(sigCh <-chan os.Signal, timeout time.Duration, out io.Writer) {
	<-sigCh // first signal
	_, _ = fmt.Fprintln(out,
		"\n[promptarena] shutdown requested. Press Ctrl-C again or wait 5s to force-exit.")
	select {
	case <-sigCh:
		_, _ = fmt.Fprintln(out, "[promptarena] force-exiting (second signal received)")
	case <-time.After(timeout):
		_, _ = fmt.Fprintln(out,
			"[promptarena] force-exiting (graceful shutdown did not complete within 5s)")
	}
	_, _ = io.WriteString(out, terminalRestoreSeq)
}

func installCtrlCEscapeHatch() {
	sigCh := make(chan os.Signal, sigChanBuf)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		awaitForceExit(sigCh, ctrlCWatchdogTimeout, os.Stderr)
		os.Exit(exitCodeSIGINT)
	}()
}

func main() {
	installCtrlCEscapeHatch()
	Execute()
}
