package turnexecutors

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/types"
)

func TestMergeStringMaps(t *testing.T) {
	base := map[string]string{"region": "us", "keep": "base"}
	override := map[string]string{"region": "eu", "extra": "over"}

	got := mergeStringMaps(base, override)

	if got["region"] != "eu" {
		t.Errorf("override should win: region = %q, want %q", got["region"], "eu")
	}
	if got["keep"] != "base" {
		t.Errorf("base-only key lost: keep = %q, want %q", got["keep"], "base")
	}
	if got["extra"] != "over" {
		t.Errorf("override-only key missing: extra = %q, want %q", got["extra"], "over")
	}
	// Inputs must be untouched.
	if base["region"] != "us" {
		t.Errorf("base map mutated: region = %q, want %q", base["region"], "us")
	}

	// Nil inputs are tolerated and yield an empty (non-nil) map.
	empty := mergeStringMaps(nil, nil)
	if empty == nil || len(empty) != 0 {
		t.Errorf("mergeStringMaps(nil, nil) = %v, want empty map", empty)
	}
}

func TestScriptedExecutor_UpdateMessagesList(t *testing.T) {
	executor := &ScriptedExecutor{}

	tests := []struct {
		name           string
		messages       []types.Message
		assistantMsg   types.Message
		assistantIndex int
		wantLength     int
	}{
		{
			name: "append new assistant message",
			messages: []types.Message{
				{Role: "user", Content: "Hello"},
			},
			assistantMsg: types.Message{
				Role:    "assistant",
				Content: "Hi there",
			},
			assistantIndex: 1,
			wantLength:     2,
		},
		{
			name: "update existing assistant message",
			messages: []types.Message{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi"},
			},
			assistantMsg: types.Message{
				Role:    "assistant",
				Content: "Hi there, updated",
			},
			assistantIndex: 1,
			wantLength:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executor.updateMessagesList(tt.messages, &tt.assistantMsg, tt.assistantIndex)

			if len(got) != tt.wantLength {
				t.Errorf("messages length = %d, want %d", len(got), tt.wantLength)
			}
			if got[tt.assistantIndex].Content != tt.assistantMsg.Content {
				t.Errorf("assistant message content = %q, want %q",
					got[tt.assistantIndex].Content, tt.assistantMsg.Content)
			}
		})
	}
}

func TestSelfPlayExecutor_UpdateMessagesList(t *testing.T) {
	executor := &SelfPlayExecutor{}

	tests := []struct {
		name           string
		messages       []types.Message
		assistantMsg   types.Message
		assistantIndex int
		wantLength     int
	}{
		{
			name: "append new message",
			messages: []types.Message{
				{Role: "user", Content: "Question"},
			},
			assistantMsg: types.Message{
				Role:    "assistant",
				Content: "Answer",
			},
			assistantIndex: 1,
			wantLength:     2,
		},
		{
			name: "update existing message",
			messages: []types.Message{
				{Role: "user", Content: "Question"},
				{Role: "assistant", Content: "Partial"},
			},
			assistantMsg: types.Message{
				Role:    "assistant",
				Content: "Complete answer",
			},
			assistantIndex: 1,
			wantLength:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executor.updateMessagesList(tt.messages, &tt.assistantMsg, tt.assistantIndex)

			if len(got) != tt.wantLength {
				t.Errorf("messages length = %d, want %d", len(got), tt.wantLength)
			}
			if got[tt.assistantIndex].Content != tt.assistantMsg.Content {
				t.Errorf("assistant message content = %q, want %q",
					got[tt.assistantIndex].Content, tt.assistantMsg.Content)
			}
		})
	}
}
