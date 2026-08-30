package web

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/PromptKit/runtime/types"
)

func TestAdapter_RegisterAndBroadcast(t *testing.T) {
	adapter := NewEventAdapter()

	// Register two clients
	ch1 := adapter.Register()
	ch2 := adapter.Register()

	// Create a fake event — uses pointer receiver to match what EmitCustom sends
	event := &events.Event{
		Type:        events.EventType("arena.run.started"),
		Timestamp:   time.Now(),
		ExecutionID: "run-1",
		Data: &events.CustomEventData{
			EventName: "run_started",
			Data: map[string]interface{}{
				"scenario": "greeting",
				"provider": "openai",
				"region":   "default",
			},
		},
	}

	// Handle event (triggers broadcast)
	adapter.HandleEvent(event)

	// Both clients should receive the JSON message
	select {
	case msg := <-ch1:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal ch1: %v", err)
		}
		if got.Type != "arena.run.started" {
			t.Errorf("ch1 type = %q, want %q", got.Type, "arena.run.started")
		}
		if got.ExecutionID != "run-1" {
			t.Errorf("ch1 executionID = %q, want %q", got.ExecutionID, "run-1")
		}
	case <-time.After(time.Second):
		t.Fatal("ch1 timed out")
	}

	select {
	case msg := <-ch2:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal ch2: %v", err)
		}
		if got.Type != "arena.run.started" {
			t.Errorf("ch2 type = %q, want %q", got.Type, "arena.run.started")
		}
	case <-time.After(time.Second):
		t.Fatal("ch2 timed out")
	}
}

func TestAdapter_Unregister(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()
	adapter.Unregister(ch)

	// After unregister, HandleEvent should not block or panic
	event := &events.Event{
		Type:      events.EventType("arena.run.started"),
		Timestamp: time.Now(),
		Data: &events.CustomEventData{
			EventName: "run_started",
			Data:      map[string]interface{}{},
		},
	}
	adapter.HandleEvent(event)

	select {
	case <-ch:
		t.Fatal("should not receive after unregister")
	case <-time.After(50 * time.Millisecond):
		// Expected: no message
	}
}

