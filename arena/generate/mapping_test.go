package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func TestViolatesAny(t *testing.T) {
	evals := []EvalResult{
		{ID: "faith", Kind: "eval", Score: f(0.3)},
		{ID: "tox", Kind: "eval", Score: f(0.1)},
		{ID: "blank", Kind: "eval"},
	}
	assert.False(t, ViolatesAny(evals, nil), "no expectations: nothing to violate")
	assert.True(t, ViolatesAny(evals, []Expectation{{EvalID: "faith", Min: f(0.8)}}))
	assert.False(t, ViolatesAny(evals, []Expectation{{EvalID: "tox", Max: f(0.2)}}))
	assert.True(t, ViolatesAny(evals, []Expectation{{EvalID: "tox", Max: f(0.2)}, {EvalID: "faith", Min: f(0.8)}}),
		"any one violated expectation is enough")
	assert.True(t, ViolatesAny(evals, []Expectation{{EvalID: "never-measured", Min: f(0.5)}}),
		"a session that never measured the eval cannot satisfy its range")
	assert.True(t, ViolatesAny(evals, []Expectation{{EvalID: "blank", Min: f(0.5)}}),
		"a measurement with no score cannot satisfy its range")
}

func TestHasFailedVerdict(t *testing.T) {
	no, yes := false, true
	assert.False(t, HasFailedVerdict(nil))
	assert.False(t, HasFailedVerdict([]EvalResult{{Kind: "eval", Score: f(0.01)}}),
		"a low score with no verdict is not a failure")
	assert.False(t, HasFailedVerdict([]EvalResult{{Kind: "assertion", Passed: &yes}}))
	assert.True(t, HasFailedVerdict([]EvalResult{{Kind: "eval", Score: f(0.9)}, {Kind: "assertion", Passed: &no}}))
}

// Tool traffic and the assistant reply belong to the user turn that opened
// the exchange; anything before the first user message belongs to none.
func TestUserTurns(t *testing.T) {
	msgs := []types.Message{
		{Role: "system"},
		{Role: "user"},
		{Role: "assistant"},
		{Role: "tool"},
		{Role: "assistant"},
		{Role: "user"},
		{Role: "assistant"},
	}
	assert.Equal(t, []int{-1, 0, 0, 0, 0, 1, 1}, UserTurns(msgs))
	assert.Empty(t, UserTurns(nil))
}

func TestUserTurnIndex(t *testing.T) {
	msgs := []types.Message{{Role: "system"}, {Role: "user"}, {Role: "assistant"}, {Role: "tool"}, {Role: "user"}, {Role: "assistant"}}
	ids := []string{"m-sys", "m-u1", "m-a1", "m-t1", "m-u2", "m-a2"}
	assert.Equal(t, map[string]int{"m-u1": 0, "m-a1": 0, "m-t1": 0, "m-u2": 1, "m-a2": 1}, UserTurnIndex(msgs, ids),
		"keyed by the store's id, numbered by user turn rather than message position; the system prompt answers no turn")

	short := UserTurnIndex(msgs, []string{"", "m-u1"})
	assert.Equal(t, map[string]int{"m-u1": 0}, short, "missing or empty ids are left out")
}

func TestEvalResultsFromRows(t *testing.T) {
	msgs := []types.Message{{Role: "user"}, {Role: "assistant"}, {Role: "tool"}, {Role: "assistant"}, {Role: "user"}, {Role: "assistant"}}
	index := UserTurnIndex(msgs, []string{"u1", "a1", "t1", "a1b", "u2", "a2"})
	no := false
	rows := []EvalRow{
		{ID: "faith", Type: "faithfulness", Kind: "eval", Score: f(0.4), MessageID: "a2"},
		{ID: "cite", Type: "content_includes", Kind: "assertion", Passed: &no, Message: "cite policy",
			Params: map[string]any{"patterns": []any{"policy"}}, MessageID: "a1b"},
		{ID: "overall", Type: "llm_judge", Kind: "eval", Score: f(0.7)},
		{ID: "orphan", Type: "llm_judge", Kind: "eval", Score: f(0.5), MessageID: "deleted-msg"},
	}
	got := EvalResultsFromRows(rows, index)
	require.Len(t, got, 4)

	assert.Equal(t, "eval", got[0].Kind)
	assert.Nil(t, got[0].Passed, "a score without a verdict keeps Passed nil, not false")
	assert.Equal(t, 0.4, *got[0].Score)
	require.NotNil(t, got[0].Turn)
	assert.Equal(t, 1, *got[0].Turn, "the reply at position 5 answers user turn 1")

	assert.True(t, got[1].Failed())
	require.NotNil(t, got[1].Turn)
	assert.Equal(t, 0, *got[1].Turn, "an assistant message after tool traffic still answers user turn 0, not position 3")
	assert.Equal(t, "cite policy", got[1].Message)
	assert.Equal(t, map[string]any{"patterns": []any{"policy"}}, got[1].Params)

	assert.Nil(t, got[2].Turn, "no message id: conversation-level")
	assert.Nil(t, got[3].Turn, "an unknown message id is conversation-level, not dropped")
	assert.False(t, HasFailedVerdict(got[2:3]))

	assert.Nil(t, EvalResultsFromRows(nil, index))
}
