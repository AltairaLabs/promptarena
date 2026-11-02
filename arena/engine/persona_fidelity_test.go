package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"
	"github.com/AltairaLabs/promptarena/v2/arena/turnexecutors"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func TestSpecifics(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{"My order is A-100, placed 28 October.", []string{"A-100"}},
		{"Email jo.smith@example.com or call 0207 946 0018.", []string{"jo.smith@example.com", "0207", "946", "0018"}},
		{"Ticket #4471, at 10:30, ref 2024-10-28.", []string{"4471", "10:30", "2024-10-28"}},
		{"I have 2 kids and waited 15 minutes.", nil},
		{"Repeat: A-100 and a-100.", []string{"A-100"}},
		{"No specifics here at all.", nil},
		{"Order #4471 on my 2nd try, the 28th, 100% sure.", []string{"4471"}},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, specifics(tt.text), tt.text)
	}
}

// customerPersona holds an order number and an order date.
func customerPersona(policy *arenaconfig.PersonaFactPolicy) *arenaconfig.UserPersonaPack {
	return &arenaconfig.UserPersonaPack{
		ID:           "customer",
		SystemPrompt: "You want to know where your order is.",
		Goals:        []string{"Find out where order A-100 is"},
		Facts: []arenaconfig.PersonaFact{
			{ID: "order_number", Value: "A-100"},
			{ID: "order_date", Value: "28 October"},
		},
		FactPolicy: policy,
	}
}

func fidelityRegistry(t *testing.T, persona *arenaconfig.UserPersonaPack) *selfplay.Registry {
	t.Helper()
	providerReg := providers.NewRegistry()
	mock, err := providers.CreateProviderFromSpec(providers.ProviderSpec{ID: "mock-selfplay", Type: "mock", Model: "m"})
	require.NoError(t, err)
	providerReg.Register(mock)
	return selfplay.NewRegistry(providerReg, map[string]string{"attacker": "mock-selfplay"},
		map[string]*arenaconfig.UserPersonaPack{persona.ID: persona},
		[]arenaconfig.SelfPlayRoleGroup{{ID: "attacker", Provider: "mock-selfplay"}})
}

// runFidelity runs a scripted opening turn whose reply gives ticket T-555,
// then a self-play turn in which the persona says says[i] on exchange i+1.
func runFidelity(t *testing.T, persona *arenaconfig.UserPersonaPack, turn arenaconfig.TurnDefinition,
	says ...string) *ConversationResult {
	t.Helper()
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{
			{Role: "user", Content: "Where is my order?"},
			{Role: "assistant", Content: "I've opened ticket T-555 for you."},
		})
	}}
	n := 0
	selfPlay := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		n++
		return saveMessagesToStateStore(ctx, req, []types.Message{
			{Role: "user", Content: says[n-1]}, {Role: "assistant", Content: "Let me check."},
		})
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, fidelityRegistry(t, persona),
		createTestPromptRegistry(t), nil)
	executor.evalOrchestrator = NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")
	turn.Role = "attacker"
	turn.Persona = persona.ID
	if turn.Turns == 0 {
		turn.Turns = len(says)
	}
	return executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{ID: "fidelity", TaskType: "assistant", Turns: []arenaconfig.TurnDefinition{
			{Role: "user", Content: "Where is my order?"}, turn,
		}},
		Provider:         &MockProvider{id: "test"},
		Config:           &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"},
		ConversationID:   "fidelity-conv",
	})
}

func fidelityResult(t *testing.T, res *ConversationResult) asrt.ConversationValidationResult {
	t.Helper()
	for _, r := range res.ConversationAssertionResults {
		if r.Type == personaFidelityResultType {
			return r
		}
	}
	t.Fatalf("no persona fidelity result in %+v", res.ConversationAssertionResults)
	return asrt.ConversationValidationResult{}
}

