package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"
	"github.com/AltairaLabs/promptarena/v2/arena/turnexecutors"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// rewindHarness runs a scripted opening turn followed by a self-play turn
// whose agent replies with replies[i] on exchange i+1 ("nothing yet" after
// them). Each exchange costs 0.01 for the agent's reply.
type rewindHarness struct {
	replies   []string
	completes map[int]bool // exchanges that return ErrConversationComplete
	vars      []map[string]string
	history   [][]string // the conversation each exchange started from
	// store overrides the in-memory state store.
	store statestore.Store
	// transformed sends each persona message rewritten, as a transform
	// would, with the original kept under persona_original.
	transformed bool
}

func (h *rewindHarness) run(t *testing.T, turn arenaconfig.TurnDefinition) (*ConversationResult, []types.Message) {
	t.Helper()
	store := h.store
	if store == nil {
		store = createTestStateStore()
	}
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{
			{Role: "user", Content: "Start"}, {Role: "assistant", Content: "Hello"},
		})
	}}
	selfPlay := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		h.vars = append(h.vars, req.SelfPlayVars)
		n := len(h.vars)
		state, err := store.Load(ctx, req.ConversationID)
		require.NoError(t, err)
		var seen []string
		for i := range state.Messages {
			seen = append(seen, state.Messages[i].Content)
		}
		h.history = append(h.history, seen)

		reply := "nothing yet"
		if n <= len(h.replies) {
			reply = h.replies[n-1]
		}
		ask := types.Message{Role: "user", Content: fmt.Sprintf("ask %d", n)}
		if h.transformed {
			ask.Meta = map[string]interface{}{selfplay.MetaPersonaOriginal: ask.Content}
			ask.Content += " (sent from my phone)"
		}
		if err := saveMessagesToStateStore(ctx, req, []types.Message{
			ask,
			{Role: "assistant", Content: reply, CostInfo: &types.CostInfo{TotalCost: 0.01, OutputTokens: 10}},
		}); err != nil {
			return err
		}
		if h.completes[n] {
			return selfplay.ErrConversationComplete
		}
		return nil
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
	executor.evalOrchestrator = NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")

	turn.Role = "attacker"
	turn.Persona = "attacker"
	res := executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{
			ID:       "rewind",
			TaskType: "assistant",
			Turns:    []arenaconfig.TurnDefinition{{Role: "user", Content: "Start"}, turn},
		},
		Provider:         &MockProvider{id: "test"},
		Config:           &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: store, UserID: "u"},
		ConversationID:   "rewind-conv",
	})
	state, err := store.Load(context.Background(), "rewind-conv")
	require.NoError(t, err)
	return res, state.Messages
}

// rewindObjective is refundObjective with a rule that rewinds any exchange
// whose reply contains "I can't".
func rewindObjective(maxRewinds int) *arenaconfig.ObjectiveConfig {
	obj := refundObjective("", nil)
	obj.Rewind = &arenaconfig.ObjectiveRewindConfig{
		Max: maxRewinds,
		When: []arenaconfig.AssertionConfig{{
			Type:   "content_includes_any",
			Params: map[string]interface{}{"patterns": []interface{}{"I can't"}},
		}},
	}
	return obj
}

func contents(messages []types.Message) []string {
	out := make([]string, len(messages))
	for i := range messages {
		out[i] = messages[i].Content
	}
	return out
}

