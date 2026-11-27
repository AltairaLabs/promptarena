package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/assertions"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// twoTurnsLookupInFirst is a conversation whose first turn called lookup and
// whose second turn, the one being asserted on, called nothing.
func twoTurnsLookupInFirst() []types.Message {
	return []types.Message{
		types.NewUserMessage("Where is order 1?"),
		{Role: "assistant", ToolCalls: []types.MessageToolCall{
			{ID: "c1", Name: "lookup", Args: json.RawMessage(`{"order":"1"}`)},
		}},
		types.NewToolResultMessage(types.MessageToolResult{ID: "c1", Name: "lookup", Parts: []types.ContentPart{types.NewTextPart(`{"status":"shipped"}`)}}),
		types.NewAssistantMessage("It has shipped."),
		types.NewUserMessage("Thanks, and order 2?"),
		types.NewAssistantMessage("Could you tell me the order number again?"),
	}
}


func TestRunAssertionsAsEvals_TurnToolAssertionsSeeOnlyThisTurn(t *testing.T) {
	hook := NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "chat")
	cfgs := []assertions.AssertionConfig{
		{Type: "tools_not_called", Params: map[string]interface{}{"tools": []interface{}{"lookup"}}},
		{Type: "tools_called_session", Params: map[string]interface{}{"tools": []interface{}{"lookup"}}},
		{Type: "tool_call_count", Params: map[string]interface{}{"tool": "lookup", "min": 1}},
		{Type: "tools_called", Params: map[string]interface{}{"tools": []interface{}{"lookup"}}},
	}

	results := hook.RunAssertionsAsConversationResults(context.Background(), cfgs, twoTurnsLookupInFirst(), 5, "s1",
		evals.TriggerEveryTurn)

	require.Len(t, results, 4)
	assert.True(t, results[0].Passed, "tools_not_called: the earlier turn's call is not this turn's")
	assert.True(t, results[1].Passed, "tools_called_session still sees the whole session")
	assert.False(t, results[2].Passed, "tool_call_count min 1: this turn made no call")
	assert.False(t, results[3].Passed, "tools_called: this turn made no call")
	for i, r := range results {
		assert.Equal(t, cfgs[i].Type, r.Type, "results keep the assertions' order")
	}
}

func TestRunAssertionsAsEvals_ConversationToolAssertionsSeeWholeSession(t *testing.T) {
	hook := NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "chat")
	cfgs := []assertions.AssertionConfig{
		{Type: "tools_called", Params: map[string]interface{}{"tools": []interface{}{"lookup"}}},
	}

	results := hook.RunAssertionsAsConversationResults(context.Background(), cfgs, twoTurnsLookupInFirst(), 5, "s1",
		evals.TriggerOnConversationComplete)

	require.Len(t, results, 1)
	assert.True(t, results[0].Passed, "a conversation assertion counts every turn's calls")
}
