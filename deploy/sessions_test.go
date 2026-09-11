package deploy

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionDetail_RoundTrip(t *testing.T) {
	score, passed, turn := 0.42, false, 1
	in := SessionDetail{
		SessionSummary: SessionSummary{
			ID: "s-1", ScenarioID: "refund", ProviderID: "claude", TurnCount: 4, HasFailures: true,
			Timestamp: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), Tags: []string{"prod"},
			Metadata: map[string]any{"cohort": "canary"},
		},
		Messages:  json.RawMessage(`[{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"c1","name":"lookup","args":{"q":1}}]}]`),
		Pack:      &SessionPack{Name: "support", Version: "1.4.2", Digest: "sha256:abc"},
		Variables: map[string]string{"tier": "gold"},
		Workflow: &SessionWorkflow{EntryState: "triage", EntryPromptTask: "triage_prompt",
			Transitions: []SessionTransition{{From: "triage", To: "refunds", Event: "Escalate", PromptTask: "refunds_prompt", MessageIndex: 1}}},
		Evals: []SessionEval{
			{ID: "faith", Type: "faithfulness", Kind: "eval", Score: &score, Threshold: &SessionThreshold{Operator: "gte", Value: 0.8}},
			{ID: "a0", Type: "content_includes", Kind: "assertion", Passed: &passed, Turn: &turn,
				Params: map[string]any{"patterns": []any{"policy"}}, Message: "cite policy", Details: map[string]any{"missing": []any{"policy"}}},
		},
	}

	data, err := json.Marshal(in)
	require.NoError(t, err)
	var out SessionDetail
	require.NoError(t, json.Unmarshal(data, &out))

	assert.Equal(t, in.SessionSummary, out.SessionSummary)
	assert.JSONEq(t, string(in.Messages), string(out.Messages))
	assert.Equal(t, in.Pack, out.Pack)
	assert.Equal(t, in.Variables, out.Variables)
	assert.Equal(t, in.Workflow, out.Workflow)
	require.Len(t, out.Evals, 2)
	assert.Equal(t, 0.42, *out.Evals[0].Score)
	assert.Nil(t, out.Evals[0].Passed, "a measurement keeps its nil verdict")
	assert.Equal(t, in.Evals[0].Threshold, out.Evals[0].Threshold)
	require.NotNil(t, out.Evals[1].Passed)
	assert.False(t, *out.Evals[1].Passed)
	assert.Equal(t, 1, *out.Evals[1].Turn)
	assert.Equal(t, in.Evals[1].Params, out.Evals[1].Params)
}

// Optional request fields stay off the wire when unset, so an adapter can
// tell "no filter" from a zero value.
func TestListSessionsRequest_OptionalFieldsOmitted(t *testing.T) {
	data, err := json.Marshal(ListSessionsRequest{DeployConfig: "{}", Environment: "default"})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	for _, k := range []string{"filter_passed", "filter_eval_type", "expectations", "limit", "cursor"} {
		_, present := m[k]
		assert.False(t, present, k)
	}
	assert.Equal(t, "{}", m["deploy_config"])

	no := false
	data, err = json.Marshal(ListSessionsRequest{FilterPassed: &no, Limit: 5, Cursor: "c"})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, false, m["filter_passed"])
	assert.Equal(t, float64(5), m["limit"])
	assert.Equal(t, "c", m["cursor"])
}

func TestListSessionsResponse_NextCursorOmittedWhenEmpty(t *testing.T) {
	data, err := json.Marshal(ListSessionsResponse{Sessions: []SessionSummary{{ID: "a"}}})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "next_cursor")
}