func TestObjectiveRewind_RemovesMatchingExchanges(t *testing.T) {
	h := &rewindHarness{replies: []string{"I can't help", "I can't do that", "REFUND ISSUED"}}
	res, stored := h.run(t, arenaconfig.TurnDefinition{Turns: 5, Objective: rewindObjective(0)})

	require.False(t, res.Failed, res.Error)
	require.Len(t, h.vars, 3)
	assert.Equal(t, []string{"Start", "Hello", "ask 3", "REFUND ISSUED"}, contents(stored),
		"the rewound exchanges are gone from the conversation")
	assert.Equal(t, []string{"Start", "Hello", "ask 3", "REFUND ISSUED"}, contents(res.Messages))
	for i, seen := range h.history {
		assert.Equal(t, []string{"Start", "Hello"}, seen, "exchange %d starts from before the rewound ones", i+1)
	}

	r := objectiveResult(t, res)
	assert.True(t, r.Passed)
	assert.Equal(t, 3, r.Details["achieved_at_exchange"])
	assert.Equal(t, 3, r.Details["exchanges"], "rewound exchanges still count")
	assert.Equal(t, "get a refund: achieved at exchange 3 of 3 (2 rewound)", r.Message)
	rewinds, ok := r.Details["rewinds"].([]map[string]interface{})
	require.True(t, ok, "rewinds detail: %#v", r.Details["rewinds"])
	require.Len(t, rewinds, 2)
	assert.Equal(t, 1, rewinds[0]["exchange"])
	assert.Equal(t, "ask 1", rewinds[0]["message"])
	assert.Equal(t, "I can't help", rewinds[0]["response"])
	assert.InDelta(t, 0.01, rewinds[0]["cost_usd"], 1e-9)
	assert.Equal(t, 2, rewinds[1]["exchange"])

	assert.InDelta(t, 0.03, res.Cost.TotalCost, 1e-9, "rewound exchanges' cost still counts")
	assert.Equal(t, 30, res.Cost.OutputTokens)
}

func TestObjectiveRewind_TellsThePersonaWhatWasRemoved(t *testing.T) {
	h := &rewindHarness{replies: []string{"I can't help", "no"}}
	_, _ = h.run(t, arenaconfig.TurnDefinition{Turns: 3, Objective: rewindObjective(2)})

	require.Len(t, h.vars, 3)
	assert.Equal(t, "", h.vars[0][selfplay.VarRewoundMessage])
	assert.Equal(t, "2", h.vars[0][selfplay.VarRewindsLeft])

	assert.Equal(t, "ask 1", h.vars[1][selfplay.VarRewoundMessage])
	assert.Equal(t, "I can't help", h.vars[1][selfplay.VarRewoundResponse])
	assert.Equal(t, "1", h.vars[1][selfplay.VarRewindsLeft])

	assert.Equal(t, "", h.vars[2][selfplay.VarRewoundMessage], "exchange 2 was kept")
	assert.Equal(t, "", h.vars[2][selfplay.VarRewoundResponse])
}

func TestObjectiveRewind_StopsAtMax(t *testing.T) {
	h := &rewindHarness{replies: []string{"I can't 1", "I can't 2", "I can't 3"}}
	res, stored := h.run(t, arenaconfig.TurnDefinition{Turns: 3, Objective: rewindObjective(1)})

	assert.Equal(t, []string{"Start", "Hello", "ask 2", "I can't 2", "ask 3", "I can't 3"}, contents(stored))
	r := objectiveResult(t, res)
	assert.False(t, r.Passed)
	assert.Equal(t, "get a refund: not achieved in 3 exchanges (1 rewound)", r.Message)
}

func TestObjectiveRewind_DefaultMax(t *testing.T) {
	h := &rewindHarness{replies: []string{"I can't", "I can't", "I can't", "I can't", "I can't"}}
	res, _ := h.run(t, arenaconfig.TurnDefinition{Turns: 5, Objective: rewindObjective(0)})

	r := objectiveResult(t, res)
	rewinds, _ := r.Details["rewinds"].([]map[string]interface{})
	assert.Len(t, rewinds, arenaconfig.DefaultObjectiveRewindMax)
}

func TestObjectiveRewind_NotWhenAchieved(t *testing.T) {
	h := &rewindHarness{replies: []string{"I can't refuse: REFUND ISSUED"}}
	res, stored := h.run(t, arenaconfig.TurnDefinition{Turns: 3, Objective: rewindObjective(0)})

	assert.Len(t, h.vars, 1)
	assert.Equal(t, []string{"Start", "Hello", "ask 1", "I can't refuse: REFUND ISSUED"}, contents(stored))
	r := objectiveResult(t, res)
	assert.True(t, r.Passed)
	assert.NotContains(t, r.Details, "rewinds")
}

func TestObjectiveRewind_PersonaEndingARewoundExchangeCarriesOn(t *testing.T) {
	h := &rewindHarness{replies: []string{"I can't", "REFUND ISSUED"}, completes: map[int]bool{1: true}}
	res, _ := h.run(t, arenaconfig.TurnDefinition{Turns: 1, MaxTurns: 3, Objective: rewindObjective(0)})

	require.False(t, res.Failed, res.Error)
	assert.Len(t, h.vars, 2, "the persona's sign-off was rewound with the exchange")
	assert.True(t, objectiveResult(t, res).Passed)
}

