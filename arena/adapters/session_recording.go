package adapters

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/recording"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// SessionRecordingAdapter loads PromptKit session recordings: the artifact the
// recording middleware produces through the event store, either exported as a
// *.recording.json SessionRecording or left as the store's per-session JSONL.
// Parsing is delegated to recording.Load, which understands every shape the
// runtime writes, so this adapter cannot drift from the writer again.
type SessionRecordingAdapter struct{}

// NewSessionRecordingAdapter creates a new session recording adapter.
func NewSessionRecordingAdapter() *SessionRecordingAdapter {
	return &SessionRecordingAdapter{}
}

// CanHandle returns true for *.recording.json or *.jsonl files, or a
// "session" type hint.
func (a *SessionRecordingAdapter) CanHandle(source, typeHint string) bool {
	if typeHint != "" {
		// An explicit hint names the format; never steal a hinted source by
		// extension from the adapter the hint was meant for.
		return matchesTypeHint(typeHint, "session", "recording", "session_recording")
	}
	return hasExtension(source, ".recording.json", ".jsonl")
}

// Enumerate expands a source into individual recording references.
// For file-based sources, this expands glob patterns to matching files.
func (a *SessionRecordingAdapter) Enumerate(source string) ([]RecordingReference, error) {
	return EnumerateFiles(source, "session")
}

// Load reads a session recording and returns its messages: one per
// message.created event, in order. Every other event type is skipped.
func (a *SessionRecordingAdapter) Load(ref RecordingReference) ([]types.Message, *RecordingMetadata, error) {
	rec, err := recording.Load(ref.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load session recording %s: %w", ref.ID, err)
	}

	messages := make([]types.Message, 0, len(rec.Events))
	timestamps := make([]time.Time, 0, len(rec.Events))
	var transitions []RecordedTransition
	for i := range rec.Events {
		ev := &rec.Events[i]
		// Every event type other than these two (tool calls, provider calls,
		// audio, ...) is telemetry about the conversation, not part of it.
		if ev.Type == events.EventMessageCreated {
			msg, mErr := messageFromEvent(ev)
			if mErr != nil {
				return nil, nil, fmt.Errorf("event %d in %s: %w", ev.Sequence, ref.ID, mErr)
			}
			messages = append(messages, msg)
			timestamps = append(timestamps, ev.Timestamp)
			continue
		}
		if ev.Type == events.EventWorkflowTransitioned {
			var data events.WorkflowTransitionedData
			if err := json.Unmarshal(ev.Data, &data); err != nil {
				return nil, nil, fmt.Errorf("event %d in %s: decoding workflow.transitioned: %w", ev.Sequence, ref.ID, err)
			}
			transitions = append(transitions, RecordedTransition{
				From: data.FromState, To: data.ToState, Event: data.Event, PromptTask: data.PromptTask,
				MessageIndex: len(messages) - 1,
			})
		}
	}

	meta := metadataFromRecording(&rec.Metadata, timestamps)
	meta.WorkflowTransitions = transitions
	return messages, meta, nil
}

// messageFromEvent decodes a message.created payload into a types.Message.
func messageFromEvent(ev *recording.RecordedEvent) (types.Message, error) {
	var data events.MessageCreatedData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		return types.Message{}, fmt.Errorf("decoding message.created: %w", err)
	}

	msg := types.Message{
		Role:      data.Role,
		Content:   data.Content,
		Parts:     data.Parts,
		Timestamp: ev.Timestamp,
	}
	if len(data.ToolCalls) > 0 {
		msg.ToolCalls = make([]types.MessageToolCall, len(data.ToolCalls))
		for i, tc := range data.ToolCalls {
			msg.ToolCalls[i] = types.MessageToolCall{ID: tc.ID, Name: tc.Name, Args: json.RawMessage(tc.Args)}
		}
	}
	if data.ToolResult != nil {
		msg.ToolResult = &types.MessageToolResult{
			ID:        data.ToolResult.ID,
			Name:      data.ToolResult.Name,
			Parts:     data.ToolResult.Parts,
			Error:     data.ToolResult.Error,
			LatencyMs: data.ToolResult.LatencyMs,
		}
		// A tool message's text lives in the result parts; surface it as the
		// message content too so GetContent and text-only assertions see it.
		if msg.Content == "" && len(msg.Parts) == 0 {
			msg.Parts = data.ToolResult.Parts
		}
	}
	return msg, nil
}

// metadataFromRecording lifts session-level fields off the recording.
func metadataFromRecording(m *recording.Metadata, timestamps []time.Time) *RecordingMetadata {
	meta := &RecordingMetadata{
		SessionID:  m.SessionID,
		Timestamps: timestamps,
		Duration:   m.Duration,
		Extras:     make(map[string]interface{}),
	}
	if meta.Duration == 0 && len(timestamps) > 1 {
		meta.Duration = timestamps[len(timestamps)-1].Sub(timestamps[0])
	}
	if m.ProviderName != "" || m.Model != "" {
		meta.ProviderInfo = map[string]interface{}{
			providerInfoIDKey:    m.ProviderName,
			providerInfoModelKey: m.Model,
		}
	}
	if m.ConversationID != "" {
		meta.Extras["conversation_id"] = m.ConversationID
	}
	for k, v := range m.Custom {
		if k == "tags" {
			meta.Tags = stringSlice(v)
			continue
		}
		meta.Extras[k] = v
	}
	return meta
}

// stringSlice coerces a JSON-decoded list (which arrives as []any) or a Go
// []string into []string; anything else yields nil.
func stringSlice(v interface{}) []string {
	switch list := v.(type) {
	case []string:
		return list
	case []interface{}:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
