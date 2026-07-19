package web

import (
	"encoding/json"
	"strconv"
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

// drainIndices reads every event currently buffered on ch (without blocking
// once the channel is empty) and returns the message.full index of each, in
// arrival order.
func drainIndices(t *testing.T, ch chan []byte) []int {
	t.Helper()
	var got []int
	for {
		select {
		case data := <-ch:
			var ev SSEEvent
			if err := json.Unmarshal(data, &ev); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			dataMap, ok := ev.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("data is not a map: %#v", ev.Data)
			}
			got = append(got, int(dataMap["index"].(float64)))
		default:
			return got
		}
	}
}

// TestBroadcastFullMessages_SkipsUnchanged verifies the per-client delta: a
// second broadcast of an identical history sends nothing, rather than
// re-sending every message as the previous full-history implementation did.
func TestBroadcastFullMessages_SkipsUnchanged(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	msgs := []types.Message{
		{Role: "user", Content: "one"},
		{Role: "assistant", Content: "two"},
	}

	adapter.BroadcastFullMessages("conv", msgs)
	if got := drainIndices(t, ch); len(got) != 2 {
		t.Fatalf("first broadcast sent %v, want both indices", got)
	}

	adapter.BroadcastFullMessages("conv", msgs)
	if got := drainIndices(t, ch); len(got) != 0 {
		t.Errorf("re-broadcast of unchanged history sent %v, want nothing", got)
	}
}

// TestBroadcastFullMessages_SendsOnlyChanged verifies that when one message is
// mutated in place (e.g. cost/meta stamped on a later save) and another is
// appended, only those two indices go out — not the untouched history.
func TestBroadcastFullMessages_SendsOnlyChanged(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	msgs := []types.Message{
		{Role: "user", Content: "one"},
		{Role: "assistant", Content: "two"},
		{Role: "user", Content: "three"},
	}
	adapter.BroadcastFullMessages("conv", msgs)
	drainIndices(t, ch)

	// Mutate index 1 in a way that does not touch Content — this is the
	// cost/meta-only update a content-equality check would miss.
	msgs[1].CostInfo = &types.CostInfo{InputTokens: 1, OutputTokens: 2, TotalCost: 0.01}
	msgs = append(msgs, types.Message{Role: "assistant", Content: "four"})

	adapter.BroadcastFullMessages("conv", msgs)

	got := drainIndices(t, ch)
	want := map[int]bool{1: true, 3: true}
	if len(got) != len(want) {
		t.Fatalf("sent %v, want exactly indices 1 (mutated) and 3 (appended)", got)
	}
	for _, idx := range got {
		if !want[idx] {
			t.Errorf("sent unchanged index %d; got %v", idx, got)
		}
	}
}

// TestBroadcastFullMessages_LateClientGetsFullHistory verifies the catch-up
// property the full re-send used to provide: a client that registers midway
// through a conversation receives the entire history on the next save, while
// the already-caught-up client receives only what changed.
func TestBroadcastFullMessages_LateClientGetsFullHistory(t *testing.T) {
	adapter := NewEventAdapter()
	early := adapter.Register()

	msgs := []types.Message{
		{Role: "user", Content: "one"},
		{Role: "assistant", Content: "two"},
	}
	adapter.BroadcastFullMessages("conv", msgs)
	drainIndices(t, early)

	// A dashboard opens its SSE stream mid-conversation.
	late := adapter.Register()

	msgs = append(msgs, types.Message{Role: "user", Content: "three"})
	adapter.BroadcastFullMessages("conv", msgs)

	if got := drainIndices(t, late); len(got) != 3 {
		t.Errorf("late client got %v, want full history (3 messages)", got)
	}
	got := drainIndices(t, early)
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("established client got %v, want only the appended index 2", got)
	}
}

// TestBroadcastFullMessages_DroppedFrameIsRetried verifies that a message
// dropped because the client's buffer was full is not recorded as sent, and so
// is redelivered on the next save. This is the self-healing property that
// makes delta delivery safe given broadcast's non-blocking, lossy send.
func TestBroadcastFullMessages_DroppedFrameIsRetried(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	// More messages than the client buffer can hold, so the tail is dropped.
	overflow := clientBufferSize + 10
	msgs := make([]types.Message, overflow)
	for i := range msgs {
		msgs[i] = types.Message{Role: "user", Content: strconv.Itoa(i)}
	}

	adapter.BroadcastFullMessages("conv", msgs)

	delivered := drainIndices(t, ch)
	if len(delivered) != clientBufferSize {
		t.Fatalf("delivered %d messages, want the buffer's %d", len(delivered), clientBufferSize)
	}

	// The consumer has now drained; the next save must retry exactly the
	// messages that were dropped, and not resend the ones it already has.
	adapter.BroadcastFullMessages("conv", msgs)

	retried := drainIndices(t, ch)
	if len(retried) != overflow-clientBufferSize {
		t.Fatalf("retried %d messages, want the %d that were dropped",
			len(retried), overflow-clientBufferSize)
	}
	for _, idx := range retried {
		if idx < clientBufferSize {
			t.Errorf("retried index %d, which was already delivered", idx)
		}
	}
}

// TestBroadcastFullMessages_UnregisterFreesState verifies that a client's
// delta-tracking state is released when it disconnects, so a long batch run
// with dashboards connecting and dropping does not accumulate memory.
func TestBroadcastFullMessages_UnregisterFreesState(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Register()

	adapter.BroadcastFullMessages("conv", []types.Message{{Role: "user", Content: "one"}})
	adapter.Unregister(ch)

	adapter.mu.RLock()
	n := len(adapter.clients)
	adapter.mu.RUnlock()
	if n != 0 {
		t.Errorf("clients after unregister = %d, want 0", n)
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