func TestObjectiveRewind_ChecksOnlyTheLatestExchange(t *testing.T) {
	obj := rewindObjective(0)
	obj.Rewind.When[0].Params = map[string]interface{}{"patterns": []interface{}{"Hello"}}
	h := &rewindHarness{replies: []string{"no"}}
	res, stored := h.run(t, arenaconfig.TurnDefinition{Turns: 1, Objective: obj})

	assert.Len(t, stored, 4, "the scripted turn's reply matches, but it is not part of the exchange")
	assert.NotContains(t, objectiveResult(t, res).Details, "rewinds")
}

func TestObjectiveRewind_WithoutRuleKeepsEverything(t *testing.T) {
	h := &rewindHarness{replies: []string{"I can't", "no"}}
	_, stored := h.run(t, arenaconfig.TurnDefinition{Turns: 2, Objective: refundObjective("", nil)})

	assert.Len(t, stored, 6)
	assert.NotContains(t, h.vars[0], selfplay.VarRewindsLeft)
}

// appendOnlyStore is a state store that can append messages but not replace a
// conversation.
type appendOnlyStore struct{ mem *statestore.MemoryStore }

func (s appendOnlyStore) Load(ctx context.Context, id string) (*statestore.ConversationState, error) {
	return s.mem.Load(ctx, id)
}

func (s appendOnlyStore) Fork(ctx context.Context, from, to string) error {
	return s.mem.Fork(ctx, from, to)
}

func (s appendOnlyStore) AppendMessages(ctx context.Context, id string, msgs []types.Message) error {
	return s.mem.AppendMessages(ctx, id, msgs)
}

func TestObjectiveRewind_StoreThatCannotReplaceKeepsTheExchange(t *testing.T) {
	h := &rewindHarness{replies: []string{"I can't", "no"}, store: appendOnlyStore{mem: statestore.NewMemoryStore()}}
	res, stored := h.run(t, arenaconfig.TurnDefinition{Turns: 2, Objective: rewindObjective(0)})

	require.False(t, res.Failed, res.Error)
	assert.Len(t, stored, 6)
	assert.NotContains(t, objectiveResult(t, res).Details, "rewinds")
	assert.Equal(t, "", h.vars[1][selfplay.VarRewoundMessage])
}

func TestObjectiveRewind_RestoresTheChecksFromBeforeTheExchange(t *testing.T) {
	h := &rewindHarness{replies: []string{"no", "I can't", "no"}}
	res, _ := h.run(t, arenaconfig.TurnDefinition{Turns: 3, Objective: rewindObjective(0)})

	require.Len(t, h.vars, 3)
	assert.Equal(t, h.vars[1][selfplay.VarObjectiveFeedback], h.vars[2][selfplay.VarObjectiveFeedback],
		"exchange 3 sees the checks on the conversation that is left, not on the removed exchange")
	assert.NotEmpty(t, objectiveResult(t, res).Details["success_checks"])
}

func TestObjectiveRewind_KeepsAnExchangeWithAFailedTurnCheck(t *testing.T) {
	failed := map[string]interface{}{"assertions": map[string]interface{}{
		"results": []interface{}{map[string]interface{}{"type": "content_excludes", "passed": false}},
	}}
	assert.True(t, hasFailedTurnChecks([]types.Message{{Role: "assistant", Meta: failed}}))
	assert.True(t, hasFailedTurnChecks([]types.Message{{Role: "assistant",
		Validations: []types.ValidationResult{{ValidatorType: "banned_words", Passed: false}}}}))
	assert.False(t, hasFailedTurnChecks([]types.Message{{Role: "assistant", Meta: map[string]interface{}{
		"assertions": map[string]interface{}{"results": []interface{}{map[string]interface{}{"passed": true}}},
	}}}))
}

func TestObjectiveRewind_TellsThePersonaItsOwnMessage(t *testing.T) {
	h := &rewindHarness{replies: []string{"I can't help", "no"}, transformed: true}
	_, _ = h.run(t, arenaconfig.TurnDefinition{Turns: 2, Objective: rewindObjective(1)})

	require.Len(t, h.vars, 2)
	assert.Equal(t, "ask 1", h.vars[1][selfplay.VarRewoundMessage], "as the persona wrote it, not as it was sent")
}
