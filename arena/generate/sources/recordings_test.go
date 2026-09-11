package sources

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/PromptKit/runtime/recording"
	"github.com/AltairaLabs/promptarena/arena/generate"
)

var fixtureStart = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

// sessionRecordingFixture writes a PromptKit SessionRecording, the shape the
// recording middleware produces, with one user/assistant exchange.
func sessionRecordingFixture(t *testing.T, dir, name string) string {
	t.Helper()
	rec := &recording.SessionRecording{
		Metadata: recording.Metadata{
			SessionID: "sess-" + name, Version: "1.0",
			StartTime: fixtureStart, EndTime: fixtureStart.Add(time.Second),
			ProviderName: "test-provider",
			Custom:       map[string]any{"tags": []any{"test"}},
		},
	}
	for i, m := range []events.MessageCreatedData{
		{Role: "user", Content: "Hello from " + name},
		{Role: "assistant", Content: "Hi back from " + name},
	} {
		raw, err := json.Marshal(m)
		require.NoError(t, err)
		rec.Events = append(rec.Events, recording.RecordedEvent{
			Sequence: int64(i + 1), Type: events.EventMessageCreated,
			Timestamp: fixtureStart.Add(time.Duration(i) * time.Second), SessionID: rec.Metadata.SessionID, Data: raw,
		})
	}
	path := filepath.Join(dir, name+".recording.json")
	require.NoError(t, rec.SaveTo(path, recording.FormatJSON))
	return path
}

// runOutputFixture writes arena run output for a two-turn scenario. The
// second assistant message carries a failed turn assertion when failed is
// true, and the run carries one conversation-level assertion with the same
// verdict.
func runOutputFixture(t *testing.T, dir, name string, failed bool) string {
	t.Helper()
	assertion := func(typ, msg string, params map[string]any) map[string]any {
		return map[string]any{
			"type": typ, "passed": !failed, "message": msg,
			"config":  map[string]any{"type": typ, "params": params},
			"details": map[string]any{"score": 0},
		}
	}
	out := map[string]any{
		"RunID":      "run-" + name,
		"ScenarioID": "refund",
		"ProviderID": "mock",
		"PromptPack": "support",
		"StartTime":  fixtureStart,
		"EndTime":    fixtureStart.Add(4 * time.Second),
		"Messages": []map[string]any{
			{"role": "system", "content": "You are support.", "timestamp": fixtureStart},
			{"role": "user", "content": "I want a refund", "timestamp": fixtureStart.Add(time.Second)},
			{"role": "assistant", "content": "Let me check.", "timestamp": fixtureStart.Add(2 * time.Second)},
			{"role": "user", "content": "Well?", "timestamp": fixtureStart.Add(3 * time.Second)},
			{
				"role": "assistant", "content": "No.", "timestamp": fixtureStart.Add(4 * time.Second),
				"meta": map[string]any{"assertions": map[string]any{
					"failed": boolToInt(failed), "passed": !failed, "total": 1,
					"results": []map[string]any{
						assertion("content_includes", "Should cite the policy.", map[string]any{"patterns": []string{"policy"}}),
					},
				}},
			},
		},
		"conversation_assertions": map[string]any{
			"failed": boolToInt(failed), "passed": !failed, "total": 1,
			"results": []map[string]any{
				assertion("tools_called", "refund tool must be called", map[string]any{"tools": []string{"issue_refund"}}),
			},
		},
	}
	data, err := json.Marshal(out)
	require.NoError(t, err)
	path := filepath.Join(dir, name+".json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestRecordingsAdapter_Name(t *testing.T) {
	assert.Equal(t, "recordings", NewRecordingsAdapter("*.json").Name())
}

func TestRecordingsAdapter_ListFromSessionRecordings(t *testing.T) {
	dir := t.TempDir()
	sessionRecordingFixture(t, dir, "one")
	sessionRecordingFixture(t, dir, "two")

	adapter := NewRecordingsAdapter(filepath.Join(dir, "*.recording.json"))
	summaries, err := adapter.List(context.Background(), generate.ListOptions{})
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	assert.Equal(t, "sess-one", summaries[0].ID)
	assert.Equal(t, 2, summaries[0].TurnCount)
	assert.Equal(t, "test-provider", summaries[0].ProviderID)
	assert.Equal(t, fixtureStart, summaries[0].Timestamp)
	assert.False(t, summaries[0].HasFailures, "a session recording carries no assertion results")
}

// Run output is what `promptarena run` writes into out/, so a plain out/*.json
// glob is the everyday input.
func TestRecordingsAdapter_ListFromRunOutput(t *testing.T) {
	dir := t.TempDir()
	runOutputFixture(t, dir, "passing", false)
	runOutputFixture(t, dir, "failing", true)

	adapter := NewRecordingsAdapter(filepath.Join(dir, "*.json"))
	summaries, err := adapter.List(context.Background(), generate.ListOptions{})
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	byID := map[string]generate.SessionSummary{}
	for _, s := range summaries {
		byID[s.ID] = s
	}
	assert.True(t, byID["run-failing"].HasFailures)
	assert.False(t, byID["run-passing"].HasFailures)
	assert.Equal(t, "refund", byID["run-failing"].ScenarioID)
	assert.Equal(t, "mock", byID["run-failing"].ProviderID)
}

func TestRecordingsAdapter_ListFilterPassed(t *testing.T) {
	dir := t.TempDir()
	runOutputFixture(t, dir, "passing", false)
	runOutputFixture(t, dir, "failing", true)
	adapter := NewRecordingsAdapter(filepath.Join(dir, "*.json"))

	failedOnly := false
	summaries, err := adapter.List(context.Background(), generate.ListOptions{FilterPassed: &failedOnly})
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "run-failing", summaries[0].ID)

	passedOnly := true
	summaries, err = adapter.List(context.Background(), generate.ListOptions{FilterPassed: &passedOnly})
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "run-passing", summaries[0].ID)
}

