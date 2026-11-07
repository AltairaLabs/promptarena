package selfplay

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// capturingProvider records every request that reaches the provider, so tests
// can check what NextUserTurn actually sends.
type capturingProvider struct {
	*mock.Provider
	mu       sync.Mutex
	requests []providers.PredictionRequest
}

func newCapturingProvider(reply string) *capturingProvider {
	repo := mock.NewInMemoryMockRepository(reply)
	return &capturingProvider{Provider: mock.NewProviderWithRepository("capture", "capture-model", false, repo)}
}

func (c *capturingProvider) Predict(ctx context.Context, req providers.PredictionRequest) (providers.PredictionResponse, error) {
	c.record(req)
	return c.Provider.Predict(ctx, req)
}

func (c *capturingProvider) PredictStream(ctx context.Context, req providers.PredictionRequest) (<-chan providers.StreamChunk, error) {
	c.record(req)
	return c.Provider.PredictStream(ctx, req)
}

func (c *capturingProvider) record(req providers.PredictionRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
}

func (c *capturingProvider) lastRequest(t *testing.T) providers.PredictionRequest {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.NotEmpty(t, c.requests, "provider was never called")
	return c.requests[len(c.requests)-1]
}

func float32Ptr(v float32) *float32 { return &v }
func intPtr(v int) *int             { return &v }

func overridesTestPersona() *arenaconfig.UserPersonaPack {
	return &arenaconfig.UserPersonaPack{
		ID:           "tester",
		SystemPrompt: "You are a test user.",
		Defaults:     arenaconfig.PersonaDefaults{Temperature: float32Ptr(0.7)},
	}
}

func TestNextUserTurn_SendsPersonaDefaultsWhenNoOverrides(t *testing.T) {
	prov := newCapturingProvider("Hello.")
	gen := NewContentGenerator(prov, overridesTestPersona())

	_, err := gen.NextUserTurn(context.Background(), nil, "s1", nil)
	require.NoError(t, err)

	req := prov.lastRequest(t)
	assert.InDelta(t, 0.7, req.Temperature, 1e-6)
	assert.Equal(t, defaultUserTurnMaxTokens, req.MaxTokens)
	assert.Nil(t, req.Seed, "no seed configured, so none is sent")
}

func TestNextUserTurn_Seed(t *testing.T) {
	tests := []struct {
		name        string
		personaSeed *int
		turnSeed    *int
		want        *int
	}{
		{name: "persona seed is sent", personaSeed: intPtr(42), want: intPtr(42)},
		{name: "turn seed beats persona seed", personaSeed: intPtr(42), turnSeed: intPtr(7), want: intPtr(7)},
		{name: "persona seed zero sends none", personaSeed: intPtr(0)},
		{name: "nothing set sends none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			persona := overridesTestPersona()
			persona.Defaults.Seed = tt.personaSeed
			prov := newCapturingProvider("Hello.")
			gen := NewContentGenerator(prov, persona)

			_, err := gen.NextUserTurn(context.Background(), nil, "s1", &GeneratorOptions{Seed: tt.turnSeed})
			require.NoError(t, err)
			assert.Equal(t, tt.want, prov.lastRequest(t).Seed)
		})
	}
}

func TestNextUserTurn_TurnOverridesReachProvider(t *testing.T) {
	prov := newCapturingProvider("Hello.")
	gen := NewContentGenerator(prov, overridesTestPersona())

	_, err := gen.NextUserTurn(context.Background(), nil, "s1", &GeneratorOptions{
		Temperature: float32Ptr(1.1),
		Seed:        intPtr(7),
	})
	require.NoError(t, err)

	req := prov.lastRequest(t)
	assert.InDelta(t, 1.1, req.Temperature, 1e-6)
	require.NotNil(t, req.Seed)
	assert.Equal(t, 7, *req.Seed)
}

