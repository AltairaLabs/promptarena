package flow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/arena/generate"
	"github.com/AltairaLabs/promptarena/deploy"
)

// fakeSessions is an in-process sessionsClient.
type fakeSessions struct {
	info    *deploy.ProviderInfo
	infoErr error
	pages   []deploy.ListSessionsResponse
	listErr error
	detail  *deploy.SessionDetail
	getErr  error

	gotLists []*deploy.ListSessionsRequest
	gotGet   *deploy.GetSessionRequest
	closed   bool
}

func (f *fakeSessions) GetProviderInfo(context.Context) (*deploy.ProviderInfo, error) {
	return f.info, f.infoErr
}

func (f *fakeSessions) ListSessions(_ context.Context, req *deploy.ListSessionsRequest) (*deploy.ListSessionsResponse, error) {
	copied := *req
	f.gotLists = append(f.gotLists, &copied)
	if f.listErr != nil {
		return nil, f.listErr
	}
	idx := len(f.gotLists) - 1
	if idx >= len(f.pages) {
		return &deploy.ListSessionsResponse{}, nil
	}
	return &f.pages[idx], nil
}

func (f *fakeSessions) GetSession(_ context.Context, req *deploy.GetSessionRequest) (*deploy.GetSessionResponse, error) {
	f.gotGet = req
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &deploy.GetSessionResponse{Session: *f.detail}, nil
}

func (f *fakeSessions) Close() error { f.closed = true; return nil }

func withSessions() *deploy.ProviderInfo {
	return &deploy.ProviderInfo{Name: "omnia", Version: "1.6.0", Capabilities: []string{deploy.LoginCapability, deploy.SessionsCapability}}
}

func TestNewSessionSource_CapabilityPresent(t *testing.T) {
	c := &fakeSessions{info: withSessions()}
	src, err := newSessionSource(context.Background(), c, "omnia", "default", `{"workspace":"demo"}`)
	require.NoError(t, err)
	assert.Equal(t, "omnia", src.Name())
	assert.False(t, c.closed)
	require.NoError(t, src.Close())
	assert.True(t, c.closed)
}

func TestNewSessionSource_CapabilityAbsent(t *testing.T) {
	c := &fakeSessions{info: &deploy.ProviderInfo{Name: "omnia", Version: "1.5.1", Capabilities: []string{deploy.LoginCapability}}}
	_, err := newSessionSource(context.Background(), c, "omnia", "default", "{}")
	require.Error(t, err)
	assert.Equal(t, `adapter "omnia" v1.5.1 does not support session sourcing; upgrade with: promptarena deploy adapter install omnia`, err.Error())
	assert.True(t, c.closed, "a rejected adapter must not be left running")
}

func TestNewSessionSource_ProviderInfoError(t *testing.T) {
	c := &fakeSessions{infoErr: errors.New("boom")}
	_, err := newSessionSource(context.Background(), c, "omnia", "default", "{}")
	require.Error(t, err)
	assert.True(t, c.closed)
}

func TestSessionSource_ListFollowsPagesAndPassesFilters(t *testing.T) {
	c := &fakeSessions{
		info: withSessions(),
		pages: []deploy.ListSessionsResponse{
			{Sessions: []deploy.SessionSummary{{ID: "a", HasFailures: true, ScenarioID: "refund"}}, NextCursor: "p2"},
			{Sessions: []deploy.SessionSummary{{ID: "b"}, {ID: "c"}}, NextCursor: "p3"},
			{Sessions: []deploy.SessionSummary{{ID: "d"}}},
		},
	}
	src, err := newSessionSource(context.Background(), c, "omnia", "staging", `{"workspace":"demo"}`)
	require.NoError(t, err)

	no := false
	exp := []generate.Expectation{{EvalID: "faith", Min: f(0.8)}}
	got, err := src.List(context.Background(), generate.ListOptions{FilterPassed: &no, FilterEvalType: "faithfulness", Expectations: exp})
	require.NoError(t, err)
	require.Len(t, got, 4, "all pages are followed when there is no limit")
	assert.Equal(t, "a", got[0].ID)
	assert.Equal(t, "a", got[0].Source)
	assert.Equal(t, "refund", got[0].ScenarioID)
	assert.True(t, got[0].HasFailures)

	require.Len(t, c.gotLists, 3)
	first := c.gotLists[0]
	assert.Equal(t, `{"workspace":"demo"}`, first.DeployConfig)
	assert.Equal(t, "staging", first.Environment)
	require.NotNil(t, first.FilterPassed)
	assert.False(t, *first.FilterPassed)
	assert.Equal(t, "faithfulness", first.FilterEvalType)
	require.Len(t, first.Expectations, 1)
	assert.Equal(t, "faith", first.Expectations[0].EvalID)
	assert.Equal(t, 0.8, *first.Expectations[0].Min)
	assert.Empty(t, first.Cursor)
	assert.Equal(t, "p2", c.gotLists[1].Cursor)
	assert.Equal(t, "p3", c.gotLists[2].Cursor)
}

