package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/PromptKit/runtime/recording"
	"github.com/AltairaLabs/PromptKit/runtime/types"
)

// The shipped example is a PromptKit SessionRecording exported by the
// recording middleware. Before this adapter read that shape it returned zero
// messages from it (every event was skipped as a non-message), so the
// session-replay example was documented against a reader that could not read
// it.
const exampleSessionRecording = "../../examples/session-replay/recordings/geography-session.recording.json"

func TestSessionRecordingAdapter_CanHandle(t *testing.T) {
	a := NewSessionRecordingAdapter()
	tests := []struct {
		source, hint string
		want         bool
	}{
		{"session.recording.json", "", true},
		{"recordings/*.recording.json", "", true},
		{"out/recordings/run-123.jsonl", "", true},
		{"anything.json", "session", true},
		{"anything.json", "recording", true},
		{"out/run-123.json", "", false},
		{"session.transcript.yaml", "", false},
		// A hint for another adapter must not be overridden by the extension.
		{"session.recording.json", "arena_output", false},
		{"session.recording.json", "mock", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, a.CanHandle(tt.source, tt.hint), "%s / %q", tt.source, tt.hint)
	}
}

func TestSessionRecordingAdapter_Load_ShippedExample(t *testing.T) {
	msgs, meta, err := NewSessionRecordingAdapter().Load(RecordingReference{ID: exampleSessionRecording})
	require.NoError(t, err)

	require.Len(t, msgs, 6, "every message.created event becomes a message")
	assert.Equal(t, "user", msgs[0].Role)
	assert.Equal(t, "What is the capital of France?", msgs[0].GetContent())
	assert.Equal(t, "assistant", msgs[1].Role)

	require.NotNil(t, meta)
	assert.Equal(t, "geo-session-001", meta.SessionID)
	assert.Len(t, meta.Timestamps, 6)
	assert.Equal(t, 90*time.Second, meta.Duration)
}

// writeSessionRecording marshals a SessionRecording the way recording.Export
// does, so the fixture is the shape the middleware actually produces.
func writeSessionRecording(t *testing.T, dir string, rec *recording.SessionRecording) string {
	t.Helper()
	data, err := json.Marshal(rec)
	require.NoError(t, err)
	path := filepath.Join(dir, rec.Metadata.SessionID+".recording.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func messageEvent(t *testing.T, seq int64, at time.Time, data events.MessageCreatedData) recording.RecordedEvent {
	t.Helper()
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	return recording.RecordedEvent{
		Sequence:  seq,
		Type:      events.EventMessageCreated,
		Timestamp: at,
		SessionID: "sess-1",
		DataType:  "MessageCreatedData",
		Data:      raw,
	}
}

func TestSessionRecordingAdapter_Load_ToolCallsAndParts(t *testing.T) {
	start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	rec := &recording.SessionRecording{
		Metadata: recording.Metadata{
			SessionID:    "sess-1",
			StartTime:    start,
			EndTime:      start.Add(3 * time.Second),
			Duration:     3 * time.Second,
			ProviderName: "openai",
			Model:        "gpt-4o",
			Version:      "1.0",
			Custom:       map[string]any{"tags": []any{"regression", "tools"}},
		},
		Events: []recording.RecordedEvent{
			messageEvent(t, 1, start, events.MessageCreatedData{
				Role: "user",
				Parts: []types.ContentPart{
					types.NewTextPart("What is in this image?"),
					{Type: types.ContentTypeImage, Media: &types.MediaContent{MIMEType: "image/png"}},
				},
			}),
			// Not a message: must be skipped, not turned into an empty message.
			{Sequence: 2, Type: events.EventToolCallStarted, Timestamp: start.Add(time.Second), SessionID: "sess-1", Data: json.RawMessage(`{"tool_name":"lookup"}`)},
			messageEvent(t, 3, start.Add(2*time.Second), events.MessageCreatedData{
				Role:      "assistant",
				ToolCalls: []events.MessageToolCall{{ID: "call-1", Name: "lookup", Args: `{"q":"paris"}`}},
			}),
			messageEvent(t, 4, start.Add(3*time.Second), events.MessageCreatedData{
				Role: "tool",
				ToolResult: &events.MessageToolResult{
					ID: "call-1", Name: "lookup",
					Parts: []types.ContentPart{types.NewTextPart("Eiffel Tower")},
				},
			}),
		},
	}
	path := writeSessionRecording(t, t.TempDir(), rec)

	msgs, meta, err := NewSessionRecordingAdapter().Load(RecordingReference{ID: path})
	require.NoError(t, err)
	require.Len(t, msgs, 3)

	require.Len(t, msgs[0].Parts, 2)
	assert.Equal(t, "What is in this image?", msgs[0].GetContent())
	assert.Equal(t, types.ContentTypeImage, msgs[0].Parts[1].Type)

	require.Len(t, msgs[1].ToolCalls, 1)
	assert.Equal(t, "call-1", msgs[1].ToolCalls[0].ID)
	assert.Equal(t, "lookup", msgs[1].ToolCalls[0].Name)
	assert.JSONEq(t, `{"q":"paris"}`, string(msgs[1].ToolCalls[0].Args))

	require.NotNil(t, msgs[2].ToolResult)
	assert.Equal(t, "call-1", msgs[2].ToolResult.ID)
	assert.Equal(t, "Eiffel Tower", msgs[2].GetContent())

	assert.Equal(t, "sess-1", meta.SessionID)
	assert.Equal(t, []string{"regression", "tools"}, meta.Tags)
	assert.Equal(t, "openai", meta.ProviderInfo["provider_id"])
	assert.Equal(t, "gpt-4o", meta.ProviderInfo["model"])
	assert.Equal(t, 3*time.Second, meta.Duration)
	assert.Len(t, meta.Timestamps, 3)
}

// Recordings are also written as JSONL (the event store keeps one file per
// session, and SaveTo can export the same). recording.Load reads both, so the
// adapter must accept the extension.
func TestSessionRecordingAdapter_Load_JSONL(t *testing.T) {
	start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	rec := &recording.SessionRecording{
		Metadata: recording.Metadata{SessionID: "sess-jsonl", Version: "1.0", StartTime: start, EndTime: start},
		Events: []recording.RecordedEvent{
			messageEvent(t, 1, start, events.MessageCreatedData{Role: "user", Content: "hi"}),
			messageEvent(t, 2, start, events.MessageCreatedData{Role: "assistant", Content: "hello"}),
		},
	}
	path := filepath.Join(t.TempDir(), "sess-jsonl.jsonl")
	require.NoError(t, rec.SaveTo(path, recording.FormatJSONLines))

	msgs, meta, err := NewSessionRecordingAdapter().Load(RecordingReference{ID: path})
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "hello", msgs[1].GetContent())
	assert.Equal(t, "sess-jsonl", meta.SessionID)
}

func TestSessionRecordingAdapter_Load_Errors(t *testing.T) {
	a := NewSessionRecordingAdapter()

	_, _, err := a.Load(RecordingReference{ID: "/nonexistent/file.recording.json"})
	require.Error(t, err)

	bad := filepath.Join(t.TempDir(), "bad.recording.json")
	require.NoError(t, os.WriteFile(bad, []byte("{not json"), 0o600))
	_, _, err = a.Load(RecordingReference{ID: bad})
	require.Error(t, err)
}
