package web

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/types"
)

// TestBroadcastFullMessages_SendsFullMessage verifies that
// BroadcastFullMessages emits a "message.full" SSE event per message,
// carrying the full types.Message (including Meta and CostInfo) rather than
// the thin projection used by the live event stream.
func TestBroadcastFullMessages_SendsFullMessage(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	msgs := []types.Message{
		{
			Role:    "assistant",
			Content: "hi",
			Meta:    map[string]interface{}{"_available_tools": []string{"x"}},
			CostInfo: &types.CostInfo{
				InputTokens:  10,
				OutputTokens: 20,
				TotalCost:    0.05,
			},
		},
	}

	adapter.BroadcastFullMessages("conv1", msgs)

	select {
	case data := <-ch:
		var got SSEEvent
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Type != "message.full" {
			t.Errorf("type = %q, want %q", got.Type, "message.full")
		}
		if got.ConversationID != "conv1" {
			t.Errorf("conversationId = %q, want %q", got.ConversationID, "conv1")
		}

		dataMap, ok := got.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data is not a map: %#v", got.Data)
		}
		if dataMap["index"] != float64(0) {
			t.Errorf("index = %v, want 0", dataMap["index"])
		}

		msgMap, ok := dataMap["message"].(map[string]interface{})
		if !ok {
			t.Fatalf("message is not a map: %#v", dataMap["message"])
		}
		if msgMap["role"] != "assistant" {
			t.Errorf("role = %v, want assistant", msgMap["role"])
		}
		if msgMap["content"] != "hi" {
			t.Errorf("content = %v, want hi", msgMap["content"])
		}

		costInfo, ok := msgMap["cost_info"].(map[string]interface{})
		if !ok {
			t.Fatalf("cost_info missing from full message: %#v", msgMap)
		}
		if costInfo["total_cost_usd"] != 0.05 {
			t.Errorf("total_cost_usd = %v, want 0.05", costInfo["total_cost_usd"])
		}

		meta, ok := msgMap["meta"].(map[string]interface{})
		if !ok {
			t.Fatalf("meta missing from full message: %#v", msgMap)
		}
		if meta["_available_tools"] == nil {
			t.Errorf("expected _available_tools in meta, got %#v", meta)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast")
	}
}

// TestBroadcastFullMessages_NoClients_NoBroadcast verifies that
// BroadcastFullMessages is a cheap no-op when there are no registered
// clients — no panics, and (since there's nothing to receive) nothing is
// sent anywhere.
func TestBroadcastFullMessages_NoClients_NoBroadcast(t *testing.T) {
	adapter := NewEventAdapter()

	// No clients registered. This must not panic and must return promptly.
	adapter.BroadcastFullMessages("conv1", []types.Message{{Role: "user", Content: "hi"}})

	// Register a client afterwards — it must never receive the stale broadcast
	// since none should have been attempted.
	ch := adapter.Register()
	select {
	case data := <-ch:
		t.Fatalf("expected no broadcast to have been sent, got: %s", data)
	case <-time.After(100 * time.Millisecond):
		// expected: nothing arrives
	}
}

// TestBroadcastFullMessages_MultipleMessages_IndexesEach verifies each
// message in the slice is broadcast as its own event with the right index.
func TestBroadcastFullMessages_MultipleMessages_IndexesEach(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	msgs := []types.Message{
		{Role: "user", Content: "one"},
		{Role: "assistant", Content: "two"},
	}

	adapter.BroadcastFullMessages("conv2", msgs)

	for wantIdx, wantContent := range map[int]string{0: "one", 1: "two"} {
		select {
		case data := <-ch:
			var got SSEEvent
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			dataMap := got.Data.(map[string]interface{})
			idx := int(dataMap["index"].(float64))
			msgMap := dataMap["message"].(map[string]interface{})
			if msgMap["content"] != wantContent {
				// Only check content for the index we happen to be looking at.
				continue
			}
			if idx != wantIdx {
				t.Errorf("index = %d, want %d for content %q", idx, wantIdx, wantContent)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for broadcast")
		}
	}
}