func TestSessionSource_ListStopsAtLimit(t *testing.T) {
	c := &fakeSessions{
		info: withSessions(),
		pages: []deploy.ListSessionsResponse{
			{Sessions: []deploy.SessionSummary{{ID: "a"}, {ID: "b"}}, NextCursor: "p2"},
			{Sessions: []deploy.SessionSummary{{ID: "c"}}},
		},
	}
	src, err := newSessionSource(context.Background(), c, "omnia", "default", "{}")
	require.NoError(t, err)
	got, err := src.List(context.Background(), generate.ListOptions{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Len(t, c.gotLists, 1, "no second page is fetched once the limit is met")
	assert.Equal(t, 2, c.gotLists[0].Limit)
}

// An adapter that advertised the capability but does not serve the method is
// reported as unsupported, not as a bare RPC failure.
func TestSessionSource_MethodNotSupportedBecomesUpgradeGuidance(t *testing.T) {
	c := &fakeSessions{info: withSessions(), listErr: deploy.ErrMethodNotSupported}
	src, err := newSessionSource(context.Background(), c, "omnia", "default", "{}")
	require.NoError(t, err)
	_, err = src.List(context.Background(), generate.ListOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support session sourcing")
	assert.Contains(t, err.Error(), "v1.6.0")

	c.listErr = errors.New("502 from gateway")
	_, err = src.List(context.Background(), generate.ListOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502 from gateway")
	assert.Contains(t, err.Error(), `adapter "omnia"`)
}

func TestSessionSource_GetMapsEveryField(t *testing.T) {
	score, passed, turn := 0.42, false, 1
	when := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	c := &fakeSessions{info: withSessions(), detail: &deploy.SessionDetail{
		SessionSummary: deploy.SessionSummary{ID: "s-1", ScenarioID: "refund", ProviderID: "claude", Timestamp: when, TurnCount: 3, HasFailures: true, Tags: []string{"prod"}},
		Messages: json.RawMessage(`[
			{"role":"user","content":"refund please"},
			{"role":"assistant","tool_calls":[{"id":"c1","name":"issue_refund","args":{"amount":10}}]},
			{"role":"tool","tool_result":{"id":"c1","name":"issue_refund","parts":[{"type":"text","text":"done"}]}}
		]`),
		Pack:      &deploy.SessionPack{Name: "support", Version: "1.4.2", Digest: "sha256:abc"},
		Variables: map[string]string{"tier": "gold"},
		Workflow: &deploy.SessionWorkflow{EntryState: "triage", EntryPromptTask: "triage_prompt",
			Transitions: []deploy.SessionTransition{{From: "triage", To: "refunds", Event: "Escalate", PromptTask: "refunds_prompt", MessageIndex: 1}}},
		Evals: []deploy.SessionEval{
			{ID: "faith", Type: "faithfulness", Kind: "eval", Score: &score, Threshold: &deploy.SessionThreshold{Operator: "gte", Value: 0.8}},
			{ID: "a0", Type: "content_includes", Kind: "assertion", Passed: &passed, Turn: &turn, Params: map[string]any{"patterns": []any{"policy"}}, Message: "cite"},
		},
	}}
	src, err := newSessionSource(context.Background(), c, "omnia", "default", `{"workspace":"demo"}`)
	require.NoError(t, err)

	d, err := src.Get(context.Background(), "s-1")
	require.NoError(t, err)
	assert.Equal(t, "s-1", c.gotGet.SessionID)
	assert.Equal(t, `{"workspace":"demo"}`, c.gotGet.DeployConfig)

	assert.Equal(t, "s-1", d.ID)
	assert.Equal(t, "refund", d.ScenarioID)
	assert.Equal(t, when, d.Timestamp)
	require.Len(t, d.Messages, 3)
	assert.Equal(t, "refund please", d.Messages[0].GetContent())
	require.Len(t, d.Messages[1].ToolCalls, 1)
	assert.Equal(t, "issue_refund", d.Messages[1].ToolCalls[0].Name)
	require.NotNil(t, d.Messages[2].ToolResult)
	assert.Equal(t, "done", d.Messages[2].GetContent())

	assert.Equal(t, &generate.PackRef{Name: "support", Version: "1.4.2", Digest: "sha256:abc"}, d.Pack)
	assert.Equal(t, map[string]string{"tier": "gold"}, d.Variables)
	require.NotNil(t, d.Workflow)
	assert.Equal(t, "triage_prompt", d.Workflow.EntryPromptTask)
	assert.Equal(t, generate.WorkflowTransition{From: "triage", To: "refunds", Event: "Escalate", PromptTask: "refunds_prompt", MessageIndex: 1}, d.Workflow.Transitions[0])

	require.Len(t, d.Evals, 2)
	assert.Equal(t, "eval", d.Evals[0].Kind)
	assert.Nil(t, d.Evals[0].Passed)
	assert.Equal(t, 0.42, *d.Evals[0].Score)
	assert.Equal(t, &generate.EvalThreshold{Operator: "gte", Value: 0.8}, d.Evals[0].Threshold)
	assert.True(t, d.Evals[1].Failed())
	assert.Equal(t, 1, *d.Evals[1].Turn)
	assert.Equal(t, map[string]any{"patterns": []any{"policy"}}, d.Evals[1].Params)
}

func TestSessionSource_GetToleratesNullMessages(t *testing.T) {
	c := &fakeSessions{info: withSessions(), detail: &deploy.SessionDetail{SessionSummary: deploy.SessionSummary{ID: "empty"}, Messages: json.RawMessage("null")}}
	src, err := newSessionSource(context.Background(), c, "omnia", "default", "{}")
	require.NoError(t, err)
	d, err := src.Get(context.Background(), "empty")
	require.NoError(t, err)
	assert.Empty(t, d.Messages)

	c.detail.Messages = json.RawMessage(`{"not":"an array"}`)
	_, err = src.Get(context.Background(), "empty")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding messages")
}

func TestWithWorkspace(t *testing.T) {
	out, err := withWorkspace(`{"api_endpoint":"https://x","workspace":"old"}`, "new")
	require.NoError(t, err)
	assert.JSONEq(t, `{"api_endpoint":"https://x","workspace":"new"}`, out)

	same, err := withWorkspace(`{"a":1}`, "")
	require.NoError(t, err)
	assert.Equal(t, `{"a":1}`, same, "no override leaves the document byte-for-byte")

	_, err = withWorkspace(`[]`, "demo")
	require.Error(t, err)
}

// Without an installed adapter, OpenSessionSource fails at Connect with the
// install guidance, before it ever reads the config.
func TestOpenSessionSource_AdapterMissing(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "arena.yaml")
	cfg := "apiVersion: promptkit.altairalabs.ai/v1alpha1\nkind: Arena\n" +
		"metadata:\n  name: t\nspec:\n  prompt_configs: []\n  providers: []\n" +
		"  defaults:\n    temperature: 0.7\n    max_tokens: 100\n" +
		"  deploy:\n    provider: nonexistent\n    config:\n      api_endpoint: https://x\n      workspace: demo\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0o600))
	t.Setenv("PATH", dir) // no adapter binaries anywhere on the path

	_, err := OpenSessionSource(context.Background(), "nonexistent", Options{ConfigPath: cfgPath, ProjectDir: dir}, "demo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "promptarena deploy adapter install nonexistent")
}

func f(v float64) *float64 { return &v }
