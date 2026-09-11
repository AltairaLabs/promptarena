package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/types"
)

// Two real run outputs: one shipped as an eval fixture, one captured from an
// actual `promptarena run` of the assertions-test example's tool-usage
// scenario (a tool loop, so it carries tool calls, tool results and per-turn
// assertions). The example's own out/ directory is gitignored, hence the copy.
const (
	exampleArenaEvalFixture = "../../examples/eval-test/recordings/customer-support.arena.json"
	toolLoopRunOutput       = "testdata/tool-usage.run.json"
)

func TestArenaOutputAdapter_CanHandle(t *testing.T) {
	a := NewArenaOutputAdapter()
	tests := []struct {
		source, hint string
		want         bool
	}{
		// What `promptarena run` actually writes.
		{"out/2026-08-31T19-48-12Z-0001_gemini_default_tool-usage_3a7b0b83_000b.json", "", true},
		{"out/*.json", "", true},
		{"recordings/customer-support.arena.json", "", true},
		{"whatever.txt", "arena_output", true},
		{"whatever.txt", "arena", true},
		// PromptKit session recordings are the other adapter's.
		{"session.recording.json", "", false},
		{"out/recordings/run.jsonl", "", false},
		{"conv.transcript.yaml", "", false},
		// A hint for another adapter must not be overridden by the extension.
		{"out/run.json", "session", false},
		{"out/run.json", "mock", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, a.CanHandle(tt.source, tt.hint), "%s / %q", tt.source, tt.hint)
	}
}

func TestArenaOutputAdapter_Load_EvalFixture(t *testing.T) {
	msgs, meta, err := NewArenaOutputAdapter().Load(RecordingReference{ID: exampleArenaEvalFixture})
	require.NoError(t, err)

	require.Len(t, msgs, 15)
	assert.Equal(t, "system", msgs[0].Role)
	assert.Equal(t, "user", msgs[1].Role)

	require.NotNil(t, meta)
	assert.Equal(t, "2025-12-21T14-24Z_claude-3-5-haiku_default_customer-support-scenarios_b47dd679", meta.SessionID)
	assert.Equal(t, "customer-support-scenarios", meta.Extras["scenario_id"])
	assert.Equal(t, "claude-3-5-haiku", meta.ProviderInfo["provider_id"])

	// Seven assistant messages each carry one content_matches assertion in
	// meta.assertions; they surface keyed by message index.
	require.Len(t, meta.TurnAssertions, 7)
	got := meta.TurnAssertions[2]
	require.Len(t, got, 1)
	assert.Equal(t, "content_matches", got[0].Type)
	require.NotNil(t, got[0].Passed)
	assert.True(t, *got[0].Passed)
	assert.Empty(t, meta.ConversationAssertions, "the fixture ran no conversation-level assertions")
}

func TestArenaOutputAdapter_Load_RealRunWithToolLoop(t *testing.T) {
	msgs, meta, err := NewArenaOutputAdapter().Load(RecordingReference{ID: toolLoopRunOutput})
	require.NoError(t, err)

	var toolCalls, toolResults int
	for _, m := range msgs {
		toolCalls += len(m.ToolCalls)
		if m.ToolResult != nil {
			toolResults++
		}
	}
	assert.Positive(t, toolCalls, "tool-loop run output keeps the assistant's tool calls")
	assert.Positive(t, toolResults, "tool-loop run output keeps the tool results")

	assert.Equal(t, "tool-usage", meta.Extras["scenario_id"])
	assert.Len(t, meta.Timestamps, len(msgs))

	var sawToolsCalled bool
	for _, results := range meta.TurnAssertions {
		for _, r := range results {
			if r.Type == "tools_called" {
				sawToolsCalled = true
				assert.Equal(t, map[string]any{"tools": []any{"search", "calculate"}}, r.Params,
					"the original assertion params ride along so a regression can be regenerated from them")
			}
		}
	}
	assert.True(t, sawToolsCalled)
}

