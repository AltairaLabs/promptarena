package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/tui/app"
	"github.com/spf13/cobra"

	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateResultRepository_UnsupportedFormat(t *testing.T) {
	params := &RunParameters{
		OutDir:        "/tmp/test",
		OutputFormats: []string{"invalid"},
	}

	repo, err := createResultRepository(params, "")
	assert.Error(t, err)
	assert.Nil(t, repo)
	assert.Contains(t, err.Error(), "unsupported output format: invalid")
}

func TestContains(t *testing.T) {
	slice := []string{"a", "b", "c"}

	assert.True(t, contains(slice, "a"))
	assert.True(t, contains(slice, "b"))
	assert.True(t, contains(slice, "c"))
	assert.False(t, contains(slice, "d"))
	assert.False(t, contains([]string{}, "a"))
}

// TestSetupVersion_UsesTheDetailedTemplate covers setupVersion, which was 0%.
// `promptarena --version` is the first thing anyone pastes into a bug report,
// so it has to print the detailed build info rather than cobra's bare
// "promptarena version <x>" default.
func TestSetupVersion_UsesTheDetailedTemplate(t *testing.T) {
	t.Cleanup(func() { rootCmd.SetVersionTemplate("") })

	setupVersion()

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	t.Cleanup(func() { rootCmd.SetOut(nil) })

	// Rendering the template is what proves it was installed; cobra keeps it
	// private otherwise.
	require.NoError(t, tmplRenderVersion(rootCmd, &out))
	assert.Contains(t, out.String(), GetVersionInfo())
}

// tmplRenderVersion asks cobra to print the version using whatever template is
// currently installed, which is the only way to observe SetVersionTemplate.
func tmplRenderVersion(cmd *cobra.Command, w io.Writer) error {
	cmd.SetOut(w)
	cmd.SetArgs([]string{"--version"})
	defer cmd.SetArgs(nil)
	return cmd.Execute()
}

// TestExecute_NormalizesTheQuestionMarkAlias covers Execute, which was 0%.
// "-?" is not something cobra can express as a help shorthand, so Execute
// rewrites it before parsing; without that the CLI answers a bare "-?" with an
// unknown-flag error instead of help.
func TestExecute_NormalizesTheQuestionMarkAlias(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() {
		os.Args = origArgs
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetVersionTemplate("")
	})

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	os.Args = []string{"promptarena", "-?"}

	// Execute only calls os.Exit on a non-nil error from cobra; --help is not
	// an error, so this returns normally.
	Execute()

	assert.Contains(t, out.String(), "Usage:",
		"-? must be translated to --help before cobra parses it")
}

// TestAwaitForceExit_SecondSignalWins covers the escape hatch's fast path. The
// hatch exists for a TUI wedged badly enough that its own handlers never run,
// so an impatient second Ctrl-C has to be honoured immediately rather than
// waiting out the full watchdog.
func TestAwaitForceExit_SecondSignalWins(t *testing.T) {
	sigCh := make(chan os.Signal, sigChanBuf)
	var out bytes.Buffer

	done := make(chan struct{})
	go func() {
		defer close(done)
		// A long timeout: if the second signal is not honoured this test fails
		// by timing out rather than passing for the wrong reason.
		awaitForceExit(sigCh, time.Minute, &out)
	}()

	sigCh <- os.Interrupt
	sigCh <- os.Interrupt

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a second signal must force-exit without waiting for the watchdog")
	}

	got := out.String()
	assert.Contains(t, got, "shutdown requested")
	assert.Contains(t, got, "second signal received")
	assert.Contains(t, got, terminalRestoreSeq, "the terminal must be restored on the way out")
}

// TestAwaitForceExit_WatchdogFiresWithoutASecondSignal covers the other arm: a
// graceful shutdown that never completes must not hang forever, or the user is
// left with an unusable terminal and no way out but `kill`.
func TestAwaitForceExit_WatchdogFiresWithoutASecondSignal(t *testing.T) {
	sigCh := make(chan os.Signal, sigChanBuf)
	var out bytes.Buffer

	done := make(chan struct{})
	go func() {
		defer close(done)
		awaitForceExit(sigCh, 10*time.Millisecond, &out)
	}()

	sigCh <- os.Interrupt

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the watchdog must fire on its own")
	}

	got := out.String()
	assert.Contains(t, got, "did not complete within 5s")
	assert.Contains(t, got, terminalRestoreSeq)
}

// TestBuildHubContext_DiscoversConfigOrFallsBack covers the config discovery a
// bare `promptarena` does before opening the hub. The hub is where you go to
// find out what is wrong with a kit, so a broken or absent config has to leave
// it openable rather than abort the launch.
func TestBuildHubContext_DiscoversConfigOrFallsBack(t *testing.T) {
	t.Run("no config still yields a usable context", func(t *testing.T) {
		dir := t.TempDir()
		var warn bytes.Buffer

		ctx := buildHubContext(dir, &warn)

		require.NotNil(t, ctx)
		assert.Equal(t, GetVersion(), ctx.Version)
		assert.Equal(t, filepath.Join(dir, "out"), ctx.ResultsDir)
		assert.Empty(t, warn.String(), "an absent config is not worth warning about")
	})

	t.Run("an unloadable config warns but still opens the hub", func(t *testing.T) {
		dir := t.TempDir()
		// DiscoverConfig finds this; LoadConfig cannot parse it.
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "config.arena.yaml"), []byte("\t\tnot: [valid"), 0o600))

		var warn bytes.Buffer
		ctx := buildHubContext(dir, &warn)

		require.NotNil(t, ctx, "a malformed config must not prevent the hub opening")
		assert.Equal(t, filepath.Join(dir, "out"), ctx.ResultsDir)
		assert.Contains(t, warn.String(), "could not load config")
	})
}