func TestPersonaFidelity_DeclaredFactsAndEarlierSpecificsAreAllowed(t *testing.T) {
	res := runFidelity(t, customerPersona(nil), arenaconfig.TurnDefinition{},
		"My order A-100 from 28 October hasn't arrived.",
		"Is ticket T-555 still open?")

	require.False(t, res.Failed, res.Error)
	r := fidelityResult(t, res)
	assert.True(t, r.Passed)
	assert.Equal(t, true, r.Details["reliable"])
	assert.Equal(t, 2, r.Details["checked_messages"])
	assert.Equal(t, "persona customer stated only declared facts in 2 messages", r.Message)
	assert.Empty(t, UnreliableTurns(res.ConversationAssertionResults))
}

func TestPersonaFidelity_InventedSpecificFlagsTheRunWithoutFailingIt(t *testing.T) {
	res := runFidelity(t, customerPersona(nil), arenaconfig.TurnDefinition{},
		"My order is A-100.",
		"Actually it might be B-7731, and my email is jo@example.com.")

	require.False(t, res.Failed, res.Error)
	r := fidelityResult(t, res)
	assert.True(t, r.Passed, "by default an invented specific flags the run, it doesn't fail it")
	assert.Equal(t, false, r.Details["reliable"])
	assert.Equal(t, `persona customer invented 2 specifics: "jo@example.com" (exchange 2), "B-7731" (exchange 2)`,
		r.Message)
	violations, ok := r.Details["violations"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, violations, 1)
	assert.Equal(t, 2, violations[0]["exchange"])
	assert.Equal(t, map[int]bool{1: true}, UnreliableTurns(res.ConversationAssertionResults))
}

func TestPersonaFidelity_FailOnViolation(t *testing.T) {
	res := runFidelity(t, customerPersona(&arenaconfig.PersonaFactPolicy{FailOnViolation: true}),
		arenaconfig.TurnDefinition{}, "My order is Z-999.")

	r := fidelityResult(t, res)
	assert.False(t, r.Passed)
	assert.Equal(t, `persona customer invented 1 specific: "Z-999" (exchange 1)`, r.Message)
}

func TestPersonaFidelity_CheckOffOrNoFactsAddsNoResult(t *testing.T) {
	off := false
	for name, persona := range map[string]*arenaconfig.UserPersonaPack{
		"check off": customerPersona(&arenaconfig.PersonaFactPolicy{Check: &off}),
		"no facts":  {ID: "customer", SystemPrompt: "You are a customer."},
	} {
		t.Run(name, func(t *testing.T) {
			res := runFidelity(t, persona, arenaconfig.TurnDefinition{}, "My order is Z-999.")
			for _, r := range res.ConversationAssertionResults {
				assert.NotEqual(t, personaFidelityResultType, r.Type)
			}
		})
	}
}

func TestPersonaFidelity_RewoundExchangeIsStillChecked(t *testing.T) {
	obj := refundObjective("", nil)
	obj.Rewind = &arenaconfig.ObjectiveRewindConfig{When: []arenaconfig.AssertionConfig{{
		Type: "content_includes_any", Params: map[string]interface{}{"patterns": []interface{}{"Let me check"}},
	}}, Max: 1}
	res := runFidelity(t, customerPersona(nil), arenaconfig.TurnDefinition{Turns: 2, Objective: obj},
		"My order is Q-123.", "My order is A-100.")

	r := fidelityResult(t, res)
	assert.Equal(t, false, r.Details["reliable"], "the rewound exchange's invented specific is kept")
	rewinds, _ := objectiveResult(t, res).Details["rewinds"].([]map[string]interface{})
	assert.Len(t, rewinds, 1)
}

func TestUnreliableTurns_IgnoresOtherResults(t *testing.T) {
	assert.Empty(t, UnreliableTurns([]asrt.ConversationValidationResult{
		{Type: "content_includes", Details: map[string]interface{}{"reliable": false}},
		{Type: personaFidelityResultType, Details: map[string]interface{}{}},
	}))
	assert.Equal(t, map[int]bool{3: true}, UnreliableTurns([]asrt.ConversationValidationResult{
		{Type: personaFidelityResultType, Details: map[string]interface{}{"reliable": false, "turn_index": float64(3)}},
	}), "turn_index survives a JSON round trip")
}