func TestNextUserTurn_MaxTokens(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens *int
		natural   bool
		want      int
	}{
		{name: "default", want: defaultUserTurnMaxTokens},
		{name: "default with natural termination", natural: true, want: defaultUserTurnMaxTokens + completionMarkerTokens},
		{name: "persona cap", maxTokens: intPtr(1500), want: 1500},
		{name: "persona cap is exact with natural termination", maxTokens: intPtr(1500), natural: true, want: 1500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			persona := overridesTestPersona()
			persona.Defaults.MaxTokens = tt.maxTokens
			prov := newCapturingProvider("Hello.")
			gen := NewContentGenerator(prov, persona)

			_, err := gen.NextUserTurn(context.Background(), nil, "s1", &GeneratorOptions{NaturalTerminationEnabled: tt.natural})
			require.NoError(t, err)
			assert.Equal(t, tt.want, prov.lastRequest(t).MaxTokens)
		})
	}
}

func TestNextUserTurn_AdversarialModeSkipsUserChecks(t *testing.T) {
	long := strings.Repeat("This is a sentence. ", 6) + "Step 1: do this. Step 2: do that."

	t.Run("user mode warns", func(t *testing.T) {
		gen := NewContentGenerator(newCapturingProvider(long), overridesTestPersona())
		res, err := gen.NextUserTurn(context.Background(), nil, "s1", nil)
		require.NoError(t, err)
		assert.Contains(t, res.Metadata, "validation_warning")
	})

	t.Run("adversarial mode does not", func(t *testing.T) {
		persona := overridesTestPersona()
		persona.Mode = arenaconfig.PersonaModeAdversarial
		gen := NewContentGenerator(newCapturingProvider(long), persona)
		res, err := gen.NextUserTurn(context.Background(), nil, "s1", nil)
		require.NoError(t, err)
		assert.NotContains(t, res.Metadata, "validation_warning")
		assert.Equal(t, long, res.Response.Content)
	})
}

// scriptedRepo answers the i-th request with replies[i], repeating the last.
type scriptedRepo struct {
	*mock.InMemoryMockRepository
	mu      sync.Mutex
	n       int
	replies []string
}

func (r *scriptedRepo) next() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	r.SetResponse("", -1, r.replies[min(r.n, len(r.replies))-1])
}

func (r *scriptedRepo) GetResponse(ctx context.Context, p mock.ResponseParams) (string, error) {
	r.next()
	return r.InMemoryMockRepository.GetResponse(ctx, p)
}

func (r *scriptedRepo) GetTurn(ctx context.Context, p mock.ResponseParams) (*mock.Turn, error) {
	r.next()
	return r.InMemoryMockRepository.GetTurn(ctx, p)
}

// scriptedProvider records each request and answers them in turn from
// replies.
type scriptedProvider struct{ capturingProvider }

func newScriptedProvider(replies ...string) *scriptedProvider {
	repo := &scriptedRepo{InMemoryMockRepository: mock.NewInMemoryMockRepository(""), replies: replies}
	return &scriptedProvider{capturingProvider{
		Provider: mock.NewProviderWithRepository("capture", "capture-model", false, repo),
	}}
}

func (s *scriptedProvider) calls() []providers.PredictionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]providers.PredictionRequest(nil), s.requests...)
}

func regeneratingPersona(onViolation string) *arenaconfig.UserPersonaPack {
	return &arenaconfig.UserPersonaPack{
		ID:           "customer",
		SystemPrompt: "You want to know where your order is.",
		Facts:        []arenaconfig.PersonaFact{{ID: "order_number", Value: "A-100"}},
		FactPolicy:   &arenaconfig.PersonaFactPolicy{OnViolation: onViolation},
	}
}

func TestNextUserTurn_RegeneratesAnInventedSpecific(t *testing.T) {
	prov := newScriptedProvider("My order is B-7731.", "My order is A-100.")
	gen := NewContentGenerator(prov, regeneratingPersona(arenaconfig.FactViolationRegenerate))

	res, err := gen.NextUserTurn(context.Background(), nil, "s1", nil)
	require.NoError(t, err)

	assert.Equal(t, "My order is A-100.", res.Response.Content)
	assert.Equal(t, []string{"B-7731"}, res.Metadata[MetaFidelityRepaired])
	calls := prov.calls()
	require.Len(t, calls, 2)
	assert.NotContains(t, calls[0].System, "previous draft")
	assert.Contains(t, calls[1].System, `Your previous draft of this message stated "B-7731"`)
	assert.Greater(t, res.CostInfo.InputTokens, 0)
}

