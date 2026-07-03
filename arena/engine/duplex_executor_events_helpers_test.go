package engine

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

// collectEventTypes subscribes to the bus and returns a snapshot func for the
// event types seen so far, plus a wait helper for an expected count.
func collectEventTypes(bus events.Bus, expected int) (func() []events.EventType, func(*testing.T)) {
	var mu sync.Mutex
	var seen []events.EventType
	var wg sync.WaitGroup
	wg.Add(expected)
	bus.SubscribeAll(func(e *events.Event) {
		mu.Lock()
		seen = append(seen, e.Type)
		mu.Unlock()
		wg.Done()
	})
	snapshot := func() []events.EventType {
		mu.Lock()
		defer mu.Unlock()
		out := make([]events.EventType, len(seen))
		copy(out, seen)
		return out
	}
	wait := func(t *testing.T) {
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for events")
		}
	}
	return snapshot, wait
}

func TestDuplexEventEmitters_NilEmitterIsNoOp(t *testing.T) {
	de := &DuplexConversationExecutor{}
	req := &ConversationRequest{Scenario: &arenaconfig.Scenario{ID: "s"}, ConversationID: "c"}
	assert.NotPanics(t, func() {
		de.emitSessionStarted(nil, req)
		de.emitSessionCompleted(nil, req)
		de.emitSessionError(nil, req, errors.New("x"))
		de.emitTurnStarted(nil, 0, "user", "s")
		de.emitTurnCompleted(nil, 0, "user", "s", nil)
	})
}

func TestEmitTurnCompleted_CompletedVsFailed(t *testing.T) {
	bus := events.NewEventBus()
	emitter := events.NewEmitter(bus, "run", "", "conv")
	snapshot, wait := collectEventTypes(bus, 2)

	de := &DuplexConversationExecutor{}
	de.emitTurnCompleted(emitter, 0, "user", "s", nil)
	de.emitTurnCompleted(emitter, 1, "user", "s", errors.New("boom"))

	wait(t)
	types := snapshot()
	assert.Contains(t, types, events.EventType("arena.duplex.turn.completed"))
	assert.Contains(t, types, events.EventType("arena.duplex.turn.failed"))
}

func TestEmitSessionError_EmitsErrorEvent(t *testing.T) {
	bus := events.NewEventBus()
	emitter := events.NewEmitter(bus, "run", "", "conv")
	snapshot, wait := collectEventTypes(bus, 1)

	de := &DuplexConversationExecutor{}
	req := &ConversationRequest{Scenario: &arenaconfig.Scenario{ID: "s"}, ConversationID: "conv"}
	de.emitSessionError(emitter, req, errors.New("boom"))

	wait(t)
	assert.Contains(t, snapshot(), events.EventType("arena.duplex.session.error"))
}
