package engine

import (
	"context"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The store used to subscribe to conversation.started, which nothing in Arena
// emits. The subscription is gone; only stream.interrupted comes off the bus.
func TestSessionRecording_SubscribesOnlyStreamInterrupted(t *testing.T) {
	eng := &Engine{}
	bus := events.NewEventBus()
	eng.SetEventBus(bus)
	store := &fakeEventStore{}
	require.NoError(t, eng.EnableSessionRecordingWithStore(store))

	bus.Publish(&events.Event{Type: events.EventConversationStarted, SessionID: "s"})
	bus.Publish(&events.Event{Type: events.EventStreamInterrupted, SessionID: "s"})
	bus.Close()

	got, err := store.Query(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, events.EventStreamInterrupted, got[0].Type)
}
