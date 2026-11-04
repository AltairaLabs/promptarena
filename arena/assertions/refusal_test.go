package assertions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"

	"github.com/AltairaLabs/promptarena/v2/arena/assertions/refusal"
)

func TestRefusal_AsScenarioAssertion(t *testing.T) {
	runner := evals.NewEvalRunner(evals.NewEvalTypeRegistry())
	ctx := evals.BuildEvalContext([]types.Message{{Role: "assistant", Content: "I'm unable to do that."}},
		0, "s", "", nil)
	refused := ToEvalDef(AssertionConfig{Type: refusal.TypeName}, 0)
	answered := ToEvalDef(AssertionConfig{Type: refusal.TypeName, Params: map[string]interface{}{"max_score": 0.0}}, 1)
	results := runner.RunTurnEvals(context.Background(), []evals.EvalDef{refused, answered}, ctx)
	require.Len(t, results, 2)
	assert.True(t, *results[0].Passed, "a refusal passes the default assertion")
	assert.False(t, *results[1].Passed, "and fails max_score: 0, the over-refusal check")
}
