package arenaconfig

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
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
	for _, p := range []map[string]interface{}{inner, direct, explicit} {
		assert.NotContains(t, p, "judge", "judge is dropped once resolved: no handler reads it")
	}
}

// Objectives and persona transitions take the same `judge` selector.
func TestResolveJudgeParams_ObjectivesAndPersonaTransitions(t *testing.T) {
	cfg := judgeConfig()
	success := map[string]interface{}{"eval_type": "llm_judge", "eval_params": map[string]interface{}{"judge": "picked"}}
	rewind := map[string]interface{}{"judge": "picked"}
	transition := map[string]interface{}{"judge": "picked"}
	cfg.LoadedScenarios = map[string]*Scenario{"s": {Turns: []TurnDefinition{{Objective: &ObjectiveConfig{
		Success: []AssertionConfig{{Type: "assertion", Params: success}},
		Rewind:  &ObjectiveRewindConfig{When: []AssertionConfig{{Type: "llm_judge", Params: rewind}}},
	}}}}}
	cfg.LoadedPersonas = map[string]*UserPersonaPack{
		"p":   {Transitions: []PersonaTransition{{When: []AssertionConfig{{Type: "llm_judge", Params: transition}}}}},
		"nil": nil,
	}

	require.NoError(t, cfg.resolveJudgeParams())
	inner := success["eval_params"].(map[string]interface{})
	for _, p := range []map[string]interface{}{inner, rewind, transition} {
		assert.Equal(t, "picked", p["provider"])
		assert.NotContains(t, p, "judge")
	}
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

// pack_evals take the same `judge` selector as scenario assertions.
func TestResolveJudgeParams_PackEvals(t *testing.T) {
	cfg := judgeConfig()
	direct := map[string]interface{}{"judge": "picked"}
	inner := map[string]interface{}{"judge": "picked"}
	cfg.PackEvals = []evals.EvalDef{
		{ID: "direct", Type: "llm_judge", Params: direct},
		{ID: "wrapped", Type: "assertion", Params: map[string]interface{}{"eval_params": inner}},
		{ID: "plain", Type: "contains"},
	}
	require.NoError(t, cfg.resolveJudgeParams())
	assert.Equal(t, "picked", direct["provider"])
	assert.Equal(t, "picked", inner["provider"])

	cfg.PackEvals = []evals.EvalDef{{ID: "bad", Type: "assertion", Params: map[string]interface{}{
		"eval_params": map[string]interface{}{"judge": "nope"},
	}}}
	err := cfg.resolveJudgeParams()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `pack_evals bad: eval_params: judge "nope" is not defined in spec.judges`)
}
