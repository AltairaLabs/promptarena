package tui

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/tui/logging"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
)

func mapToLog(t *testing.T, evt *events.Event) logging.Msg {
	t.Helper()
	msg := NewEventAdapter(nil).mapEvent(evt)
	require.NotNil(t, msg, "event %s produced no TUI message", evt.Type)
	lm, ok := msg.(logging.Msg)
	require.True(t, ok, "expected logging.Msg, got %T", msg)
	return lm
}

func TestEventAdapter_StageEvents(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		eventType events.EventType
		data      events.EventData
		level     string
		message   string
	}{
		{
			name:      "started with type",
			eventType: events.EventStageStarted,
			data:      &events.StageEventData{Name: "provider", StageType: "generate"},
			level:     "DEBUG",
			message:   "Stage started: provider [generate]",
		},
		{
			name:      "started without type",
			eventType: events.EventStageStarted,
			data:      events.StageEventData{Name: "template"},
			level:     "DEBUG",
			message:   "Stage started: template",
		},
		{
			name:      "completed",
			eventType: events.EventStageCompleted,
			data:      &events.StageEventData{Name: "provider", Duration: 1500 * time.Millisecond},
			level:     "DEBUG",
			message:   "Stage completed: provider (1.50s)",
		},
		{
			name:      "failed",
			eventType: events.EventStageFailed,
			data: &events.StageEventData{
				Name: "guardrail", Duration: 250 * time.Millisecond, Error: errors.New("blocked"),
			},
			level:   "ERROR",
			message: "Stage failed: guardrail (0.25s, blocked)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lm := mapToLog(t, &events.Event{Type: tc.eventType, Timestamp: time.Now(), Data: tc.data})
			assert.Equal(t, tc.level, lm.Level)
			assert.Equal(t, tc.message, lm.Message)
		})
	}
}

func TestEventAdapter_StageEventWithoutPayload(t *testing.T) {
	t.Parallel()

	adapter := NewEventAdapter(nil)
	assert.Nil(t, adapter.mapEvent(&events.Event{Type: events.EventStageStarted}))
	var nilData *events.StageEventData
	assert.Nil(t, adapter.mapEvent(&events.Event{Type: events.EventStageCompleted, Data: nilData}))
}

// Middleware events only appear in recordings made before PromptKit moved to
// stages. Those recordings load as pointers, which the completed handler used
// to drop.
func TestEventAdapter_LegacyMiddlewareEvents(t *testing.T) {
	t.Parallel()

	lm := mapToLog(t, &events.Event{
		Type: events.EventMiddlewareCompleted,
		Data: &events.MiddlewareEventData{Name: "auth", Duration: 2 * time.Second},
	})
	assert.Equal(t, "Middleware completed: auth (2.00s)", lm.Message)

	lm = mapToLog(t, &events.Event{
		Type: events.EventMiddlewareFailed,
		Data: &events.MiddlewareEventData{Name: "auth", Error: errors.New("denied")},
	})
	assert.Equal(t, "ERROR", lm.Level)
	assert.Equal(t, "Middleware failed: auth (denied)", lm.Message)

	assert.Nil(t, NewEventAdapter(nil).mapEvent(&events.Event{Type: events.EventMiddlewareCompleted}))
}
