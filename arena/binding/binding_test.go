package binding

import (
	"context"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/inference"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
	"github.com/stretchr/testify/require"
)

// stubInference stands in for a real inference backend. Only its identity in
// the registry matters here.
type stubInference struct{ name string }

func (s *stubInference) Infer(context.Context, inference.Request) (inference.Response, error) {
	return inference.Response{Raw: s.name}, nil
}

func registries(t *testing.T) (*providers.Registry, *inference.Registry) {
	t.Helper()
	pr := providers.NewRegistry()
	pr.Register(mock.NewProvider("agent", "mock-model", false))
	ir := inference.NewRegistry()
	require.NoError(t, ir.Register("topic", &stubInference{name: "topic"}))
	return pr, ir
}

func TestNewReturnsNilWhenNothingIsWired(t *testing.T) {
	require.Nil(t, New(nil, nil, nil),
		"a binding over nothing must be nil so the runtime reports ErrNoBinding, "+
			"which is a different fix from an unbound key")
}

func TestLLMResolvesAProviderID(t *testing.T) {
	pr, ir := registries(t)
	b := New(pr, ir, nil)

	p, err := b.LLM("agent")
	require.NoError(t, err)
	require.Equal(t, "agent", p.ID())
}

func TestLLMResolvesAJudgeAlias(t *testing.T) {
	pr, ir := registries(t)
	b := New(pr, ir, map[string]string{"grader": "agent"})

	p, err := b.LLM("grader")
	require.NoError(t, err)
	require.Equal(t, "agent", p.ID(),
		"a judges: entry is the logical name a pack may point at")
}

// An inference provider bound to the name a judge-backed check uses is a
// wiring mistake, and reporting it as "not found" would send the author
// hunting for a missing provider instead of looking at the one they supplied.
func TestLLMReportsAnInferenceProviderAsTheWrongKind(t *testing.T) {
	pr, ir := registries(t)
	b := New(pr, ir, nil)

	_, err := b.LLM("topic")
	require.ErrorIs(t, err, evals.ErrWrongKind)
	require.NotErrorIs(t, err, evals.ErrUnboundKey)
}

func TestLLMReportsAnUnknownNameAsUnbound(t *testing.T) {
	pr, ir := registries(t)
	b := New(pr, ir, nil)

	_, err := b.LLM("nobody")
	require.ErrorIs(t, err, evals.ErrUnboundKey)
}

// Classifier-backed checks (topic_policy, audio_emotion, image_moderation, the
// text classifiers) assert inference.Provider on what the binding returns. A
// binding that handed back anything else made every one of them report "not
// an inference provider" and fall to its on_error policy — for topic_policy,
// denying every turn.
func TestClassifierReturnsTheNamedInferenceProvider(t *testing.T) {
	wanted := &stubInference{name: "wanted"}
	ir := inference.NewRegistry()
	require.NoError(t, ir.Register("first", &stubInference{name: "first"}))
	require.NoError(t, ir.Register("wanted", wanted))
	b := New(providers.NewRegistry(), ir, nil)

	got, err := b.Classifier("wanted")
	require.NoError(t, err)
	p, ok := got.(inference.Provider)
	require.True(t, ok, "classifier-backed checks assert inference.Provider; got %T", got)
	require.Same(t, wanted, p, "the named provider, not the registry default")
}

func TestClassifierReportsAnLLMAsTheWrongKind(t *testing.T) {
	pr, ir := registries(t)
	b := New(pr, ir, nil)

	_, err := b.Classifier("agent")
	require.ErrorIs(t, err, evals.ErrWrongKind)
}

// An empty key means "the default" to inference.Registry. Letting it through
// the binding would make a check that named no provider resolve through the
// binding to whatever happens to be the default, which is the convention-based
// resolution the requires block exists to stop. The default is reached through
// the registry on the context instead.
func TestAnEmptyKeyNeverResolves(t *testing.T) {
	pr, ir := registries(t)
	b := New(pr, ir, nil)

	_, err := b.Classifier("")
	require.ErrorIs(t, err, evals.ErrUnboundKey)
}
