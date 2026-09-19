package binding

import (
	"errors"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/classify"
	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
	"github.com/stretchr/testify/require"
)

// stubTextClassifier stands in for a real inference backend. Only its
// presence in the registry matters here.
type stubTextClassifier struct{ classify.TextClassifier }

func registries(t *testing.T) (*providers.Registry, *classify.Registry) {
	t.Helper()
	pr := providers.NewRegistry()
	pr.Register(mock.NewProvider("agent", "mock-model", false))
	cr := classify.NewRegistry()
	cr.RegisterText("hf", &stubTextClassifier{})
	return pr, cr
}

func TestNewReturnsNilWhenNothingIsWired(t *testing.T) {
	require.Nil(t, New(nil, nil, nil),
		"a binding over nothing must be nil so the runtime reports ErrNoBinding, "+
			"which is a different fix from an unbound key")
}

func TestLLMResolvesAProviderID(t *testing.T) {
	pr, cr := registries(t)
	b := New(pr, cr, nil)

	p, err := b.LLM("agent")
	require.NoError(t, err)
	require.Equal(t, "agent", p.ID())
}

func TestLLMResolvesAJudgeAlias(t *testing.T) {
	pr, cr := registries(t)
	b := New(pr, cr, map[string]string{"grader": "agent"})

	p, err := b.LLM("grader")
	require.NoError(t, err)
	require.Equal(t, "agent", p.ID(),
		"a judges: entry is the logical name a pack may point at")
}

// A classify provider bound to the name a judge-backed check uses is a wiring
// mistake, and reporting it as "not found" would send the author hunting for a
// missing provider instead of looking at the one they supplied.
func TestLLMReportsAClassifierAsTheWrongKind(t *testing.T) {
	pr, cr := registries(t)
	b := New(pr, cr, nil)

	_, err := b.LLM("hf")
	require.ErrorIs(t, err, evals.ErrWrongKind)
	require.NotErrorIs(t, err, evals.ErrUnboundKey)
}

func TestLLMReportsAnUnknownNameAsUnbound(t *testing.T) {
	pr, cr := registries(t)
	b := New(pr, cr, nil)

	_, err := b.LLM("nobody")
	require.ErrorIs(t, err, evals.ErrUnboundKey)
}

func TestClassifierResolvesAnInferenceProvider(t *testing.T) {
	pr, cr := registries(t)
	b := New(pr, cr, nil)

	backend, err := b.Classifier("hf")
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestClassifierReportsAnLLMAsTheWrongKind(t *testing.T) {
	pr, cr := registries(t)
	b := New(pr, cr, nil)

	_, err := b.Classifier("agent")
	require.ErrorIs(t, err, evals.ErrWrongKind)
}

// An empty key means "the default" to classify.Registry. Letting it through
// would make a check that named no provider silently resolve to whatever
// happens to be the default classifier, which is the convention-based
// resolution the requires block exists to stop.
func TestAnEmptyKeyNeverResolves(t *testing.T) {
	pr, cr := registries(t)
	require.NoError(t, cr.SetDefaultText("hf"))
	b := New(pr, cr, nil)

	_, err := b.Classifier("")
	require.True(t, errors.Is(err, evals.ErrUnboundKey), "got %v", err)
}
