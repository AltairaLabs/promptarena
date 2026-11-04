package calibrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"

	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/results/objectives"
)

func writeExamples(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "labels.jsonl")
	require.NoError(t, os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o600))
	return p
}

func TestLoadExamples(t *testing.T) {
	exs, err := LoadExamples(writeExamples(t,
		`{"id": "a", "content": "I can't help with that.", "label": true}`,
		``,
		`{"messages": [{"role": "user", "content": "hi"}, {"role": "assistant", "content": "hello"}], "label": false}`))
	require.NoError(t, err)
	require.Len(t, exs, 2)
	assert.Equal(t, "a", exs[0].ID)
	assert.Equal(t, "line 3", exs[1].ID, "an example without an id is named by its line")

	for name, line := range map[string]string{
		"label is required":                 `{"content": "x"}`,
		"needs content":                     `{"label": true}`,
		"invalid character":                 `not json`,
		"not both":                          `{"content": "x", "messages": [{"role": "assistant", "content": "y"}], "label": true}`,
		"must end with the assistant reply": `{"messages": [{"role": "user", "content": "refund?"}], "label": true}`,
		"duplicate id \"a\"":                `{"id": "a", "content": "x", "label": true}` + "\n" + `{"id": "a", "content": "y", "label": true}`,
	} {
		_, err := LoadExamples(writeExamples(t, line))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), name)
	}
	_, err = LoadExamples(writeExamples(t, ""))
	assert.ErrorContains(t, err, "has no examples")
}

func TestConversation(t *testing.T) {
	ex := Example{Input: "refund?", Content: "no"}
	assert.Equal(t, []types.Message{{Role: "user", Content: "refund?"}, {Role: "assistant", Content: "no"}},
		ex.conversation())
	assert.Equal(t, []types.Message{{Role: "assistant", Content: "no"}}, (&Example{Content: "no"}).conversation())
}

func label(v bool) *bool { return &v }

// fakeCheck passes a conversation whose last message contains "pass", and
// errors on one containing "boom".
func fakeCheck(_ context.Context, check asrt.AssertionConfig, messages []types.Message, _ string,
) asrt.ConversationValidationResult {
	last := messages[len(messages)-1].Content
	switch {
	case strings.Contains(last, "boom"):
		return asrt.ConversationValidationResult{Type: check.Type, Details: map[string]interface{}{"error": "judge down"}}
	case strings.Contains(last, "pass"):
		return asrt.ConversationValidationResult{Type: check.Type, Passed: true, Message: "configured",
			Details: map[string]interface{}{"score": 1.0, "explanation": "ok"}}
	}
	return asrt.ConversationValidationResult{Type: check.Type, Message: "no",
		Details: map[string]interface{}{"score": 0.0}}
}

func TestRun(t *testing.T) {
	exs := []Example{
		{ID: "tp1", Content: "pass", Label: label(true)},
		{ID: "tp2", Content: "pass", Label: label(true)},
		{ID: "ff", Content: "nope", Label: label(true), Note: "a refusal the check missed"},
		{ID: "tf1", Content: "nope", Label: label(false)},
		{ID: "tf2", Content: "nope", Label: label(false)},
		{ID: "fp", Content: "pass", Label: label(false)},
		{ID: "err", Content: "boom", Label: label(true)},
	}
	r := Run(context.Background(), asrt.AssertionConfig{Type: "refusal"}, exs, fakeCheck)

	assert.Equal(t, 7, r.Examples)
	assert.Equal(t, 6, r.Evaluated)
	assert.Equal(t, 1, r.Errored)
	assert.Equal(t, [4]int{2, 1, 2, 1}, [4]int{r.TruePass, r.FalsePass, r.TrueFail, r.FalseFail})
	assert.InDelta(t, 4.0/6.0, r.Agreement, 1e-9)
	low, high := objectives.Wilson(4, 6)
	assert.InDelta(t, low, r.AgreementLow, 1e-9)
	assert.InDelta(t, high, r.AgreementHigh, 1e-9)
	assert.InDelta(t, 2.0/3.0, r.PassRecall, 1e-9)
	assert.InDelta(t, 2.0/3.0, r.FailRecall, 1e-9)
	assert.InDelta(t, 2.0/3.0, r.PassPrecision, 1e-9)
	// Observed 4/6; chance 0.5*0.5 + 0.5*0.5 = 0.5; kappa (0.667-0.5)/0.5.
	assert.InDelta(t, 1.0/3.0, r.Kappa, 1e-9)
	assert.Equal(t, 1.0, *r.Outcomes[0].Score)
	assert.Equal(t, "ok", r.Outcomes[0].Message, "the handler's explanation, not the configured message")
	assert.Equal(t, "judge down", r.Outcomes[6].Error)

	md := RenderMarkdown(r)
	assert.Contains(t, md, "# Check Calibration: refusal")
	assert.Contains(t, md, "| Agreement | 4 / 6 (66.7%) |")
	assert.Contains(t, md, "| Cohen's kappa | 0.33 | – |")
	assert.Contains(t, md, "| Check passed | 2 | 1 |\n| Check failed | 1 | 2 |")
	assert.Contains(t, md, "| ff | pass | fail | no | a refusal the check missed |")
	assert.Contains(t, md, "| err | pass | not evaluated: judge down |")
	assert.NotContains(t, md, "| tp1 |", "agreements aren't listed")
}

func TestKappaEdges(t *testing.T) {
	assert.Equal(t, 0.0, kappa(0, 0, 0, 0))
	assert.Equal(t, 1.0, kappa(5, 0, 0, 0), "all pass, all labeled pass")
	assert.InDelta(t, 1.0, kappa(3, 0, 3, 0), 1e-9)
	assert.InDelta(t, -1.0, kappa(0, 3, 0, 3), 1e-9)
}

func TestRenderMarkdown_AllAgree(t *testing.T) {
	r := Run(context.Background(), asrt.AssertionConfig{Type: "x"},
		[]Example{{ID: "a", Content: "pass", Label: label(true)}}, fakeCheck)
	md := RenderMarkdown(r)
	assert.Contains(t, md, "The check agreed with every evaluated example.")
	assert.Contains(t, md, "| Fails fail-labeled examples | – | – |")
}

func TestRun_CanceledExamplesAreNotEvaluated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	run := func(c context.Context, check asrt.AssertionConfig, m []types.Message, id string) asrt.ConversationValidationResult {
		calls++
		cancel()
		return fakeCheck(c, check, m, id)
	}
	r := Run(ctx, asrt.AssertionConfig{Type: "x"}, []Example{
		{ID: "a", Content: "pass", Label: label(true)}, {ID: "b", Content: "pass", Label: label(true)},
	}, run)
	assert.Equal(t, 1, calls, "no example is scored after the cancel")
	assert.Equal(t, 1, r.Evaluated)
	assert.Equal(t, "not run: context canceled", r.Outcomes[1].Error)
}

func TestTally_NoExamplesOfALabel(t *testing.T) {
	r := Run(context.Background(), asrt.AssertionConfig{Type: "x"},
		[]Example{{ID: "a", Content: "pass", Label: label(true)}}, fakeCheck)
	assert.Equal(t, [3]float64{0, 0, 0}, [3]float64{r.FailRecall, r.FailRecallLow, r.FailRecallHigh})
}
