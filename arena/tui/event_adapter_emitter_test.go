package tui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/tui/logging"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
)

// emitThroughBus drives one event through the real PromptKit emitter and
// event bus into a subscribed EventAdapter, and returns the TUI message the
// adapter delivered. Going through events.Emitter matters: it decides the
// payload shape (pointer vs value) the adapter receives, and the adapter's
// hand-built-event tests cannot see a mismatch there.
func emitThroughBus(t *testing.T, emit func(e *events.Emitter)) tea.Msg {
	t.Helper()

	bus := events.NewEventBus()
	t.Cleanup(bus.Close)

	got := make(chan tea.Msg, 4)
	NewEventAdapter(func(m tea.Msg) { got <- m }).Subscribe(bus)

	emit(events.NewEmitter(bus, "run-1", "session-1", "conv-1"))

	select {
	case m := <-got:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("the adapter delivered no TUI message for the emitted event")
		return nil
	}
}

// TestEventAdapter_EmitterEventsReachTheTUI pins the log line each runtime
// event becomes when it arrives the way production sends it. The emitter
// publishes every payload as a pointer; a handler that type-asserts only the
// value shape returns nil and the event silently never reaches the TUI.
func TestEventAdapter_EmitterEventsReachTheTUI(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	cases := []struct {
		name    string
		emit    func(e *events.Emitter)
		level   string
		message string
	}{
		{
			name:    "tool call started",
			emit:    func(e *events.Emitter) { e.ToolCallStarted("search", "call-1", nil, nil) },
			level:   "INFO",
			message: "Tool call started: search",
		},
		{
			name: "tool call completed",
			emit: func(e *events.Emitter) {
				e.ToolCallCompleted("search", "call-1", time.Second, "ok", nil, nil)
			},
			level:   "INFO",
			message: "Tool call completed: search",
		},
		{
			name:    "tool call failed carries its error",
			emit:    func(e *events.Emitter) { e.ToolCallFailed("search", "call-1", boom, time.Second, nil) },
			level:   "ERROR",
			message: "Tool call failed: search (boom)",
		},
		{
			name:    "validation started",
			emit:    func(e *events.Emitter) { e.ValidationStarted("schema", "json") },
			level:   "INFO",
			message: "Validation started: schema",
		},
		{
			name:    "validation passed",
			emit:    func(e *events.Emitter) { e.ValidationPassed("schema", "json", time.Second) },
			level:   "INFO",
			message: "Validation passed: schema",
		},
		{
			name:    "validation failed carries its error",
			emit:    func(e *events.Emitter) { e.ValidationFailed("schema", "json", boom, time.Second, nil) },
			level:   "ERROR",
			message: "Validation failed: schema (boom)",
		},
		{
			name:    "context built",
			emit:    func(e *events.Emitter) { e.ContextBuilt(5, 120, 1000, true) },
			level:   "INFO",
			message: "Context built (5 msgs, 120 tokens) [truncated]",
		},
		{
			name:    "token budget exceeded",
			emit:    func(e *events.Emitter) { e.TokenBudgetExceeded(1500, 1000, 500) },
			level:   "ERROR",
			message: "Token budget exceeded: need 1500, budget 1000, excess 500",
		},
		{
			name:    "state loaded",
			emit:    func(e *events.Emitter) { e.StateLoaded("conv-1", 3) },
			level:   "INFO",
			message: "State loaded: conv-1 (3 messages)",
		},
		{
			name:    "state saved",
			emit:    func(e *events.Emitter) { e.StateSaved("conv-1", 4) },
			level:   "INFO",
			message: "State saved: conv-1 (4 messages)",
		},
		{
			name:    "stream interrupted carries its reason",
			emit:    func(e *events.Emitter) { e.StreamInterrupted("client disconnected") },
			level:   "ERROR",
			message: "Stream interrupted: client disconnected",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msg := emitThroughBus(t, tc.emit)
			lm, ok := msg.(logging.Msg)
			require.True(t, ok, "expected logging.Msg, got %T", msg)
			assert.Equal(t, tc.level, lm.Level)
			assert.Equal(t, tc.message, lm.Message)
		})
	}
}

// TestEventAdapter_EmitterConversationEventsReachTheTUI covers the two
// conversation events the live drill-in consumes: the system prompt and the
// per-message latency/cost update. Both are emitted as pointers.
func TestEventAdapter_EmitterConversationEventsReachTheTUI(t *testing.T) {
	t.Parallel()

	t.Run("conversation started carries the system prompt", func(t *testing.T) {
		t.Parallel()
		msg := emitThroughBus(t, func(e *events.Emitter) { e.ConversationStarted("You are terse.") })
		started, ok := msg.(ConversationStartedMsg)
		require.True(t, ok, "expected ConversationStartedMsg, got %T", msg)
		assert.Equal(t, "conv-1", started.ConversationID)
		assert.Equal(t, "You are terse.", started.SystemPrompt)
	})

	t.Run("message updated carries latency, tokens and cost", func(t *testing.T) {
		t.Parallel()
		msg := emitThroughBus(t, func(e *events.Emitter) { e.MessageUpdated(2, 450, 100, 40, 0.0125) })
		updated, ok := msg.(MessageUpdatedMsg)
		require.True(t, ok, "expected MessageUpdatedMsg, got %T", msg)
		assert.Equal(t, "conv-1", updated.ConversationID)
		assert.Equal(t, 2, updated.Index)
		assert.Equal(t, int64(450), updated.LatencyMs)
		assert.Equal(t, 100, updated.InputTokens)
		assert.Equal(t, 40, updated.OutputTokens)
		assert.InDelta(t, 0.0125, updated.TotalCost, 1e-9)
	})
}
