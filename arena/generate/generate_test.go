package generate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// fakeSource is an in-process SessionSourceAdapter.
type fakeSource struct {
	summaries []SessionSummary
	details   map[string]*SessionDetail
	getErr    map[string]error
	listErr   error
	gotList   ListOptions
}

func (f *fakeSource) Name() string { return "fake" }
func (f *fakeSource) List(_ context.Context, opts ListOptions) ([]SessionSummary, error) {
	f.gotList = opts
	return f.summaries, f.listErr
}
func (f *fakeSource) Get(_ context.Context, id string) (*SessionDetail, error) {
	if err, ok := f.getErr[id]; ok {
		return nil, err
	}
	d, ok := f.details[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return d, nil
}

func session(id, content string, evals ...EvalResult) *SessionDetail {
	return &SessionDetail{
		SessionSummary: SessionSummary{ID: id},
		Messages:       []types.Message{{Role: "user", Content: content}, {Role: "assistant", Content: "ok"}},
		Evals:          evals,
	}
}

func TestGenerate_ConvertsEverySession(t *testing.T) {
	no := false
	src := &fakeSource{
		summaries: []SessionSummary{{ID: "a"}, {ID: "b"}},
		details: map[string]*SessionDetail{
			"a": session("a", "one", EvalResult{ID: "x", Type: "content_includes", Kind: "assertion", Passed: &no}),
			"b": session("b", "two"),
		},
	}
	res, err := Generate(context.Background(), Request{Source: src})
	require.NoError(t, err)
	require.Len(t, res.Scenarios, 2)
	assert.Equal(t, "a", res.Scenarios[0].Metadata.Name)
	assert.Empty(t, res.Skipped)
	require.Len(t, res.Decisions, 1)
	assert.Equal(t, DecisionAssertedVerdict, res.Decisions[0].Outcome)
}

func TestGenerate_SkipsUnloadableAndUnconvertible(t *testing.T) {
	src := &fakeSource{
		summaries: []SessionSummary{{ID: "gone"}, {ID: "ok"}},
		details:   map[string]*SessionDetail{"ok": session("ok", "hi")},
		getErr:    map[string]error{"gone": errors.New("boom")},
	}
	res, err := Generate(context.Background(), Request{Source: src})
	require.NoError(t, err)
	require.Len(t, res.Scenarios, 1)
	require.Len(t, res.Skipped, 1)
	assert.Equal(t, "gone", res.Skipped[0].SessionID)
	assert.Contains(t, res.Skipped[0].Reason, "boom")
}

func TestGenerate_ListErrorIsFatal(t *testing.T) {
	_, err := Generate(context.Background(), Request{Source: &fakeSource{listErr: errors.New("offline")}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "offline")
}

func TestGenerate_NilSource(t *testing.T) {
	_, err := Generate(context.Background(), Request{})
	require.Error(t, err)
}

func TestGenerate_Dedup(t *testing.T) {
	src := &fakeSource{
		summaries: []SessionSummary{{ID: "a"}, {ID: "b"}},
		details:   map[string]*SessionDetail{"a": session("a", "same"), "b": session("b", "same")},
	}
	res, err := Generate(context.Background(), Request{Source: src, Dedup: true})
	require.NoError(t, err)
	assert.Len(t, res.Scenarios, 1)
	assert.Equal(t, 1, res.Deduplicated)

	res, err = Generate(context.Background(), Request{Source: src, Dedup: false})
	require.NoError(t, err)
	assert.Len(t, res.Scenarios, 2)
	assert.Equal(t, 0, res.Deduplicated)
}

func TestGenerate_WarningsAggregate(t *testing.T) {
	multi := session("m", "one")
	multi.Messages = append(multi.Messages, types.Message{Role: "user", Content: "two"})
	src := &fakeSource{summaries: []SessionSummary{{ID: "m"}}, details: map[string]*SessionDetail{"m": multi}}
	res, err := Generate(context.Background(), Request{Source: src})
	require.NoError(t, err)
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "2 user turns")
}

// One expectation does both jobs: it is passed to List for selection and used
// by the converter for the bound, unless the caller set Convert.Expectations
// separately.
func TestGenerate_ExpectationsFlowToListAndConvert(t *testing.T) {
	src := &fakeSource{
		summaries: []SessionSummary{{ID: "a"}},
		details:   map[string]*SessionDetail{"a": session("a", "hi", EvalResult{ID: "faith", Type: "faithfulness", Kind: "eval", Score: f(0.3)})},
	}
	exp := []Expectation{{EvalID: "faith", Min: f(0.8)}}
	res, err := Generate(context.Background(), Request{Source: src, List: ListOptions{Expectations: exp}})
	require.NoError(t, err)
	assert.Equal(t, exp, src.gotList.Expectations)
	require.Len(t, res.Scenarios[0].Spec.ConversationAssertions, 1)
	assert.Equal(t, 0.8, res.Scenarios[0].Spec.ConversationAssertions[0].Params["min_score"])
}

// A pack supplies params and the declared threshold for evals the source
// recorded without them (Omnia does not store eval params).
func TestGenerate_PackEnrichesEvals(t *testing.T) {
	src := &fakeSource{
		summaries: []SessionSummary{{ID: "a"}},
		details:   map[string]*SessionDetail{"a": session("a", "hi", EvalResult{ID: "faith", Type: "faithfulness", Kind: "eval", Score: f(0.3)})},
	}
	v := 0.8
	pack := &packspec.Pack{Evals: []*packspec.Eval{{
		ID: "faith", Type: "faithfulness", Params: map[string]any{"judge": "default"},
		Threshold: &packspec.EvalThreshold{Operator: "gte", Value: &v},
	}}}
	res, err := Generate(context.Background(), Request{Source: src, Pack: pack})
	require.NoError(t, err)
	require.Len(t, res.Scenarios[0].Spec.ConversationAssertions, 1)
	ac := res.Scenarios[0].Spec.ConversationAssertions[0]
	assert.Equal(t, "default", ac.Params["judge"])
	assert.Equal(t, 0.8, ac.Params["min_score"])
	assert.Equal(t, DecisionAssertedThreshold, res.Decisions[0].Outcome)
}

func TestWriteScenarios(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "out")
	c, err := Convert(session("write-me", "hi"), ConvertOptions{})
	require.NoError(t, err)

	paths, err := WriteScenarios(dir, []*arenaconfig.ScenarioConfig{c.Scenario})
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Equal(t, filepath.Join(dir, "write-me.scenario.yaml"), paths[0])
	data, err := os.ReadFile(paths[0])
	require.NoError(t, err)
	assert.Contains(t, string(data), "kind: Scenario")
	assert.Contains(t, string(data), "content: hi")
}