func TestRecordingsAdapter_ListWithLimit(t *testing.T) {
	dir := t.TempDir()
	sessionRecordingFixture(t, dir, "one")
	sessionRecordingFixture(t, dir, "two")
	sessionRecordingFixture(t, dir, "three")

	adapter := NewRecordingsAdapter(filepath.Join(dir, "*.recording.json"))
	summaries, err := adapter.List(context.Background(), generate.ListOptions{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, summaries, 2)
}

func TestRecordingsAdapter_GetSessionRecording(t *testing.T) {
	dir := t.TempDir()
	path := sessionRecordingFixture(t, dir, "test")

	detail, err := NewRecordingsAdapter(filepath.Join(dir, "*.recording.json")).Get(context.Background(), path)
	require.NoError(t, err)

	assert.Equal(t, "sess-test", detail.ID)
	assert.Len(t, detail.Messages, 2)
	assert.Equal(t, "user", detail.Messages[0].Role)
	assert.Equal(t, []string{"test"}, detail.Tags)
	assert.Equal(t, fixtureStart, detail.Timestamp)
	assert.Empty(t, detail.EvalResults)
	assert.Empty(t, detail.TurnEvalResults)
}

// Assertion results from run output land where the converter reads them:
// conversation-level as EvalResults, per-turn keyed by the USER turn they
// answer (the converter indexes scenario turns, which are user turns), with
// the original params so the regenerated scenario asserts the same thing.
func TestRecordingsAdapter_GetRunOutputCarriesAssertions(t *testing.T) {
	dir := t.TempDir()
	path := runOutputFixture(t, dir, "failing", true)

	detail, err := NewRecordingsAdapter(filepath.Join(dir, "*.json")).Get(context.Background(), path)
	require.NoError(t, err)

	assert.Equal(t, "run-failing", detail.ID)
	assert.Equal(t, "refund", detail.ScenarioID)
	assert.True(t, detail.HasFailures)
	assert.Len(t, detail.Messages, 5)

	require.Len(t, detail.EvalResults, 1)
	assert.Equal(t, "tools_called", detail.EvalResults[0].Type)
	assert.False(t, detail.EvalResults[0].Passed)
	assert.Equal(t, "refund tool must be called", detail.EvalResults[0].Message)

	// Message index 4 is the assistant reply to the second user turn.
	require.Len(t, detail.TurnEvalResults, 1)
	turn, ok := detail.TurnEvalResults[1]
	require.True(t, ok, "turn assertions keyed by user-turn ordinal, got %v", detail.TurnEvalResults)
	require.Len(t, turn, 1)
	assert.Equal(t, "content_includes", turn[0].Type)
	assert.False(t, turn[0].Passed)
	assert.Equal(t, map[string]any{"patterns": []any{"policy"}}, turn[0].Params)
}

// The CLI lists, then calls Get with each summary's ID. For run output that ID
// is the RunID, not the path, so Get must resolve it; and a fresh adapter that
// never listed must still find it.
func TestRecordingsAdapter_GetBySessionID(t *testing.T) {
	dir := t.TempDir()
	runOutputFixture(t, dir, "failing", true)

	listed := NewRecordingsAdapter(filepath.Join(dir, "*.json"))
	summaries, err := listed.List(context.Background(), generate.ListOptions{})
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	detail, err := listed.Get(context.Background(), summaries[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "run-failing", detail.ID)

	fresh := NewRecordingsAdapter(filepath.Join(dir, "*.json"))
	detail, err = fresh.Get(context.Background(), "run-failing")
	require.NoError(t, err)
	assert.Equal(t, "run-failing", detail.ID)

	_, err = fresh.Get(context.Background(), "run-unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "run-unknown")
}

func TestRecordingsAdapter_GetMissingFile(t *testing.T) {
	_, err := NewRecordingsAdapter("*.json").Get(context.Background(), "/nonexistent/file.json")
	require.Error(t, err)
}
