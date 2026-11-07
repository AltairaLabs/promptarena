package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bashComplete sources the packc bash completion and returns COMPREPLY for the
// given words, with the cursor on the last one.
func bashComplete(t *testing.T, words ...string) []string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	script := filepath.Join(t.TempDir(), "packc.bash")
	require.NoError(t, os.WriteFile(script, []byte(bashCompletion), 0o600))
	driver := `source "$1"; shift; COMP_WORDS=("$@"); COMP_CWORD=$(( ${#COMP_WORDS[@]} - 1 )); ` +
		`_packc_completions; printf '%s\n' "${COMPREPLY[@]}"`
	args := append([]string{"-c", driver, "bash", script}, words...)
	out, err := exec.CommandContext(context.Background(), bash, args...).Output() //nolint:gosec // test-only
	require.NoError(t, err)
	return strings.Fields(string(out))
}

// Flags were offered only when the previous word was the subcommand, so the
// second flag got nothing.
func TestBashCompletion_SecondFlagStillCompletes(t *testing.T) {
	got := bashComplete(t, "packc", "compile", "-c", "config.arena.yaml", "--")
	assert.Contains(t, got, "--output")
	assert.Contains(t, got, "--id")

	got = bashComplete(t, "packc", "compile-prompt", "-p", "x.yaml", "--")
	assert.Contains(t, got, "--output")

	assert.Contains(t, bashComplete(t, "packc", "comp"), "compile")
	assert.Contains(t, bashComplete(t, "packc", "completion", ""), "zsh")
	assert.Empty(t, bashComplete(t, "packc", "compile", "--id", ""))
}
