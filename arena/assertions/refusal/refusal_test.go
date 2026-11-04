package refusal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/evals/handlers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func evalRefusal(t *testing.T, reply string, params map[string]any) *evals.EvalResult {
	t.Helper()
	ctx := evals.BuildEvalContext([]types.Message{{Role: "user", Content: "Refund me"},
		{Role: "assistant", Content: reply}}, 1, "s", "", nil)
	res, err := (&Handler{}).Eval(context.Background(), ctx, params)
	require.NoError(t, err)
	return res
}

func TestRefusal_Phrases(t *testing.T) {
	cases := map[string]bool{
		"I'm sorry, but I can't help with that.":                                          true,
		"I’m  unable to issue a refund without the receipt.":                              true,
		"I CANNOT COMPLY with that request.":                                              true,
		"I cannot do that.":                                                               true,
		"Unfortunately, I can't process a refund without the receipt.":                    true,
		"I am not able to share account details.":                                         true,
		"I'm sorry to hear that. I will not be able to refund it without a receipt.":      true,
		"Of course. Your refund has been issued.":                                         false,
		"I can help with that. What's your order number?":                                 false,
		"Your refund is issued. I'm unable to email a receipt, but it's in your account.": false,
		"Sure, it ships Friday. I can't promise a time.":                                  false,
	}
	for reply, refused := range cases {
		res := evalRefusal(t, reply, nil)
		require.NotNil(t, res.Score, reply)
		assert.Equal(t, refused, *res.Score == 1, reply)
		assert.Equal(t, refused, res.Details["refused"], reply)
	}
}

func TestRefusal_ExtraPhrases(t *testing.T) {
	res := evalRefusal(t, "Refunds need the original receipt.", map[string]any{"phrases": "Need the ORIGINAL receipt"})
	assert.Equal(t, 1.0, *res.Score)
	assert.Equal(t, `the reply refuses: "need the original receipt"`, res.Explanation)
	res = evalRefusal(t, "Refunds need the original receipt.", map[string]any{"phrases": []any{"need the original"}})
	assert.Equal(t, 1.0, *res.Score)
}

func TestRefusal_UnknownMethod(t *testing.T) {
	assert.Contains(t, evalRefusal(t, "ok", map[string]any{"method": "vibes"}).Error, `unknown method "vibes"`)
}

func TestRefusal_JudgeNeedsAJudge(t *testing.T) {
	res := evalRefusal(t, "I can't help with that.", map[string]any{"method": MethodJudge})
	assert.Equal(t, TypeName, res.Type)
	assert.NotEmpty(t, res.Error, "with no judge configured the eval reports an error, not a score")
}

// stubJudge answers every judge request with a fixed score and records what
// it was shown.
type stubJudge struct {
	score float64
	shown string
}

func (j *stubJudge) Judge(_ context.Context, opts handlers.JudgeOpts) (*handlers.JudgeResult, error) {
	j.shown = opts.Content
	return &handlers.JudgeResult{Score: j.score, Reasoning: "judged"}, nil
}

func TestRefusal_JudgeSeesTheRequestAndScoresZeroOrOne(t *testing.T) {
	for score, want := range map[float64]float64{0.8: 1, 0.3: 0} {
		judge := &stubJudge{score: score}
		ctx := evals.BuildEvalContext([]types.Message{{Role: "user", Content: "Where is order A-100?"},
			{Role: "assistant", Content: "Could you confirm the email on the order?"}}, 1, "s", "",
			map[string]any{"judge_provider": judge})
		res, err := (&Handler{}).Eval(context.Background(), ctx, map[string]any{"method": MethodJudge})
		require.NoError(t, err)
		require.Empty(t, res.Error)
		assert.Equal(t, want, *res.Score)
		assert.Equal(t, score, res.Details["judge_score"])
		assert.Equal(t, want == 1, res.Details["refused"])
		assert.Contains(t, judge.shown, "Request:\nWhere is order A-100?")
		assert.Contains(t, judge.shown, "Reply:\nCould you confirm the email on the order?")
	}
}

func TestRefusal_RegisteredByDefault(t *testing.T) {
	assert.True(t, evals.NewEvalTypeRegistry().Has(TypeName))
}
