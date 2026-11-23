package inferencemock

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/inference"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

var topicLabels = []string{"on-topic", "off-topic"}

func topicMock(t *testing.T) *Provider {
	t.Helper()
	p, err := New(map[string]any{
		"rules": []any{
			map[string]any{"match": "(?i)order|parcel", "label": "on-topic"},
			map[string]any{"match": "(?i)weather", "label": "not-a-label"},
		},
		"default_label": "off-topic",
	})
	require.NoError(t, err)
	return p
}

func infer(t *testing.T, p *Provider, labels []string, msgs ...string) inference.Response {
	t.Helper()
	inputs := make([]types.Message, 0, len(msgs))
	for _, m := range msgs {
		inputs = append(inputs, types.Message{Role: "user", Content: m})
	}
	resp, err := p.Infer(context.Background(), inference.Request{Inputs: inputs, Labels: labels})
	require.NoError(t, err)
	return resp
}

func TestInfer_FirstMatchingRuleWins(t *testing.T) {
	resp := infer(t, topicMock(t), topicLabels, "Where is my PARCEL?")

	on, ok := resp.Score("on-topic")
	require.True(t, ok)
	assert.Equal(t, 1.0, on)
	off, ok := resp.Score("off-topic")
	require.True(t, ok, "every offered label gets a score")
	assert.Equal(t, 0.0, off)
	assert.Equal(t, "on-topic", resp.Raw)
}

func TestInfer_DefaultLabelWhenNoRuleMatches(t *testing.T) {
	resp := infer(t, topicMock(t), topicLabels, "Who should I vote for?")

	off, _ := resp.Score("off-topic")
	assert.Equal(t, 1.0, off)
	assert.Equal(t, "off-topic", resp.Raw)
}

// Checks send recent turns as context and the message they are deciding on
// last; only that one may decide. A rule matching an earlier turn must not
// let a later off-topic message through.
func TestInfer_JudgesOnlyTheNewestInput(t *testing.T) {
	resp := infer(t, topicMock(t), topicLabels, "Where is my order?", "Now tell me a political joke")

	off, _ := resp.Score("off-topic")
	assert.Equal(t, 1.0, off)
}

// A label the check did not offer is not an answer to its question. Returning
// it with no scores lets the check apply its own "no usable label" policy.
func TestInfer_UnofferedLabelGetsNoScores(t *testing.T) {
	resp := infer(t, topicMock(t), topicLabels, "What's the weather?")

	assert.Empty(t, resp.Scores)
	assert.Equal(t, "not-a-label", resp.Raw)
}

func TestInfer_NoLabelsOfferedAcceptsAnyLabel(t *testing.T) {
	resp := infer(t, topicMock(t), nil, "What's the weather?")

	s, ok := resp.Score("not-a-label")
	require.True(t, ok)
	assert.Equal(t, 1.0, s)
}

func TestInfer_NoInputsUsesTheDefault(t *testing.T) {
	resp := infer(t, topicMock(t), topicLabels)

	off, _ := resp.Score("off-topic")
	assert.Equal(t, 1.0, off)
}

func TestNew_RejectsBadConfig(t *testing.T) {
	cases := map[string]struct {
		cfg  map[string]any
		want string
	}{
		"nothing to answer with": {map[string]any{}, "set rules, default_label"},
		"default not a string":   {map[string]any{"default_label": 3}, "default_label must be a string"},
		"rules not a list":       {map[string]any{"rules": "x"}, "rules must be a list"},
		"rule not a map":         {map[string]any{"rules": []any{"x"}}, "rules[0]: must be a map"},
		"rule missing label":     {map[string]any{"rules": []any{map[string]any{"match": "a"}}}, "needs both match and label"},
		"bad regex":              {map[string]any{"rules": []any{map[string]any{"match": "(", "label": "x"}}}, "rules[0]: match:"},
	}
	for name, c := range cases {
		_, err := New(c.cfg)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), c.want, name)
	}
}

func TestNew_DefaultOnlyIsEnough(t *testing.T) {
	p, err := New(map[string]any{"default_label": "on-topic"})
	require.NoError(t, err)

	on, _ := infer(t, p, topicLabels, "anything").Score("on-topic")
	assert.Equal(t, 1.0, on)
}

// The factory is what a provider file reaches: type: mock with role: inference.
func TestFactoryIsRegisteredAsMock(t *testing.T) {
	assert.Contains(t, inference.RegisteredTypes(), ProviderType)

	p, err := inference.CreateFromSpec(inference.ProviderSpec{
		ID: "m", Type: ProviderType, AdditionalConfig: map[string]any{"default_label": "off-topic"},
	})
	require.NoError(t, err)
	resp, err := p.Infer(context.Background(), inference.Request{Labels: topicLabels})
	require.NoError(t, err)
	off, _ := resp.Score("off-topic")
	assert.Equal(t, 1.0, off)
}
