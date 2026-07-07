package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/logger"
)

// TestQuietSetupLogs_NonTUINoop verifies the headless path leaves logging alone.
func TestQuietSetupLogs_NonTUINoop(t *testing.T) {
	restore := quietSetupLogs(false, &RunParameters{})
	require.NotNil(t, restore)
	restore() // must not panic
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
