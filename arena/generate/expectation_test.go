package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func f(v float64) *float64 { return &v }

func TestParseExpectation(t *testing.T) {
	tests := []struct {
		in      string
		wantID  string
		wantMin *float64
		wantMax *float64
	}{
		{"faithfulness>=0.8", "faithfulness", f(0.8), nil},
		{"faithfulness >= 0.8", "faithfulness", f(0.8), nil},
		{"toxicity<=0.2", "toxicity", nil, f(0.2)},
		{"latency_budget<1500", "latency_budget", nil, f(1500)},
		{"cost>0.01", "cost", f(0.01), nil},
		{"exact==1", "exact", f(1), f(1)},
		{"exact=1", "exact", f(1), f(1)},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseExpectation(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.EvalID)
			assert.Equal(t, tt.wantMin, got.Min)
			assert.Equal(t, tt.wantMax, got.Max)
		})
	}
}

func TestParseExpectation_Errors(t *testing.T) {
	for _, in := range []string{"", "faithfulness", ">=0.8", "faithfulness>=", "faithfulness>=high", "a>=1>=2"} {
		_, err := ParseExpectation(in)
		assert.Error(t, err, in)
	}
}

func TestExpectation_Violated(t *testing.T) {
	e := Expectation{EvalID: "x", Min: f(0.8)}
	assert.True(t, e.Violated(f(0.79)))
	assert.False(t, e.Violated(f(0.8)), "bounds are inclusive, like min_score")
	assert.True(t, e.Violated(nil), "a measurement that never produced a score cannot satisfy an expectation")

	e = Expectation{EvalID: "x", Max: f(0.2)}
	assert.True(t, e.Violated(f(0.21)))
	assert.False(t, e.Violated(f(0.2)))
}

func TestBoundsFromThreshold(t *testing.T) {
	tests := []struct {
		op       string
		wantMin  *float64
		wantMax  *float64
		wantNote bool
	}{
		{"gte", f(0.8), nil, false},
		{">=", f(0.8), nil, false},
		{"gt", f(0.8), nil, true}, // min_score is inclusive; strictness is lost and said so
		{"lte", nil, f(0.8), false},
		{"lt", nil, f(0.8), true},
		{"eq", f(0.8), f(0.8), false},
		{"==", f(0.8), f(0.8), false},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			minS, maxS, note := BoundsFromThreshold(&EvalThreshold{Operator: tt.op, Value: 0.8})
			assert.Equal(t, tt.wantMin, minS)
			assert.Equal(t, tt.wantMax, maxS)
			assert.Equal(t, tt.wantNote, note != "")
		})
	}
	minS, maxS, note := BoundsFromThreshold(&EvalThreshold{Operator: "between", Value: 0.8})
	assert.Nil(t, minS)
	assert.Nil(t, maxS)
	assert.Contains(t, note, "between")
	minS, maxS, _ = BoundsFromThreshold(nil)
	assert.Nil(t, minS)
	assert.Nil(t, maxS)
}

func TestEvalResult_Failed(t *testing.T) {
	no := false
	yes := true
	assert.True(t, EvalResult{Passed: &no}.Failed())
	assert.False(t, EvalResult{Passed: &yes}.Failed())
	assert.False(t, EvalResult{Score: f(0.1)}.Failed(), "a bare measurement has no verdict to fail")
}
