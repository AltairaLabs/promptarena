package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func judgeConfig() *Config {
	return &Config{LoadedJudges: map[string]*JudgeTarget{"aaa-low": {}, "picked": {}}}
}

// `judge: <name>` was read by nothing: the handlers graded with the
// lowest-sorted judge whatever the check named.
func TestResolveJudgeParams_NamedJudgeBecomesProvider(t *testing.T) {
	cfg := judgeConfig()
	inner := map[string]interface{}{"judge": "picked", "criteria": "c"}
	direct := map[string]interface{}{"judge": "picked"}
	explicit := map[string]interface{}{"judge": "picked", "provider": "other"}
	cfg.LoadedScenarios = map[string]*Scenario{"s": {
		Turns: []TurnDefinition{{Assertions: []AssertionConfig{
			{Type: "assertion", Params: map[string]interface{}{"eval_type": "llm_judge", "eval_params": inner}},
			{Type: "llm_judge", Params: direct},
		}}},
		ConversationAssertions: []AssertionConfig{{Type: "llm_judge_conversation", Params: explicit}},
	}}
	cfg.Globals = &Globals{ConversationAssertions: []AssertionConfig{{Type: "contains"}}}
	cfg.LoadedEvals = map[string]*Eval{"e": {Turns: []EvalTurnConfig{{AllTurns: &EvalAllTurnsConfig{
		Assertions: []AssertionConfig{{Type: "llm_judge", Params: map[string]interface{}{"judge": "aaa-low"}}},
	}}}}}

	require.NoError(t, cfg.resolveJudgeParams())
	assert.Equal(t, "picked", inner["provider"])
	assert.Equal(t, "picked", direct["provider"])
	assert.Equal(t, "other", explicit["provider"], "an explicit provider is left alone")
	assert.Equal(t, "aaa-low", cfg.LoadedEvals["e"].Turns[0].AllTurns.Assertions[0].Params["provider"])
}

func TestResolveJudgeParams_UnknownJudgeIsAnError(t *testing.T) {
	cfg := judgeConfig()
	cfg.LoadedScenarios = map[string]*Scenario{"s": {
		ConversationAssertions: []AssertionConfig{{Type: "assertion", Params: map[string]interface{}{
			"eval_type": "llm_judge", "eval_params": map[string]interface{}{"judge": "nope"},
		}}},
	}}
	err := cfg.resolveJudgeParams()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `judge "nope" is not defined in spec.judges`)
	assert.Contains(t, err.Error(), "scenario s conversation_assertions")

	cfg.LoadedScenarios["s"].ConversationAssertions[0].Params = map[string]interface{}{"judge": "nope"}
	require.Error(t, cfg.resolveJudgeParams())
}

func TestResolveJudgeParams_NoJudgeParamIsUntouched(t *testing.T) {
	params := map[string]interface{}{"criteria": "c"}
	cfg := judgeConfig()
	cfg.LoadedScenarios = map[string]*Scenario{"s": nil, "t": {
		ConversationAssertions: []AssertionConfig{{Type: "llm_judge", Params: params}},
	}}
	cfg.LoadedEvals = map[string]*Eval{"e": nil}
	require.NoError(t, cfg.resolveJudgeParams())
	assert.NotContains(t, params, "provider")
}
