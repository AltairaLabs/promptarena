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

	"github.com/AltairaLabs/promptarena/arena/generate"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/recording"
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
			"details": map[string]any{"score": 0, "eval_id": "assertion_0_" + typ},
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
		"eval_results": []map[string]any{
			{"eval_id": "faith", "type": "faithfulness", "kind": "eval", "score": 0.42},
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
	assert.Empty(t, detail.Evals, "a session recording carries no assertion results")
}

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

func TestRecordingsAdapter_GetRunOutputFillsModel(t *testing.T) {
	dir := t.TempDir()
	path := runOutputFixture(t, dir, "failing", true)

	detail, err := NewRecordingsAdapter(filepath.Join(dir, "*.json")).Get(context.Background(), path)
	require.NoError(t, err)

	require.NotNil(t, detail.Pack)
	assert.Equal(t, "support", detail.Pack.Name)
	assert.Nil(t, detail.Workflow, "run output records no transitions")

	// Two judged results (conversation + turn) and one measurement.
	require.Len(t, detail.Evals, 3)
	byID := map[string]generate.EvalResult{}
	for _, e := range detail.Evals {
		byID[e.ID] = e
	}

	turn := byID["assertion_0_content_includes"]
	assert.Equal(t, "assertion", turn.Kind)
	require.NotNil(t, turn.Passed)
	assert.False(t, *turn.Passed)
	require.NotNil(t, turn.Turn)
	assert.Equal(t, 1, *turn.Turn, "message index 4 answers the second user turn")
	assert.Equal(t, map[string]any{"patterns": []any{"policy"}}, turn.Params)

	faith := byID["faith"]
	assert.Equal(t, "eval", faith.Kind)
	assert.Nil(t, faith.Passed)
	require.NotNil(t, faith.Score)
	assert.Equal(t, 0.42, *faith.Score)
	assert.Nil(t, faith.Turn)

	conv := byID["assertion_0_tools_called"]
	assert.Nil(t, conv.Turn)
	assert.True(t, conv.Failed())
	assert.True(t, detail.HasFailures)
}

// Run output records template variables under Params; only string values are
// variables (numbers there are provider params).
func TestRecordingsAdapter_GetVariables(t *testing.T) {
	dir := t.TempDir()
	out := map[string]any{
		"RunID":    "run-vars",
		"Params":   map[string]any{"customer_tier": "gold", "temperature": 0.1},
		"Messages": []map[string]any{{"role": "user", "content": "hi"}},
	}
	data, err := json.Marshal(out)
	require.NoError(t, err)
	path := filepath.Join(dir, "run-vars.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	detail, err := NewRecordingsAdapter(filepath.Join(dir, "*.json")).Get(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"customer_tier": "gold"}, detail.Variables)
}

// An expectation selects sessions whose measurement fell outside it, and a
// session with no such measurement is excluded too: it cannot satisfy it.
func TestRecordingsAdapter_ListExpectations(t *testing.T) {
	dir := t.TempDir()
	runOutputFixture(t, dir, "low", true)   // faith 0.42
	runOutputFixture(t, dir, "high", false) // faith 0.42 too: same fixture score
	adapter := NewRecordingsAdapter(filepath.Join(dir, "*.json"))

	summaries, err := adapter.List(context.Background(), generate.ListOptions{
		Expectations: []generate.Expectation{{EvalID: "faith", Min: f(0.8)}},
	})
	require.NoError(t, err)
	assert.Len(t, summaries, 2, "both fixtures score 0.42 < 0.8")
	for _, s := range summaries {
		assert.True(t, s.HasFailures, "an expectation violation counts as a failure")
	}

	summaries, err = adapter.List(context.Background(), generate.ListOptions{
		Expectations: []generate.Expectation{{EvalID: "faith", Min: f(0.4)}},
	})
	require.NoError(t, err)
	assert.Empty(t, summaries, "0.42 satisfies >= 0.4")
}

func TestRecordingsAdapter_GetWorkflowFromSessionRecording(t *testing.T) {
	dir := t.TempDir()
	rec := &recording.SessionRecording{
		Metadata: recording.Metadata{SessionID: "wf", Version: "1.0", StartTime: fixtureStart, EndTime: fixtureStart},
	}
	user, err := json.Marshal(events.MessageCreatedData{Role: "user", Content: "refund"})
	require.NoError(t, err)
	tr, err := json.Marshal(events.WorkflowTransitionedData{FromState: "triage", ToState: "refunds", Event: "Escalate", PromptTask: "refunds_prompt"})
	require.NoError(t, err)
	rec.Events = []recording.RecordedEvent{
		{Sequence: 1, Type: events.EventMessageCreated, Timestamp: fixtureStart, SessionID: "wf", Data: user},
		{Sequence: 2, Type: events.EventWorkflowTransitioned, Timestamp: fixtureStart, SessionID: "wf", Data: tr},
	}
	path := filepath.Join(dir, "wf.recording.json")
	require.NoError(t, rec.SaveTo(path, recording.FormatJSON))

	detail, err := NewRecordingsAdapter(filepath.Join(dir, "*.recording.json")).Get(context.Background(), path)
	require.NoError(t, err)
	require.NotNil(t, detail.Workflow)
	assert.Equal(t, "triage", detail.Workflow.EntryState)
	assert.Empty(t, detail.Workflow.EntryPromptTask, "the event carries the destination's prompt task, not the origin's")
	require.Len(t, detail.Workflow.Transitions, 1)
	assert.Equal(t, generate.WorkflowTransition{From: "triage", To: "refunds", Event: "Escalate", PromptTask: "refunds_prompt", MessageIndex: 0}, detail.Workflow.Transitions[0])
}

func f(v float64) *float64 { return &v }
