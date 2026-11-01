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
