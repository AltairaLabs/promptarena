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
// RegisterExecutor. All three do it at engine-init time only.
//
// The tool registry is shared by every concurrent run and keys executors by
// name, so a registration made once a run is underway silently takes ownership
// of that tool for every run in flight. That is how the memory subsystem came
// to write one run's memories into another run's scope: it built a per-run
// executor carrying that run's scope and registered it from the run path.
//
// Registration belongs at init, describing what the runtime CAN do. Per-run
// state belongs in a run-keyed map inside the registered executor, resolved
// from runIDFromContext at execute time — see memoryToolExecutor and
// skillsToolExecutor.
//
// If you are adding a subsystem that needs per-run state, follow those two
// rather than adding a file here.
var registrationAllowlist = map[string]bool{
	"builder_integration.go":            true,
	"execution_memory_integration.go":   true,
	"execution_workflow_integration.go": true,
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
			"%s registers a tool executor outside engine init; the registry is shared by every "+
				"concurrent run, so per-run state must be resolved at execute time instead", name)
	}
}

// The allowlisted files may register at init, but must not do so from the
// per-run functions. This pins the specific regression: registerMemoryForRun
// used to call RegisterExecutor.
func TestPerRunFunctionsDoNotRegisterExecutors(t *testing.T) {
	perRunFunctions := map[string]string{
		"execution_memory_integration.go": "func (e *Engine) registerMemoryForRun(",
		"execution.go":                    "func (e *Engine) executeScenarioRun(",
	}

	for file, signature := range perRunFunctions {
		src, err := os.ReadFile(filepath.Clean(file))
		require.NoError(t, err)

		body, found := functionBody(string(src), signature)
		require.True(t, found, "could not locate %q in %s — update this test if it was renamed", signature, file)

		assert.NotContains(t, body, "RegisterExecutor(",
			"%s registers a tool executor per run, which clobbers the shared registry slot", signature)
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