// TestBuildHubContext_LoadsAValidConfig covers the successful-load branch. The
// hub takes its results directory from the config's own location, so a kit
// opened from elsewhere still points at that kit's out/ rather than the
// caller's working directory.
func TestBuildHubContext_LoadsAValidConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: hub-context-test
spec:
  scenarios: []
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.arena.yaml"), []byte(cfg), 0o600))

	var warn bytes.Buffer
	ctx := buildHubContext(dir, &warn)

	require.NotNil(t, ctx)
	assert.Empty(t, warn.String(), "a config that loads must not warn")
	assert.NotEmpty(t, ctx.ResultsDir, "a loaded config must leave a results directory set")
}

// TestPersistentPreRun_HonorsVerboseOnlyWhenSet covers the root command's
// PersistentPreRun. It runs ahead of every subcommand, so flipping the logger
// when the flag was never given would make every run verbose.
func TestPersistentPreRun_HonorsVerboseOnlyWhenSet(t *testing.T) {
	require.NotNil(t, rootCmd.PersistentPreRun)
	t.Cleanup(func() { logger.SetVerbose(false) })

	debugEnabled := func() bool {
		return logger.GetLogger().Enabled(context.Background(), slog.LevelDebug)
	}
	probe := func(verboseArgs ...string) *cobra.Command {
		cmd := &cobra.Command{Use: "probe"}
		cmd.Flags().Bool("verbose", false, "")
		require.NoError(t, cmd.Flags().Parse(verboseArgs))
		return cmd
	}

	t.Run("an untouched flag leaves the logger alone", func(t *testing.T) {
		// Debug is already on (say from LOG_LEVEL); the flag's default false
		// must not switch it off.
		logger.SetVerbose(true)
		rootCmd.PersistentPreRun(probe(), nil)
		assert.True(t, debugEnabled(), "a --verbose that was never given must not reset the level")
	})

	t.Run("an explicit --verbose is applied", func(t *testing.T) {
		logger.SetVerbose(false)
		rootCmd.PersistentPreRun(probe("--verbose"), nil)
		assert.True(t, debugEnabled(), "--verbose must turn debug logging on")
	})

	t.Run("an explicit --verbose=false is applied", func(t *testing.T) {
		logger.SetVerbose(true)
		rootCmd.PersistentPreRun(probe("--verbose=false"), nil)
		assert.False(t, debugEnabled(), "--verbose=false must turn debug logging off")
	})

	t.Run("a command with no verbose flag at all leaves the logger alone", func(t *testing.T) {
		logger.SetVerbose(true)
		rootCmd.PersistentPreRun(&cobra.Command{Use: "probe"}, nil)
		assert.True(t, debugEnabled())
	})
}

// TestRootCommandLaunchesTheHub covers the bare-`promptarena` path. There is no
// subcommand to name it, so a regression here — a nil context, the wrong
// results directory — would only surface as a hub that opens looking empty.
func TestRootCommandLaunchesTheHub(t *testing.T) {
	var gotCtx *app.AppContext
	orig := runHub
	runHub = func(ctx *app.AppContext, root app.Page) error {
		gotCtx = ctx
		return nil
	}
	t.Cleanup(func() { runHub = orig })

	dir := t.TempDir()
	origWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	require.NoError(t, rootCmd.RunE(rootCmd, nil))

	require.NotNil(t, gotCtx, "the hub must be handed a context")
	assert.Equal(t, GetVersion(), gotCtx.Version)
	assert.NotEmpty(t, gotCtx.ResultsDir)
}

func TestExtractObjectivesGroupBy(t *testing.T) {
	t.Run("defaults to scenario without the flag", func(t *testing.T) {
		params := &RunParameters{}
		require.NoError(t, extractObjectivesGroupBy(&cobra.Command{}, params))
		assert.Equal(t, []string{"scenario"}, params.ObjectivesGroupBy)
	})

	newCmd := func(args ...string) *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().StringSlice(flagObjectivesGroupBy, []string{"scenario"}, "")
		require.NoError(t, cmd.Flags().Parse(args))
		return cmd
	}

	withFormat := func() *RunParameters { return &RunParameters{OutputFormats: []string{"objectives"}} }

	t.Run("reads the flag", func(t *testing.T) {
		params := withFormat()
		require.NoError(t, extractObjectivesGroupBy(newCmd("--objectives-group-by=label.category,data.tier"), params))
		assert.Equal(t, []string{"label.category", "data.tier"}, params.ObjectivesGroupBy)
	})

	t.Run("rejects an unknown key", func(t *testing.T) {
		err := extractObjectivesGroupBy(newCmd("--objectives-group-by=row"), withFormat())
		assert.ErrorContains(t, err, `unknown objectives group-by key "row"`)
	})

	t.Run("without the format the flag is ignored with a warning", func(t *testing.T) {
		cmd := newCmd("--objectives-group-by=row")
		var stderr bytes.Buffer
		cmd.SetErr(&stderr)
		params := &RunParameters{OutputFormats: []string{"json"}}
		require.NoError(t, extractObjectivesGroupBy(cmd, params))
		assert.Contains(t, stderr.String(), "has no effect without --formats objectives")
	})
}