// runOutputFixture writes a minimal run output in the shape engine.RunResult
// marshals to, with one failed turn assertion and one failed conversation
// assertion.
func runOutputFixture(t *testing.T, dir string) string {
	t.Helper()
	start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	out := map[string]any{
		"RunID":       "run-1",
		"PromptPack":  "support-pack",
		"Region":      "default",
		"ScenarioID":  "refund",
		"ProviderID":  "mock",
		"Params":      map[string]any{"temperature": 0.1},
		"SessionTags": []string{"nightly"},
		"StartTime":   start,
		"EndTime":     start.Add(2 * time.Second),
		"Duration":    int64(2 * time.Second),
		"Messages": []map[string]any{
			{"role": "user", "content": "I want a refund", "timestamp": start},
			{
				"role": "assistant", "content": "No.", "timestamp": start.Add(time.Second),
				"meta": map[string]any{
					"assertions": map[string]any{
						"failed": 1, "passed": false, "total": 1,
						"results": []map[string]any{{
							"type":    "content_includes",
							"passed":  false,
							"message": "Should acknowledge the refund policy.",
							"config":  map[string]any{"type": "content_includes", "params": map[string]any{"patterns": []string{"policy"}}},
							"details": map[string]any{"missing": []string{"policy"}, "eval_id": "assertion_0_content_includes"},
						}},
					},
				},
			},
		},
		"conversation_assertions": map[string]any{
			"failed": 1, "passed": false, "total": 1,
			"results": []map[string]any{{
				"type": "tools_called", "passed": false, "message": "refund tool was never called",
				"details": map[string]any{"missing": []string{"issue_refund"}},
			}},
		},
	}
	data, err := json.Marshal(out)
	require.NoError(t, err)
	path := filepath.Join(dir, "run-1.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestArenaOutputAdapter_Load_FailedAssertions(t *testing.T) {
	path := runOutputFixture(t, t.TempDir())

	msgs, meta, err := NewArenaOutputAdapter().Load(RecordingReference{ID: path})
	require.NoError(t, err)
	require.Len(t, msgs, 2)

	assert.Equal(t, "run-1", meta.SessionID)
	assert.Equal(t, []string{"nightly"}, meta.Tags)
	assert.Equal(t, 2*time.Second, meta.Duration)
	assert.Equal(t, "refund", meta.Extras["scenario_id"])
	assert.Equal(t, "support-pack", meta.Extras["prompt_pack"])
	assert.Equal(t, "default", meta.Extras["region"])
	assert.Equal(t, map[string]any{"temperature": 0.1}, meta.Extras["params"])

	require.Len(t, meta.ConversationAssertions, 1)
	conv := meta.ConversationAssertions[0]
	assert.Equal(t, "tools_called", conv.Type)
	require.NotNil(t, conv.Passed)
	assert.False(t, *conv.Passed)
	assert.Equal(t, "refund tool was never called", conv.Message)
	assert.Equal(t, []any{"issue_refund"}, conv.Details["missing"])

	require.Len(t, meta.TurnAssertions[1], 1)
	turn := meta.TurnAssertions[1][0]
	assert.Equal(t, "content_includes", turn.Type)
	require.NotNil(t, turn.Passed)
	assert.False(t, *turn.Passed)
	assert.Equal(t, map[string]any{"patterns": []any{"policy"}}, turn.Params)
	assert.Equal(t, "Should acknowledge the refund policy.", turn.Message)
}

func TestArenaOutputAdapter_Load_RejectsOtherJSON(t *testing.T) {
	dir := t.TempDir()
	a := NewArenaOutputAdapter()

	// Valid JSON, but not a run output: say so rather than return nothing.
	other := filepath.Join(dir, "other.json")
	require.NoError(t, os.WriteFile(other, []byte(`{"scenarios":{"x":{}}}`), 0o600))
	_, _, err := a.Load(RecordingReference{ID: other})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RunID")

	bad := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte("{not json"), 0o600))
	_, _, err = a.Load(RecordingReference{ID: bad})
	require.Error(t, err)

	_, _, err = a.Load(RecordingReference{ID: filepath.Join(dir, "missing.json")})
	require.Error(t, err)
}

// A run output whose Messages carry parts survives the round trip as
// types.Message, since the file is that type's own JSON.
func TestArenaOutputAdapter_Load_MessagesAreNative(t *testing.T) {
	msg := types.Message{Role: "user", Parts: []types.ContentPart{types.NewTextPart("hello")}}
	out := map[string]any{"RunID": "run-2", "Messages": []types.Message{msg}}
	data, err := json.Marshal(out)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "run-2.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	msgs, _, err := NewArenaOutputAdapter().Load(RecordingReference{ID: path})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "hello", msgs[0].GetContent())
}

// Top-level eval_results are pack evals: measurements with a score and no
// verdict. They surface as kind "eval" at conversation level.
func TestArenaOutputAdapter_Load_PackEvalResults(t *testing.T) {
	out := map[string]any{
		"RunID":    "run-ev",
		"Messages": []map[string]any{{"role": "user", "content": "hi"}, {"role": "assistant", "content": "yo"}},
		"eval_results": []map[string]any{
			{"eval_id": "faith", "type": "faithfulness", "kind": "eval", "score": 0.42, "explanation": "drifted"},
			{"eval_id": "gate", "type": "toxicity", "kind": "guardrail", "passed": false},
		},
	}
	data, err := json.Marshal(out)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "run-ev.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	_, meta, err := NewArenaOutputAdapter().Load(RecordingReference{ID: path})
	require.NoError(t, err)
	require.Len(t, meta.EvalResults, 2)

	faith := meta.EvalResults[0]
	assert.Equal(t, "faith", faith.ID)
	assert.Equal(t, "faithfulness", faith.Type)
	assert.Equal(t, "eval", faith.Kind)
	require.NotNil(t, faith.Score)
	assert.Equal(t, 0.42, *faith.Score)
	assert.Nil(t, faith.Passed)
	assert.Equal(t, "drifted", faith.Message)

	gate := meta.EvalResults[1]
	assert.Equal(t, "guardrail", gate.Kind)
	require.NotNil(t, gate.Passed)
	assert.False(t, *gate.Passed)
}

// Assertions recorded on messages and at conversation level carry their eval
// id and kind so the generate layer can tell a judged result from a measurement.
func TestArenaOutputAdapter_Load_AssertionKindAndID(t *testing.T) {
	path := runOutputFixture(t, t.TempDir())
	_, meta, err := NewArenaOutputAdapter().Load(RecordingReference{ID: path})
	require.NoError(t, err)
	conv := meta.ConversationAssertions[0]
	assert.Equal(t, "assertion", conv.Kind)
	turn := meta.TurnAssertions[1][0]
	assert.Equal(t, "assertion", turn.Kind)
	assert.Equal(t, "assertion_0_content_includes", turn.ID, "eval_id from details when the summary carries it")
}
