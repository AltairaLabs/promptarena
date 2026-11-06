package turnexecutors

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

// countingEmbedder records the texts of every Embed call.
type countingEmbedder struct {
	mu    sync.Mutex
	calls [][]string
}

func (e *countingEmbedder) Embed(_ context.Context, req providers.EmbeddingRequest) (providers.EmbeddingResponse, error) {
	e.mu.Lock()
	e.calls = append(e.calls, append([]string(nil), req.Texts...))
	e.mu.Unlock()
	out := make([][]float32, len(req.Texts))
	for i, text := range req.Texts {
		out[i] = []float32{float32(len(text)%7) + 1, float32(strings.Count(text, "a")) + 1, 1}
	}
	return providers.EmbeddingResponse{Embeddings: out}, nil
}

func (e *countingEmbedder) EmbeddingDimensions() int { return 3 }
func (e *countingEmbedder) MaxBatchSize() int        { return 100 }
func (e *countingEmbedder) ID() string               { return "counting" }

func (e *countingEmbedder) embedded() [][]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([][]string(nil), e.calls...)
}

// useCountingEmbedder makes relevance policies embed with one counting
// provider, and reports how many times a provider was built.
func useCountingEmbedder(t *testing.T) (*countingEmbedder, *int) {
	t.Helper()
	emb := &countingEmbedder{}
	built := 0
	orig := newEmbeddingProvider
	newEmbeddingProvider = func(string, string) (providers.EmbeddingProvider, error) {
		built++
		return emb, nil
	}
	t.Cleanup(func() { newEmbeddingProvider = orig })
	return emb, &built
}

func relevanceScenario(cache bool) *arenaconfig.Scenario {
	return &arenaconfig.Scenario{
		ID: "relevance",
		ContextPolicy: &arenaconfig.ContextPolicy{
			TokenBudget: 60,
			Strategy:    "relevance",
			Relevance: &arenaconfig.RelevanceConfig{
				Provider:          "counting",
				MinRecentMessages: 1,
				CacheEmbeddings:   cache,
			},
		},
	}
}

// history returns n alternating user/assistant messages long enough that a
// handful exceed the scenario's 60-token budget.
func history(n int) []types.Message {
	msgs := make([]types.Message, 0, n)
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs = append(msgs, types.Message{
			Role:    role,
			Content: fmt.Sprintf("message %d talks at some length about topic %d so that it costs tokens", i, i),
		})
	}
	return msgs
}

// runTurn runs one turn's messages through s and returns what it emitted.
func runTurn(t *testing.T, s stage.Stage, msgs []types.Message) ([]stage.StreamElement, error) {
	t.Helper()
	input := make(chan stage.StreamElement, len(msgs))
	output := make(chan stage.StreamElement, len(msgs))
	for i := range msgs {
		input <- stage.StreamElement{Message: &msgs[i], Meta: stage.ElementMetadata{FromHistory: i < len(msgs)-1}}
	}
	close(input)
	err := s.Process(context.Background(), input, output)
	var out []stage.StreamElement
	for elem := range output {
		out = append(out, elem)
	}
	return out, err
}

func contains(calls [][]string, text string) bool {
	for _, call := range calls {
		for _, got := range call {
			if got == text {
				return true
			}
		}
	}
	return false
}