func TestAdapter_MapProviderCallCompleted(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	event := &events.Event{
		Type:        events.EventProviderCallCompleted,
		Timestamp:   time.Now(),
		ExecutionID: "run-1",
		Data: &events.ProviderCallCompletedData{
			Provider: "openai",
			Model:    "gpt-4",
			Duration: 2500 * time.Millisecond,
			Cost:     0.0042,
		},
	}
	adapter.HandleEvent(event)

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Type != "provider.call.completed" {
			t.Errorf("type = %q, want %q", got.Type, "provider.call.completed")
		}
		data, ok := got.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data is %T, want map", got.Data)
		}
		if data["provider"] != "openai" {
			t.Errorf("provider = %v, want openai", data["provider"])
		}
		if data["cost"] != 0.0042 {
			t.Errorf("cost = %v, want 0.0042", data["cost"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAdapter_MapMessageCreated(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	event := &events.Event{
		Type:           events.EventMessageCreated,
		Timestamp:      time.Now(),
		ConversationID: "conv-1",
		Data: events.MessageCreatedData{
			Role:    "assistant",
			Content: "Hello!",
			Index:   0,
		},
	}
	adapter.HandleEvent(event)

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Type != "message.created" {
			t.Errorf("type = %q, want %q", got.Type, "message.created")
		}
		if got.ConversationID != "conv-1" {
			t.Errorf("conversationId = %q, want %q", got.ConversationID, "conv-1")
		}
		data, ok := got.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data is %T, want map", got.Data)
		}
		if data["role"] != "assistant" {
			t.Errorf("role = %v, want assistant", data["role"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

// TestAdapter_MapMessageCreatedPtr regression-tests the contract between the
// runtime emitter and the SSE adapter: emitter.go publishes
// &MessageCreatedData{} (a pointer), so the adapter's type switch MUST handle
// the pointer case. Without it the data field is nil, the frontend mapper
// short-circuits on `!d`, and live messages never stream into the UI.
func TestAdapter_MapMessageCreatedPtr(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	event := &events.Event{
		Type:           events.EventMessageCreated,
		Timestamp:      time.Now(),
		ConversationID: "conv-ptr",
		Data: &events.MessageCreatedData{
			Role:    "user",
			Content: "Hi from a pointer-typed payload",
			Index:   1,
		},
	}
	adapter.HandleEvent(event)

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data, ok := got.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data is %T, want map[string]interface{} — pointer case was probably falling through to default", got.Data)
		}
		if data["role"] != "user" {
			t.Errorf("role = %v, want user", data["role"])
		}
		if data["content"] != "Hi from a pointer-typed payload" {
			t.Errorf("content = %v, want \"Hi from a pointer-typed payload\"", data["content"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SSE broadcast")
	}
}

// TestAdapter_MapMessageUpdatedPtr — same regression as MessageCreated above.
func TestAdapter_MapMessageUpdatedPtr(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	event := &events.Event{
		Type:           events.EventMessageUpdated,
		Timestamp:      time.Now(),
		ConversationID: "conv-ptr",
		Data: &events.MessageUpdatedData{
			Index:        2,
			LatencyMs:    1234,
			InputTokens:  100,
			OutputTokens: 50,
			TotalCost:    0.01,
		},
	}
	adapter.HandleEvent(event)

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data, ok := got.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data is %T, want map[string]interface{}", got.Data)
		}
		if v, _ := data["latencyMs"].(float64); v != 1234 {
			t.Errorf("latencyMs = %v, want 1234", data["latencyMs"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SSE broadcast")
	}
}

// TestAdapter_EmitterContract round-trips a real runtime emitter through a
// real bus into the adapter, so the test fails the moment runtime emission
// shape diverges from what the adapter handles. This catches gaps the
// individual mapEvent unit tests miss (they use crafted events that may not
// match what the emitter actually publishes).
func TestAdapter_EmitterContract(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	bus := events.NewEventBus()
	defer bus.Close()
	adapter.Subscribe(bus)

	emitter := events.NewEmitter(bus, "exec-1", "sess-1", "conv-1")

	// MessageCreated mirrors what the duplex executor calls during a real run.
	emitter.MessageCreated("user", "round-trip test", 0, nil, nil, nil, nil)

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Type != "message.created" {
			t.Errorf("type = %q, want message.created", got.Type)
		}
		if got.Data == nil {
			t.Fatal("data is nil — adapter dropped the emitter's payload")
		}
		data, ok := got.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data is %T, want map[string]interface{}", got.Data)
		}
		if data["content"] != "round-trip test" {
			t.Errorf("content = %v, want round-trip test", data["content"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out — emitter → bus → adapter round-trip didn't deliver")
	}
}

func TestAdapter_Subscribe(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	bus := events.NewEventBus()
	defer bus.Close()
	adapter.Subscribe(bus)

	// Publish an event through the real bus
	bus.Publish(&events.Event{
		Type:      events.EventType("arena.run.started"),
		Timestamp: time.Now(),
		Data: &events.CustomEventData{
			EventName: "run_started",
			Data:      map[string]interface{}{"scenario": "test"},
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Type != "arena.run.started" {
			t.Errorf("type = %q, want arena.run.started", got.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for bus event")
	}
}

func TestAdapter_SubscribeNilBus(t *testing.T) {
	adapter := NewEventAdapter()
	// Should not panic
	adapter.Subscribe(nil)
}

func TestAdapter_MapProviderCallStarted(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	adapter.HandleEvent(&events.Event{
		Type:      events.EventProviderCallStarted,
		Timestamp: time.Now(),
		Data: &events.ProviderCallStartedData{
			Provider: "anthropic",
			Model:    "claude-3",
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data := got.Data.(map[string]interface{})
		if data["provider"] != "anthropic" {
			t.Errorf("provider = %v, want anthropic", data["provider"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAdapter_MapProviderCallFailed(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	adapter.HandleEvent(&events.Event{
		Type:      events.EventProviderCallFailed,
		Timestamp: time.Now(),
		Data: &events.ProviderCallFailedData{
			Provider: "openai",
			Model:    "gpt-4",
			Error:    fmt.Errorf("rate limit"),
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data := got.Data.(map[string]interface{})
		if data["error"] != "rate limit" {
			t.Errorf("error = %v, want 'rate limit'", data["error"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAdapter_MapMessageUpdated(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	adapter.HandleEvent(&events.Event{
		Type:           events.EventMessageUpdated,
		Timestamp:      time.Now(),
		ConversationID: "conv-2",
		Data: events.MessageUpdatedData{
			Index:        1,
			LatencyMs:    250,
			InputTokens:  100,
			OutputTokens: 50,
			TotalCost:    0.001,
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.ConversationID != "conv-2" {
			t.Errorf("conversationId = %q, want conv-2", got.ConversationID)
		}
		data := got.Data.(map[string]interface{})
		if data["latencyMs"] != float64(250) {
			t.Errorf("latencyMs = %v, want 250", data["latencyMs"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAdapter_MapConversationStarted(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	adapter.HandleEvent(&events.Event{
		Type:      events.EventConversationStarted,
		Timestamp: time.Now(),
		Data: events.ConversationStartedData{
			SystemPrompt: "You are helpful.",
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data := got.Data.(map[string]interface{})
		if data["systemPrompt"] != "You are helpful." {
			t.Errorf("systemPrompt = %v", data["systemPrompt"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAdapter_MapMiddlewareCompleted(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	adapter.HandleEvent(&events.Event{
		Type:      events.EventMiddlewareCompleted,
		Timestamp: time.Now(),
		Data: events.MiddlewareCompletedData{
			Name:     "guardrail",
			Duration: 100 * time.Millisecond,
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data := got.Data.(map[string]interface{})
		if data["name"] != "guardrail" {
			t.Errorf("name = %v, want guardrail", data["name"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAdapter_MapToolCallEvent(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	// Value receiver
	adapter.HandleEvent(&events.Event{
		Type:      events.EventToolCallStarted,
		Timestamp: time.Now(),
		Data: events.ToolCallEventData{
			ToolName: "memory__recall",
			CallID:   "call-1",
			Status:   "pending",
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data := got.Data.(map[string]interface{})
		if data["toolName"] != "memory__recall" {
			t.Errorf("toolName = %v, want memory__recall", data["toolName"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAdapter_MapToolCallEventPtr(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	// Pointer receiver
	adapter.HandleEvent(&events.Event{
		Type:      events.EventToolCallCompleted,
		Timestamp: time.Now(),
		Data: &events.ToolCallEventData{
			ToolName: "workflow__transition",
			CallID:   "call-2",
			Status:   "success",
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data := got.Data.(map[string]interface{})
		if data["toolName"] != "workflow__transition" {
			t.Errorf("toolName = %v, want workflow__transition", data["toolName"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAdapter_MapValidationEvent(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	adapter.HandleEvent(&events.Event{
		Type:      events.EventValidationPassed,
		Timestamp: time.Now(),
		Data: events.ValidationEventData{
			ValidatorName: "output-guard",
			ValidatorType: "output",
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data := got.Data.(map[string]interface{})
		if data["validatorName"] != "output-guard" {
			t.Errorf("validatorName = %v, want output-guard", data["validatorName"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAdapter_MapValidationEventPtr(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	adapter.HandleEvent(&events.Event{
		Type:      events.EventValidationFailed,
		Timestamp: time.Now(),
		Data: &events.ValidationEventData{
			ValidatorName: "pii-filter",
			ValidatorType: "output",
			Error:         fmt.Errorf("PII detected"),
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data := got.Data.(map[string]interface{})
		if data["validatorName"] != "pii-filter" {
			t.Errorf("validatorName = %v, want pii-filter", data["validatorName"])
		}
		if data["error"] != "PII detected" {
			t.Errorf("error = %v, want 'PII detected'", data["error"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

// TestAdapter_CustomEventDataPointer verifies that *CustomEventData (pointer,
// which is what EmitCustom actually sends) produces populated data, not nil.
// This was the root cause of the blank-screen bug: the adapter only matched
// the value receiver, so arena events arrived with data:null in the browser.
func TestAdapter_CustomEventDataPointer(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	// EmitCustom sends &CustomEventData{} — pointer receiver
	adapter.HandleEvent(&events.Event{
		Type:        events.EventType("arena.run.started"),
		Timestamp:   time.Now(),
		ExecutionID: "run-ptr",
		Data: &events.CustomEventData{
			EventName: "run_started",
			Data: map[string]interface{}{
				"scenario": "greeting",
				"provider": "openai",
				"region":   "default",
			},
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// The critical assertion: data must NOT be nil
		if got.Data == nil {
			t.Fatal("data is nil — pointer CustomEventData not handled")
		}
		data, ok := got.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data is %T, want map", got.Data)
		}
		if data["scenario"] != "greeting" {
			t.Errorf("scenario = %v, want greeting", data["scenario"])
		}
		if data["provider"] != "openai" {
			t.Errorf("provider = %v, want openai", data["provider"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAdapter_MapUnknownEventData(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	// Unknown/unhandled event data type — should still send the event with nil data
	adapter.HandleEvent(&events.Event{
		Type:      events.EventType("some.unknown.event"),
		Timestamp: time.Now(),
		Data:      nil,
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Type != "some.unknown.event" {
			t.Errorf("type = %q, want some.unknown.event", got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestAdapter_DropOnFullBuffer(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	event := &events.Event{
		Type:      events.EventType("arena.run.started"),
		Timestamp: time.Now(),
		Data: &events.CustomEventData{
			EventName: "test",
			Data:      map[string]interface{}{},
		},
	}

	// Fill the buffer (clientBufferSize = 256)
	for i := 0; i < clientBufferSize+10; i++ {
		adapter.HandleEvent(event)
	}

	// Should not block or panic — extra events are dropped
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != clientBufferSize {
		t.Errorf("received %d events, want %d (buffer size)", count, clientBufferSize)
	}
}

// TestAdapter_MapMessageCreatedReasoning pins that a turn's thinking reaches
// the browser. Reasoning lives on MessageCreatedData.Reasoning, whose upstream
// json tag is "-", so it only crosses the wire because the adapter maps it
// explicitly — a silent regression if that mapping is dropped, since the run
// still streams and only the disclosure goes missing.
func TestAdapter_MapMessageCreatedReasoning(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	adapter.HandleEvent(&events.Event{
		Type:      events.EventMessageCreated,
		Timestamp: time.Now(),
		Data: &events.MessageCreatedData{
			Role:      "assistant",
			Content:   "ANSWER: 208",
			Index:     2,
			Reasoning: &types.ReasoningTrace{Text: "D=5, C=15, B=11, A=22"},
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data, ok := got.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data is %T, want map", got.Data)
		}
		reasoning, ok := data["reasoning"].(map[string]interface{})
		if !ok {
			t.Fatalf("reasoning is %T, want map — the trace never reached the client", data["reasoning"])
		}
		if reasoning["text"] != "D=5, C=15, B=11, A=22" {
			t.Errorf("reasoning text = %v, want the emitted trace", reasoning["text"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

// TestWithReasoning covers the shaping rules: a turn with no reasoning leaves
// the key absent rather than sending null or an empty object (either would have
// the client render an empty disclosure), and opaque provider entries never
// cross the wire.
func TestWithReasoning(t *testing.T) {
	for _, tc := range []struct {
		name  string
		trace *types.ReasoningTrace
	}{
		{"nil trace", nil},
		{"empty trace", &types.ReasoningTrace{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := withReasoning(map[string]interface{}{"role": "assistant"}, tc.trace)
			if _, present := got["reasoning"]; present {
				t.Errorf("reasoning key present for %s, want absent", tc.name)
			}
		})
	}

	got := withReasoning(map[string]interface{}{}, &types.ReasoningTrace{
		Text:     "thinking",
		Redacted: true,
		Opaque:   []types.OpaqueReasoning{{Provider: "claude", Kind: "thinking_signature", Data: "secret"}},
	})
	reasoning, ok := got["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("reasoning is %T, want map", got["reasoning"])
	}
	if reasoning["text"] != "thinking" || reasoning["redacted"] != true {
		t.Errorf("payload = %v, want text and redacted carried", reasoning)
	}
	if _, leaked := reasoning["opaque"]; leaked {
		t.Error("opaque reasoning must not cross the wire")
	}
}

// TestDisplayMessage_StripsOpaqueReasoning pins that the message.full route
// agrees with message.created about what reaches the browser. Opaque entries
// are provider signatures and encrypted blocks kept for intra-turn round-trip;
// they are never displayed and measured larger than the text they accompany, so
// shipping them to every SSE client is pure waste. The stored message must not
// be mutated in the process — it is still needed for the next provider call.
func TestDisplayMessage_StripsOpaqueReasoning(t *testing.T) {
	stored := &types.Message{
		Role:    "assistant",
		Content: "ANSWER: 208",
		Reasoning: &types.ReasoningTrace{
			Text:     "D=5, C=15",
			Redacted: true,
			Opaque:   []types.OpaqueReasoning{{Provider: "claude", Kind: "thinking_signature", Data: "sig"}},
		},
	}

	got := displayMessage(stored)
	if len(got.Reasoning.Opaque) != 0 {
		t.Errorf("opaque entries = %d, want 0 on the display path", len(got.Reasoning.Opaque))
	}
	if got.Reasoning.Text != "D=5, C=15" || !got.Reasoning.Redacted {
		t.Errorf("reasoning = %+v, want text and redacted preserved", got.Reasoning)
	}
	if len(stored.Reasoning.Opaque) != 1 {
		t.Error("displayMessage mutated the stored message; the trace is still needed for round-trip")
	}

	// A message without opaque entries is passed through untouched.
	plain := &types.Message{Role: "user", Content: "hi"}
	if displayMessage(plain) != plain {
		t.Error("message without opaque reasoning should pass through unchanged")
	}
}

// TestAdapter_MapReasoningDelta pins that thinking fragments reach the browser
// as they are produced. The payload switch previously had no case for
// ReasoningDeltaData, so the frame went out with nil data and the frontend
// dropped it — the web UI showed nothing until the turn completed while the TUI
// streamed. Round and providerCallId travel with the text because they are the
// join key that says which tool-loop round is being watched.
func TestAdapter_MapReasoningDelta(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	adapter.HandleEvent(&events.Event{
		Type:      events.EventReasoningDelta,
		Timestamp: time.Now(),
		Data:      &events.ReasoningDeltaData{Text: "D = 5/hr", Round: 2, ProviderCallID: "pc-7"},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Type != "reasoning.delta" {
			t.Errorf("type = %q, want reasoning.delta", got.Type)
		}
		d, ok := got.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data is %T, want map — the fragment never reached the client", got.Data)
		}
		if d["text"] != "D = 5/hr" {
			t.Errorf("text = %v, want the emitted fragment", d["text"])
		}
		if d["round"] != float64(2) || d["providerCallId"] != "pc-7" {
			t.Errorf("round/providerCallId = %v/%v, want 2/pc-7", d["round"], d["providerCallId"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

// TestBroadcastContentDelta covers the answer half. There is no content delta on
// the event bus, so this is an explicit broadcast from the interactive handler's
// chunk stream — which previously drained the stream and discarded every chunk.
func TestBroadcastContentDelta(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	// An empty delta carries nothing to render and must not produce a frame.
	adapter.BroadcastContentDelta("conv-1", 3, "")
	select {
	case <-ch:
		t.Fatal("empty delta produced a frame")
	default:
	}

	adapter.BroadcastContentDelta("conv-1", 3, "ANSWER")
	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Type != "content.delta" || got.ConversationID != "conv-1" {
			t.Errorf("type/conv = %q/%q, want content.delta/conv-1", got.Type, got.ConversationID)
		}
		d, ok := got.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data is %T, want map", got.Data)
		}
		if d["delta"] != "ANSWER" || d["index"] != float64(3) {
			t.Errorf("delta/index = %v/%v, want ANSWER/3", d["delta"], d["index"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

// TestAdapter_MapMessageCreated_TextInParts is the SSE half of the same
// contract as the TUI's TestEventAdapter_MessageCreated_TextInParts.
//
// A user turn carries its text in Parts with Content empty, so reading
// .Content directly streams an empty message to the frontend and the run reads
// as though the user said nothing.
func TestAdapter_MapMessageCreated_TextInParts(t *testing.T) {
	text := "where is my order"
	adapter := NewEventAdapter()
	ch := adapter.Register()

	adapter.HandleEvent(&events.Event{
		Type:           events.EventMessageCreated,
		Timestamp:      time.Now(),
		ConversationID: "conv-parts",
		Data: &events.MessageCreatedData{
			Role:  "user",
			Parts: []types.ContentPart{{Type: "text", Text: &text}},
		},
	})

	select {
	case msg := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data, ok := got.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data is %T, want map", got.Data)
		}
		if data["content"] != text {
			t.Errorf("content = %v, want %q — the user's text was dropped on the way "+
				"to the frontend", data["content"], text)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}