func TestPersonaFidelity_ObjectiveAndPromptTextAreGiven(t *testing.T) {
	persona := customerPersona(nil)
	persona.SystemPrompt = "Your loyalty number is LN-42."
	obj := refundObjective("", nil)
	obj.Description = "Get a refund for order R-2020"
	res := runFidelity(t, persona, arenaconfig.TurnDefinition{Objective: obj},
		"Refund R-2020 please, my loyalty number is LN-42.")

	assert.Equal(t, true, fidelityResult(t, res).Details["reliable"])
}

func TestPersonaFidelity_RewoundReplyCanBeRepeated(t *testing.T) {
	obj := refundObjective("", nil)
	obj.Rewind = &arenaconfig.ObjectiveRewindConfig{When: []arenaconfig.AssertionConfig{{
		Type: "content_includes_any", Params: map[string]interface{}{"patterns": []interface{}{"Let me check"}},
	}}, Max: 1}
	persona := customerPersona(nil)
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "Hi"}, {Role: "assistant", Content: "Hello"}})
	}}
	n := 0
	selfPlay := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		n++
		says := []string{"Where is A-100?", "About ticket K-8080, please."}[n-1]
		reply := []string{"Let me check, ticket K-8080.", "Done."}[n-1]
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: says}, {Role: "assistant", Content: reply}})
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, fidelityRegistry(t, persona), createTestPromptRegistry(t), nil)
	executor.evalOrchestrator = NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")
	res := executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{ID: "f", TaskType: "assistant", Turns: []arenaconfig.TurnDefinition{
			{Role: "user", Content: "Hi"}, {Role: "attacker", Persona: persona.ID, Turns: 2, Objective: obj},
		}},
		Provider: &MockProvider{id: "test"}, Config: &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"}, ConversationID: "f",
	})

	rewinds, _ := objectiveResult(t, res).Details["rewinds"].([]map[string]interface{})
	require.Len(t, rewinds, 1, "exchange 1, which gave K-8080, was rewound")
	assert.Equal(t, true, fidelityResult(t, res).Details["reliable"],
		"the persona was handed K-8080 as {{rewound_response}}, so repeating it is not inventing")
}

func TestPersonaFidelity_MessageTheAgentFailedOnIsChecked(t *testing.T) {
	persona := customerPersona(nil)
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "Hi"}, {Role: "assistant", Content: "Hello"}})
	}}
	selfPlay := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		if err := saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "My order is Z-9999."}}); err != nil {
			return err
		}
		return errors.New("validation failed")
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, fidelityRegistry(t, persona), createTestPromptRegistry(t), nil)
	res := executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{ID: "f", TaskType: "assistant", Turns: []arenaconfig.TurnDefinition{
			{Role: "user", Content: "Hi"}, {Role: "attacker", Persona: persona.ID, Turns: 2},
		}},
		Provider: &MockProvider{id: "test"}, Config: &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"}, ConversationID: "f",
	})

	require.True(t, res.Failed)
	r := fidelityResult(t, res)
	assert.Equal(t, false, r.Details["reliable"])
	assert.Equal(t, 1, r.Details["checked_messages"])
}

func TestPersonaFidelity_PersonaGenerationFailureChecksNothingTwice(t *testing.T) {
	persona := customerPersona(nil)
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "Hi"}, {Role: "assistant", Content: "Hello"}})
	}}
	n := 0
	selfPlay := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		n++
		if n == 2 {
			return errors.New("persona provider down")
		}
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "Order Z-1"}, {Role: "assistant", Content: "ok"}})
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, fidelityRegistry(t, persona), createTestPromptRegistry(t), nil)
	res := executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{ID: "f", TaskType: "assistant", Turns: []arenaconfig.TurnDefinition{
			{Role: "user", Content: "Hi"}, {Role: "attacker", Persona: persona.ID, Turns: 3},
		}},
		Provider: &MockProvider{id: "test"}, Config: &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"}, ConversationID: "f",
	})

	assert.Equal(t, 1, fidelityResult(t, res).Details["checked_messages"])
}
