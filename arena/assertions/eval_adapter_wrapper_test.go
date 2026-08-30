package assertions

import (
	"context"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
)

// Arena's converter is coupled to how promptkit's assertion wrapper shapes a
// result, and every test around it hand-built that shape:
// evals.EvalResult{Value: true}. So when promptkit stopped putting the boolean
// in Value, the whole suite stayed green while every assertion in the product
// reported the wrong answer — a fixture agreeing with a fixture.
//
// The test below runs the REAL wrapper, through the real runner, into the real
// converter. It is the only thing here that would have noticed.

type wrapperJudge struct{}

func (wrapperJudge) Type() string { return "llm_judge" }
func (wrapperJudge) Eval(context.Context, *evals.EvalContext, map[string]any) (*evals.EvalResult, error) {
	score := 0.9
	return &evals.EvalResult{
		Score:       &score,
		Value:       map[string]any{"accuracy": 0.9},
		Explanation: "close enough",
	}, nil
}

// TestConvertEvalResult_RealAssertionWrapper drives the actual promptkit
// assertion wrapper and feeds its result to arena's converter, which is what
// every report reads.
//
// min_score 0.8 over a judge scoring 0.9: promptkit's wrapper says passed. If
// arena re-derives instead of reading what the result states, it applies
// `score >= 1.0` — the assertion's DEFAULT threshold — and reports FAIL.
func TestConvertEvalResult_RealAssertionWrapper(t *testing.T) {
	reg := evals.NewEvalTypeRegistry()
	reg.Register(wrapperJudge{})

	runner := evals.NewEvalRunner(reg)
	results := runner.RunTurnEvals(context.Background(), []evals.EvalDef{{
		ID:      "quality",
		Type:    "assertion",
		Trigger: evals.TriggerEveryTurn,
		Params: map[string]any{
			"eval_type": "llm_judge",
			"min_score": 0.8,
		},
	}}, &evals.EvalContext{})

	if len(results) != 1 {
		t.Fatalf("got %d results", len(results))
	}
	r := results[0]
	if r.Passed == nil {
		t.Fatal("the assertion wrapper stated no pass/fail")
	}
	if !*r.Passed {
		t.Fatalf("promptkit says the assertion failed at score %v; the fixture is wrong", *r.Score)
	}

	got := convertOneEvalResult(&r)
	if !got.Passed {
		t.Fatalf("score 0.9 clears min_score 0.8 and promptkit stated a pass, but arena " +
			"reported FAIL. Arena is re-deriving from score instead of reading Passed")
	}

	// The inner eval's own output must survive into the report. The wrapper
	// used to overwrite it with its boolean, so a judge's structured reasoning
	// never reached a reader.
	value, ok := got.Details["value"].(map[string]any)
	if !ok {
		t.Fatalf("the inner eval's value did not reach the report: %#v", got.Details["value"])
	}
	if value["accuracy"] != 0.9 {
		t.Errorf("value = %#v, want accuracy 0.9", value)
	}
}