func TestNextUserTurn_RegenerateAcceptsTheRetry(t *testing.T) {
	prov := newScriptedProvider("My order is B-7731.", "Maybe C-2002?")
	gen := NewContentGenerator(prov, regeneratingPersona(arenaconfig.FactViolationRegenerate))

	res, err := gen.NextUserTurn(context.Background(), nil, "s1", nil)
	require.NoError(t, err)
	assert.Equal(t, "Maybe C-2002?", res.Response.Content, "the retry is sent even if it invents too")
	assert.Len(t, prov.calls(), 2, "only one retry")
}

func TestNextUserTurn_NoRegenerationWhenGivenOrFlagging(t *testing.T) {
	history := []types.Message{{Role: "assistant", Content: "Your ticket is T-555."}}
	for name, tc := range map[string]struct {
		policy, reply string
	}{
		"fact":           {arenaconfig.FactViolationRegenerate, "Order A-100, ticket T-555."},
		"flag policy":    {arenaconfig.FactViolationFlag, "Order B-7731."},
		"default policy": {"", "Order B-7731."},
	} {
		t.Run(name, func(t *testing.T) {
			prov := newScriptedProvider(tc.reply)
			gen := NewContentGenerator(prov, regeneratingPersona(tc.policy))
			res, err := gen.NextUserTurn(context.Background(), history, "s1", nil)
			require.NoError(t, err)
			assert.Len(t, prov.calls(), 1)
			assert.NotContains(t, res.Metadata, MetaFidelityRepaired)
		})
	}
}

func TestNextUserTurn_RegenerateSeesTemplateVariables(t *testing.T) {
	prov := newScriptedProvider("I want a refund for R-42X.")
	gen := NewContentGenerator(prov, regeneratingPersona(arenaconfig.FactViolationRegenerate))
	_, err := gen.NextUserTurn(context.Background(), nil, "s1",
		&GeneratorOptions{Variables: &map[string]string{VarObjective: "refund R-42X"}})
	require.NoError(t, err)
	assert.Len(t, prov.calls(), 1, "the objective gave the persona R-42X")
}

func TestNextUserTurn_FactsWithNaturalTermination(t *testing.T) {
	prov := newScriptedProvider("Where is A-100?")
	gen := NewContentGenerator(prov, regeneratingPersona(""))
	_, err := gen.NextUserTurn(context.Background(), nil, "s1", &GeneratorOptions{NaturalTerminationEnabled: true})
	require.NoError(t, err, "the facts rule and the completion instruction share one stage")
	system := prov.lastRequest(t).System
	assert.Contains(t, system, "Never make up any other specific")
	assert.Contains(t, system, strings.TrimSpace(CompletionInstruction)[:20])
}

func TestNextUserTurn_RegenerateUsesTheNextSeedAndKeepsThePrompt(t *testing.T) {
	prov := newScriptedProvider("My order is B-7731.", "My order is A-100.")
	persona := regeneratingPersona(arenaconfig.FactViolationRegenerate)
	persona.Defaults.Seed = intPtr(7)
	res, err := NewContentGenerator(prov, persona).NextUserTurn(context.Background(), nil, "s1", nil)
	require.NoError(t, err)
	calls := prov.calls()
	require.Len(t, calls, 2)
	assert.Equal(t, 7, *calls[0].Seed)
	assert.Equal(t, 8, *calls[1].Seed)
	assert.NotContains(t, res.Metadata["system_prompt"], "previous draft",
		"the dev panel shows the persona's prompt, not the retry note")
}

func TestNextUserTurn_PrivateNotesAreNotGiven(t *testing.T) {
	prov := newScriptedProvider("Maybe C-9921?", "My order is A-100.")
	persona := regeneratingPersona(arenaconfig.FactViolationRegenerate)
	persona.PrivateNotes = true
	history := []types.Message{{Role: "user", Content: "Hi", Meta: map[string]interface{}{
		"persona": "customer", MetaPersonaNotes: "try C-9921",
	}}}
	res, err := NewContentGenerator(prov, persona).NextUserTurn(context.Background(), history, "s1", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"C-9921"}, res.Metadata[MetaFidelityRepaired], "the persona's own notes don't count as given")
}
