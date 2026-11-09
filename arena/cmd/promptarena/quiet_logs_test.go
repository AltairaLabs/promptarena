package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"
)

// TestQuietSetupLogs_NonTUINoop verifies the headless path leaves logging alone:
// logs keep going where they already went, and no promptarena.log is created
// even for a verbose run with an output directory.
//
// Not parallel: it mutates the process-global runtime logger output.
func TestQuietSetupLogs_NonTUINoop(t *testing.T) {
	var current bytes.Buffer
	logger.SetOutput(&current)
	t.Cleanup(func() { logger.SetOutput(nil) })
	dir := t.TempDir()

	restore := quietSetupLogs(false, &RunParameters{Verbose: true, OutDir: dir})
	logger.Info("headless-setup-marker")
	restore()
	logger.Info("headless-after-restore-marker")

	assert.Contains(t, current.String(), "headless-setup-marker", "setup logs must not be redirected")
	assert.Contains(t, current.String(), "headless-after-restore-marker",
		"restore must not move logging off its existing output")
	assert.NoFileExists(t, filepath.Join(dir, "promptarena.log"))
}

// TestQuietSetupLogs_VerboseCapturesToFile verifies that on the verbose TUI path
// engine-setup logs are routed to promptarena.log (off the screen) and that
// restore returns logging to stderr afterwards.
//
// Not parallel: it mutates the process-global runtime logger output.
func TestQuietSetupLogs_VerboseCapturesToFile(t *testing.T) {
	dir := t.TempDir()
	params := &RunParameters{Verbose: true, OutDir: dir}

	restore := quietSetupLogs(true, params)
	logger.Info("setup-log-marker")
	restore()

	// Anything logged after restore must NOT land in the file.
	logger.Info("post-restore-marker")

	data, err := os.ReadFile(filepath.Join(dir, "promptarena.log"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "setup-log-marker")
	assert.NotContains(t, string(data), "post-restore-marker")
}
