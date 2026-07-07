package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/pipeline"
	"github.com/AltairaLabs/PromptKit/runtime/tools"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

// fakeProviderErr models a runtime provider error carrying typed
// classification (status code + auth/retryable), like gemini.APIError.
type fakeProviderErr struct {
	msg       string
	auth      bool
	retryable bool
	status    int
}

func (e *fakeProviderErr) Error() string     { return e.msg }
func (e *fakeProviderErr) IsAuthError() bool { return e.auth }
func (e *fakeProviderErr) IsRetryable() bool { return e.retryable }
func (e *fakeProviderErr) StatusCode() int   { return e.status }

func TestIsRecoverableError(t *testing.T) {
	// nil and context death are never retried.
	assert.False(t, isRecoverableError(nil))
	assert.False(t, isRecoverableError(context.Canceled))
	assert.False(t, isRecoverableError(context.DeadlineExceeded))

	// A 401 is a 401 — terminal, even when wrapped in the error chain.
	authErr := &fakeProviderErr{msg: "authentication failed", auth: true, status: 401}
	assert.False(t, isRecoverableError(authErr))
	assert.False(t, isRecoverableError(fmt.Errorf("duplex conversation failed: %w", authErr)))

	// Other 4xx client errors are terminal too.
	assert.False(t, isRecoverableError(&fakeProviderErr{msg: "bad request", status: 400}))

	// Provider-declared transient errors (429, 5xx) retry.
	assert.True(t, isRecoverableError(&fakeProviderErr{msg: "rate limited", retryable: true, status: 429}))
	assert.True(t, isRecoverableError(&fakeProviderErr{msg: "unavailable", retryable: true, status: 503}))

	// Untyped errors are NOT retried — no more blind substring matching (a
	// message merely containing "websocket" used to loop a 401 forever).
	assert.False(t, isRecoverableError(errors.New("websocket read failed")))
}

func TestSelfplayHistoryView(t *testing.T) {
	t.Run("nil for empty input", func(t *testing.T) {
		assert.Nil(t, selfplayHistoryView(nil))
	})

	t.Run("keeps only non-empty user and assistant turns", func(t *testing.T) {
		in := []types.Message{
			{Role: "system", Content: "big system prompt"},
			{Role: "user", Content: "  hello  "},
			{Role: "assistant", Content: "hi"},
			{Role: "assistant", Content: "   "},
			{Role: "tool", Content: "tool output"},
		}
		out := selfplayHistoryView(in)
		require.Len(t, out, 2)
		assert.Equal(t, "user", out[0].Role)
		assert.Equal(t, "hello", out[0].Content, "content should be trimmed")
		assert.Equal(t, "assistant", out[1].Role)
		assert.Equal(t, "hi", out[1].Content)
	})
}

func TestConvertToolDefinition(t *testing.T) {
	de := &DuplexConversationExecutor{}

	t.Run("parses valid input schema", func(t *testing.T) {
		td := &tools.ToolDescriptor{
			Name:        "t1",
			Description: "desc",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}
		got := de.convertToolDefinition(td)
		assert.Equal(t, "t1", got.Name)
		assert.Equal(t, "desc", got.Description)
		assert.Equal(t, "object", got.Parameters["type"])
	})

	t.Run("nil schema yields nil params", func(t *testing.T) {
		got := de.convertToolDefinition(&tools.ToolDescriptor{Name: "t2"})
		assert.Nil(t, got.Parameters)
	})

	t.Run("invalid schema is swallowed to nil params", func(t *testing.T) {
		td := &tools.ToolDescriptor{Name: "t3", InputSchema: json.RawMessage(`{not json`)}
		got := de.convertToolDefinition(td)
		assert.Nil(t, got.Parameters)
	})
}

func TestSilenceTailDuration(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		t.Setenv(silenceTailDurationEnv, "")
		assert.Equal(t, defaultSilenceTailDuration, silenceTailDuration())
	})

	t.Run("env override applied", func(t *testing.T) {
		t.Setenv(silenceTailDurationEnv, "250")
		assert.Equal(t, 250*time.Millisecond, silenceTailDuration())
	})

	t.Run("unparseable env falls back to default", func(t *testing.T) {
		t.Setenv(silenceTailDurationEnv, "abc")
		assert.Equal(t, defaultSilenceTailDuration, silenceTailDuration())
	})
}

func TestStampSelfplayDevMeta(t *testing.T) {
	t.Run("nil turnMeta is a no-op", func(t *testing.T) {
		assert.NotPanics(t, func() {
			stampSelfplayDevMeta(nil, nil, "p", nil)
		})
	})

	t.Run("copies system prompt from result metadata", func(t *testing.T) {
		turnMeta := map[string]any{}
		stampSelfplayDevMeta(turnMeta, nil, "", map[string]interface{}{"system_prompt": "you are X"})
		assert.Equal(t, "you are X", turnMeta["_selfplay_prompt"])
	})

	t.Run("nil registry skips persona yaml", func(t *testing.T) {
		turnMeta := map[string]any{}
		stampSelfplayDevMeta(turnMeta, nil, "persona", nil)
		assert.NotContains(t, turnMeta, "_persona_yaml")
	})
}

func TestLastAssistantMessage(t *testing.T) {
	t.Run("nil when none", func(t *testing.T) {
		assert.Nil(t, lastAssistantMessage([]types.Message{{Role: "user"}}))
	})

	t.Run("returns most recent assistant", func(t *testing.T) {
		msgs := []types.Message{
			{Role: "assistant", Content: "first"},
			{Role: "user", Content: "q"},
			{Role: "assistant", Content: "second"},
		}
		got := lastAssistantMessage(msgs)
		require.NotNil(t, got)
		assert.Equal(t, "second", got.Content)
	})
}

func TestTurnAssertionConfigs(t *testing.T) {
	defs := []arenaconfig.AssertionConfig{
		{Type: "contains", Message: "must contain"},
		{Type: "latency_budget"},
	}
	got := turnAssertionConfigs(defs)
	require.Len(t, got, 2)
	assert.Equal(t, "contains", got[0].Type)
	assert.Equal(t, "must contain", got[0].Message)
	assert.Equal(t, "latency_budget", got[1].Type)
}

func TestBuildSelfplayTurnMeta(t *testing.T) {
	de := &DuplexConversationExecutor{}
	turn := &arenaconfig.TurnDefinition{Role: "user", Persona: "shopper"}

	t.Run("base metadata always present", func(t *testing.T) {
		res := &pipeline.ExecutionResult{}
		meta := de.buildSelfplayTurnMeta(turn, 2, res)
		assert.Equal(t, true, meta["self_play"])
		assert.Equal(t, "shopper", meta["persona"])
		assert.Equal(t, 2, meta["selfplay_turn_index"])
		assert.NotContains(t, meta, "self_play_cost", "no cost when tokens are zero")
	})

	t.Run("copies relevant fields and cost", func(t *testing.T) {
		res := &pipeline.ExecutionResult{
			Metadata: map[string]interface{}{
				"self_play_provider": "openai",
				"warning_type":       "truncated",
				"system_prompt":      "ignored internal field is not copied verbatim",
			},
			CostInfo: types.CostInfo{InputTokens: 10, OutputTokens: 5, TotalCost: 0.01},
		}
		meta := de.buildSelfplayTurnMeta(turn, 1, res)
		assert.Equal(t, "openai", meta["self_play_provider"])
		assert.Equal(t, "truncated", meta["warning_type"])
		cost, ok := meta["self_play_cost"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, 10, cost["input_tokens"])
	})
}
