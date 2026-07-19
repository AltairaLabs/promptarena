package statestore

import (
	"context"
	"testing"
	"time"

	runtimestore "github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/types"
)

// TestSetOnSave_FiresWithFullMessage verifies that a callback registered via
// SetOnSave fires on Save and receives the full persisted ConversationState,
// including message fields (Meta, CostInfo) that the thin SSE projection
// omits today.
func TestSetOnSave_FiresWithFullMessage(t *testing.T) {
	store := NewArenaStateStore()
	ctx := context.Background()

	done := make(chan *runtimestore.ConversationState, 1)
	store.SetOnSave(func(st *runtimestore.ConversationState) {
		done <- st
	})

	state := &runtimestore.ConversationState{
		ID: "conv-onsave",
		Messages: []types.Message{
			{
				Role:    "assistant",
				Content: "hi",
				Meta:    map[string]interface{}{"_available_tools": []string{"x"}},
				CostInfo: &types.CostInfo{
					InputTokens:  10,
					OutputTokens: 20,
					TotalCost:    0.01,
				},
			},
		},
	}

	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	select {
	case got := <-done:
		if got == nil {
			t.Fatal("onSave callback received nil state")
		}
		if len(got.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(got.Messages))
		}
		msg := got.Messages[0]
		if msg.CostInfo == nil || msg.CostInfo.TotalCost != 0.01 {
			t.Errorf("expected CostInfo.TotalCost=0.01, got %+v", msg.CostInfo)
		}
		if msg.Meta == nil || msg.Meta["_available_tools"] == nil {
			t.Errorf("expected Meta to carry _available_tools, got %+v", msg.Meta)
		}
	case <-time.After(time.Second):
		t.Fatal("onSave callback did not fire")
	}
}

// TestSetOnSave_RunsAfterUnlock proves the callback runs after the store's
// internal lock is released: calling back into the store (Load) from within
// the callback must not deadlock.
func TestSetOnSave_RunsAfterUnlock(t *testing.T) {
	store := NewArenaStateStore()
	ctx := context.Background()

	done := make(chan struct{})
	store.SetOnSave(func(st *runtimestore.ConversationState) {
		if _, err := store.Load(ctx, st.ID); err != nil {
			t.Errorf("Load from within onSave callback failed: %v", err)
		}
		close(done)
	})

	state := &runtimestore.ConversationState{
		ID:       "conv-deadlock",
		Messages: []types.Message{{Role: "user", Content: "hello"}},
	}

	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out — onSave callback likely deadlocked while holding s.mu")
	}
}

// TestSetOnSave_SaveWithTraceAlsoFires verifies SaveWithTrace invokes the
// callback the same way Save does.
func TestSetOnSave_SaveWithTraceAlsoFires(t *testing.T) {
	store := NewArenaStateStore()
	ctx := context.Background()

	var calls int
	store.SetOnSave(func(st *runtimestore.ConversationState) {
		calls++
	})

	state := &runtimestore.ConversationState{
		ID:       "conv-trace",
		Messages: []types.Message{{Role: "user", Content: "hello"}},
	}

	if err := store.SaveWithTrace(ctx, state, nil); err != nil {
		t.Fatalf("SaveWithTrace failed: %v", err)
	}

	if calls != 1 {
		t.Errorf("expected onSave to fire once, got %d", calls)
	}
}

// TestSave_NoOnSave_NoPanic verifies Save works fine when no callback is
// registered (the nil-by-default case).
func TestSave_NoOnSave_NoPanic(t *testing.T) {
	store := NewArenaStateStore()
	ctx := context.Background()

	state := &runtimestore.ConversationState{
		ID:       "conv-no-callback",
		Messages: []types.Message{{Role: "user", Content: "hello"}},
	}

	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
}