func TestConversationContextBuilder_SecondTurnHitsEmbeddingCache(t *testing.T) {
	emb, built := useCountingEmbedder(t)
	scenario := relevanceScenario(true)
	cb := NewConversationContextBuilder()
	req := &TurnRequest{Scenario: scenario, ContextBuilder: cb}

	turn1 := history(6)
	out, err := runTurn(t, contextBuilderStageFor(req, stage.NewTurnState()), turn1)
	require.NoError(t, err)
	require.NotEmpty(t, out)
	require.Len(t, emb.embedded(), 1, "turn 1 embeds once")

	turn2 := append(history(8), types.Message{Role: "user", Content: "a brand new question"})
	_, err = runTurn(t, contextBuilderStageFor(req, stage.NewTurnState()), turn2)
	require.NoError(t, err)

	calls := emb.embedded()
	require.Len(t, calls, 2)
	second := calls[1]
	assert.NotContains(t, second, turn1[0].Content, "turn 1's messages come from the cache")
	assert.Contains(t, second, "a brand new question", "the new message is embedded")
	assert.Less(t, len(second), len(calls[0]), "turn 2 embeds only what turn 1 did not")
	assert.Equal(t, 1, *built, "one embedding provider for the conversation")
}

func TestConversationContextBuilder_WithoutCachingReembeds(t *testing.T) {
	emb, _ := useCountingEmbedder(t)
	req := &TurnRequest{Scenario: relevanceScenario(false), ContextBuilder: NewConversationContextBuilder()}

	for i := 0; i < 2; i++ {
		_, err := runTurn(t, contextBuilderStageFor(req, stage.NewTurnState()), history(6))
		require.NoError(t, err)
	}
	calls := emb.embedded()
	require.Len(t, calls, 2)
	assert.True(t, contains(calls[1:], history(1)[0].Content), "cache_embeddings off re-embeds every turn")
}

func TestContextBuilderStageFor_PerTurnWithoutConversationBuilder(t *testing.T) {
	emb, built := useCountingEmbedder(t)
	req := &TurnRequest{Scenario: relevanceScenario(true)}

	for i := 0; i < 2; i++ {
		_, err := runTurn(t, contextBuilderStageFor(req, stage.NewTurnState()), history(6))
		require.NoError(t, err)
	}
	assert.Equal(t, 2, *built, "a stage per turn builds a provider per turn")
	assert.True(t, contains(emb.embedded()[1:], history(1)[0].Content), "and starts with an empty cache")
}

func TestConversationContextBuilder_ReadsEachTurnsSystemPrompt(t *testing.T) {
	useCountingEmbedder(t)
	req := &TurnRequest{Scenario: relevanceScenario(true), ContextBuilder: NewConversationContextBuilder()}

	_, err := runTurn(t, contextBuilderStageFor(req, stage.NewTurnState()), history(2))
	require.NoError(t, err)

	// A system prompt bigger than the whole budget must reach the reused
	// stage's budget accounting on this turn.
	ts := stage.NewTurnState()
	ts.SystemPrompt = strings.Repeat("system prompt words ", 100)
	_, err = runTurn(t, contextBuilderStageFor(req, ts), history(2))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token budget too small")

	_, err = runTurn(t, contextBuilderStageFor(req, stage.NewTurnState()), history(2))
	require.NoError(t, err, "the next turn's prompt replaces it")
}

func TestContextBuilderStageFor_NoPolicy(t *testing.T) {
	scenario := &arenaconfig.Scenario{ID: "plain"}
	assert.Nil(t, contextBuilderStageFor(&TurnRequest{Scenario: scenario}, stage.NewTurnState()))
	assert.Nil(t, contextBuilderStageFor(
		&TurnRequest{Scenario: scenario, ContextBuilder: NewConversationContextBuilder()}, stage.NewTurnState()))
}

func TestConversationContextStage_Cancelled(t *testing.T) {
	useCountingEmbedder(t)
	req := &TurnRequest{Scenario: relevanceScenario(true), ContextBuilder: NewConversationContextBuilder()}
	s := contextBuilderStageFor(req, stage.NewTurnState())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	msgs := history(3)
	input := make(chan stage.StreamElement, len(msgs))
	for i := range msgs {
		input <- stage.StreamElement{Message: &msgs[i]}
	}
	close(input)
	output := make(chan stage.StreamElement) // unbuffered and never read
	err := s.Process(ctx, input, output)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, input, "input is drained so upstream is not blocked")
}
