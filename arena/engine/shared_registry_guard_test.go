package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registrationAllowlist names the engine files permitted to call
// RegisterExecutor.
//
// The engine-wide registry is shared by every concurrent run and keys executors
// by name, holding exactly one per name. An executor registered into it once a
// run is underway silently takes ownership of that tool for every run in
// flight. That is how the memory subsystem came to write one run's memories
// into another run's scope.
//
// The first three register at engine-init time only. run_tools.go is the
// exception that proves the rule: it registers per run, but into that run's own
// child registry — TestRunToolsRegistersOnlyIntoTheChildRegistry below pins
// that it never touches the engine-wide one.
var registrationAllowlist = map[string]bool{
	"builder_integration.go":            true,
	"execution_memory_integration.go":   true,
	"execution_workflow_integration.go": true,
	"run_tools.go":                      true,
}

func TestExecutorRegistrationIsConfinedToInit(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if registrationAllowlist[name] {
			continue
		}

		src, readErr := os.ReadFile(filepath.Clean(name))
		require.NoError(t, readErr)

		assert.NotContains(t, string(src), "RegisterExecutor(",
			"%s registers a tool executor outside engine init; the engine registry is shared by "+
				"every concurrent run, so per-run executors belong in the run's child registry "+
				"(see run_tools.go)", name)
	}
}

// The per-run builder may register freely, but only into the run's own child.
// A single `e.toolRegistry.RegisterExecutor` here would reintroduce the exact
// bug the child registry was adopted to make unrepresentable.
func TestRunToolsRegistersOnlyIntoTheChildRegistry(t *testing.T) {
	src, err := os.ReadFile(filepath.Clean("run_tools.go"))
	require.NoError(t, err)

	body, found := functionBody(string(src), "func (e *Engine) buildRunTools(")
	require.True(t, found, "could not locate buildRunTools — update this test if it was renamed")

	assert.NotContains(t, body, "e.toolRegistry.RegisterExecutor(",
		"buildRunTools must register into the run's child registry, never the engine-wide one")
	assert.Contains(t, body, "e.toolRegistry.Child()",
		"buildRunTools must take a child of the engine registry")
}

// The run path itself must not register: it delegates to buildRunTools, which
// owns the child. This pins the specific regression — registerMemoryForRun used
// to call RegisterExecutor on the shared registry from inside a run.
func TestPerRunFunctionsDoNotRegisterExecutors(t *testing.T) {
	perRunFunctions := map[string]string{
		"execution.go": "func (e *Engine) executeScenarioRun(",
	}

	for file, signature := range perRunFunctions {
		src, err := os.ReadFile(filepath.Clean(file))
		require.NoError(t, err)

		body, found := functionBody(string(src), signature)
		require.True(t, found, "could not locate %q in %s — update this test if it was renamed", signature, file)

		assert.NotContains(t, body, "RegisterExecutor(",
			"%s registers a tool executor per run; that belongs in buildRunTools, on the child", signature)
	}
}

// functionBody returns the source between the given function signature and the
// closing brace in column 0 that ends it.
func functionBody(src, signature string) (string, bool) {
	start := strings.Index(src, signature)
	if start < 0 {
		return "", false
	}
	rest := src[start:]
	if end := strings.Index(rest, "\n}\n"); end >= 0 {
		return rest[:end], true
	}
	return rest, true
}
