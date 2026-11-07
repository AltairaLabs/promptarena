package junit_test

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/engine"
	"github.com/AltairaLabs/promptarena/v2/arena/results"
	"github.com/AltairaLabs/promptarena/v2/arena/results/junit"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func withTurnAssertions(result engine.RunResult, passed bool) engine.RunResult {
	failed := 0
	if !passed {
		failed = 1
	}
	result.Messages = []types.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello", Meta: map[string]interface{}{
			"assertions": map[string]interface{}{
				"results": []interface{}{map[string]interface{}{"type": "content_includes", "passed": passed}},
				"passed":  passed,
				"total":   1,
				"failed":  failed,
			},
		}},
	}
	return result
}

// TestJUnitAgreesWithRunPassed pins every JUnit test case's pass/fail to
// results.RunPassed, the predicate index.json uses.
func TestJUnitAgreesWithRunPassed(t *testing.T) {
	expectedGuardrail := withTurnAssertions(createResultWithViolations("run-expected", "s", "mock"), true)
	unexpectedGuardrail := createResultWithViolations("run-unexpected", "s", "mock")
	turnFailure := withTurnAssertions(
		createTestResult("run-turn-fail", "s", "mock", "us-east-1", false, 0, time.Second), false)
	passing := withTurnAssertions(
		createTestResult("run-pass", "s", "mock", "us-east-1", false, 0, time.Second), true)
	runResults := []engine.RunResult{expectedGuardrail, unexpectedGuardrail, turnFailure, passing}

	junitFile := filepath.Join(t.TempDir(), "junit.xml")
	require.NoError(t, junit.NewJUnitResultRepository(junitFile).SaveResults(runResults))
	data, err := os.ReadFile(junitFile)
	require.NoError(t, err)
	var suites junit.JUnitTestSuites
	require.NoError(t, xml.Unmarshal(data, &suites))

	failedByName := map[string]bool{}
	for _, suite := range suites.TestSuites {
		for _, tc := range suite.TestCases {
			failedByName[tc.Name] = tc.Failure != nil || tc.Error != nil
		}
	}
	for i := range runResults {
		name := "mock." + runResults[i].Region + "." + runResults[i].RunID
		assert.Equal(t, !results.RunPassed(&runResults[i]), failedByName[name], name)
	}
	assert.Equal(t, 2, suites.Failures)

	for _, suite := range suites.TestSuites {
		for _, tc := range suite.TestCases {
			if tc.Name == "mock.us-east-1.run-turn-fail" {
				require.NotNil(t, tc.Failure)
				assert.Equal(t, "Turn assertions failed: 1 of 1", tc.Failure.Message)
				assert.Equal(t, "AssertionFailure", tc.Failure.Type)
			}
		}
	}
}

// TestJUnitFailsRunWithFailedTurnEval covers a run that fails only on a
// turn-level eval result, which carries no assertion counts.
func TestJUnitFailsRunWithFailedTurnEval(t *testing.T) {
	failed := false
	result := createTestResult("run-eval", "s", "mock", "us-east-1", false, 0, time.Second)
	result.Messages = []types.Message{
		{Role: "assistant", Content: "x", Meta: map[string]interface{}{
			"eval_results": []evals.EvalResult{{EvalID: "e1", Passed: &failed}},
		}},
	}
	require.False(t, results.RunPassed(&result))

	junitFile := filepath.Join(t.TempDir(), "junit.xml")
	require.NoError(t, junit.NewJUnitResultRepository(junitFile).SaveResults([]engine.RunResult{result}))
	data, err := os.ReadFile(junitFile)
	require.NoError(t, err)
	var suites junit.JUnitTestSuites
	require.NoError(t, xml.Unmarshal(data, &suites))
	require.Len(t, suites.TestSuites, 1)
	require.Len(t, suites.TestSuites[0].TestCases, 1)
	tc := suites.TestSuites[0].TestCases[0]
	require.NotNil(t, tc.Failure)
	assert.Equal(t, "Turn evals failed", tc.Failure.Message)
}
