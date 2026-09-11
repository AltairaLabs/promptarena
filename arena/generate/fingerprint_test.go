package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AltairaLabs/PromptKit/runtime/types"
)

func failed(id, typ string, turn *int, details map[string]any) EvalResult {
	no := false
	return EvalResult{ID: id, Type: typ, Kind: "assertion", Passed: &no, Turn: turn, Details: details}
}

func TestFingerprint_Stability(t *testing.T) {
	s := &SessionDetail{
		Messages: []types.Message{{Role: "user", Content: "hello"}},
		Evals:    []EvalResult{failed("a", "content_matches", nil, map[string]any{"pattern": "x"})},
	}
	assert.Equal(t, Fingerprint(s), Fingerprint(s))
	assert.Len(t, Fingerprint(s), fingerprintLength)
}

func TestFingerprint_DifferentFailuresDiffer(t *testing.T) {
	a := &SessionDetail{Messages: []types.Message{{Role: "user", Content: "hello"}},
		Evals: []EvalResult{failed("a", "content_matches", nil, nil)}}
	b := &SessionDetail{Messages: []types.Message{{Role: "user", Content: "hello"}},
		Evals: []EvalResult{failed("a", "tools_called", nil, nil)}}
	assert.NotEqual(t, Fingerprint(a), Fingerprint(b))
}

func TestFingerprint_TurnMatters(t *testing.T) {
	a := &SessionDetail{Messages: []types.Message{{Role: "user", Content: "hello"}},
		Evals: []EvalResult{failed("a", "content_matches", intp(0), nil)}}
	b := &SessionDetail{Messages: []types.Message{{Role: "user", Content: "hello"}},
		Evals: []EvalResult{failed("a", "content_matches", intp(1), nil)}}
	assert.NotEqual(t, Fingerprint(a), Fingerprint(b))
}

// A passed verdict and a bare measurement say nothing about how the session
// failed, so neither changes the fingerprint.
func TestFingerprint_IgnoresPassedAndMeasurements(t *testing.T) {
	yes := true
	base := &SessionDetail{Messages: []types.Message{{Role: "user", Content: "hello"}}}
	with := &SessionDetail{Messages: []types.Message{{Role: "user", Content: "hello"}},
		Evals: []EvalResult{
			{ID: "p", Type: "x", Kind: "assertion", Passed: &yes},
			{ID: "m", Type: "faithfulness", Kind: "eval", Score: f(0.4)},
		}}
	assert.Equal(t, Fingerprint(base), Fingerprint(with))
}

func TestFingerprint_ContentMatters(t *testing.T) {
	a := &SessionDetail{Messages: []types.Message{{Role: "user", Content: "hello"}}}
	b := &SessionDetail{Messages: []types.Message{{Role: "user", Content: "goodbye"}}}
	assert.NotEqual(t, Fingerprint(a), Fingerprint(b))
}

func TestDeduplicateSessions(t *testing.T) {
	dup := func(id string) *SessionDetail {
		return &SessionDetail{SessionSummary: SessionSummary{ID: id},
			Messages: []types.Message{{Role: "user", Content: "same"}},
			Evals:    []EvalResult{failed("a", "content_matches", nil, nil)}}
	}
	other := &SessionDetail{SessionSummary: SessionSummary{ID: "c"}, Messages: []types.Message{{Role: "user", Content: "different"}}}
	out := DeduplicateSessions([]*SessionDetail{dup("a"), dup("b"), other})
	assert.Len(t, out, 2)
	assert.Equal(t, "a", out[0].ID, "first occurrence is kept")
	assert.Equal(t, "c", out[1].ID)
	assert.Empty(t, DeduplicateSessions(nil))
}
