package assertions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

func scenarioWith(a ...arenaconfig.AssertionConfig) *arenaconfig.Config {
	return &arenaconfig.Config{LoadedScenarios: map[string]*arenaconfig.Scenario{
		"s": {Turns: []arenaconfig.TurnDefinition{{Assertions: a}}},
	}}
}

func paramFindings(t *testing.T, a ...arenaconfig.AssertionConfig) ParamFindings {
	t.Helper()
	return ValidateConfigAssertionParams(scenarioWith(a...), evals.NewEvalTypeRegistry())
}

func TestValidateConfigAssertionParams_ValidParamsPass(t *testing.T) {
	f := paramFindings(t,
		arenaconfig.AssertionConfig{Type: "content_includes", Params: map[string]any{"patterns": []any{"x"}}},
		arenaconfig.AssertionConfig{Type: "tool_called", Params: map[string]any{"tool_names": []any{"a"}, "min_calls": 1}},
		arenaconfig.AssertionConfig{Type: "banned_words", Params: map[string]any{"words": []any{"x"}}},
		arenaconfig.AssertionConfig{Type: "max_length", Params: map[string]any{"max_characters": 10, "min_score": 1}},
		arenaconfig.AssertionConfig{Type: "llm_judge", Params: map[string]any{
			"criteria": "c", "judge": "j", "provider": "j",
		}},
		arenaconfig.AssertionConfig{Type: "guardrail_triggered", Params: map[string]any{
			"validator_type": "banned_words", "should_trigger": true,
		}},
		arenaconfig.AssertionConfig{Type: "refusal"}, // not in the table: unchecked
		arenaconfig.AssertionConfig{Type: "no_such_type", Params: map[string]any{"x": 1}},
		arenaconfig.AssertionConfig{},
	)
	assert.Empty(t, f.Errors)
	assert.Empty(t, f.Warnings)
}

func TestValidateConfigAssertionParams_MessageInParamsWarns(t *testing.T) {
	f := paramFindings(t, arenaconfig.AssertionConfig{
		Type: "content_includes", Params: map[string]any{"patterns": []any{"x"}, "message": "m"},
	})
	assert.Empty(t, f.Errors)
	require.Len(t, f.Warnings, 1)
	assert.Contains(t, f.Warnings[0], "s: turns[0].assertions: content_includes: params.message is ignored")
	assert.Contains(t, f.Warnings[0], "beside type")
}

func TestValidateConfigAssertionParams_UnknownParamsError(t *testing.T) {
	f := paramFindings(t,
		arenaconfig.AssertionConfig{Type: "tool_called", Params: map[string]any{"tool_name": "a"}},
		arenaconfig.AssertionConfig{Type: "llm_judge", Params: map[string]any{"criteria": "c", "judge_provider": "x"}},
		arenaconfig.AssertionConfig{Type: "llm_judge", Params: map[string]any{"criteria": "c", "model": "x"}},
		arenaconfig.AssertionConfig{Type: "content_includes", Params: map[string]any{
			"patterns": []any{"x"}, "case_sensitive": true,
		}},
		arenaconfig.AssertionConfig{Type: "workflow_complete", Params: map[string]any{"state": "x"}},
		arenaconfig.AssertionConfig{Type: "max_sentences", Params: map[string]any{"max_sentences": 3}},
	)
	require.Len(t, f.Errors, 6)
	joined := f.Errors
	assert.Contains(t, joined[0], `content_includes: unknown param "case_sensitive": case_sensitive is not supported`)
	assert.Contains(t, joined[1], `llm_judge: unknown param "judge_provider": select a judge with judge:`)
	assert.Contains(t, joined[2], `llm_judge: unknown param "model"`)
	assert.Contains(t, joined[3], "max_sentences: sentence_count measures and always scores 1.0")
	assert.Contains(t, joined[4], `tool_called: unknown param "tool_name" (did you mean "tool_names"?)`)
	assert.Contains(t, joined[5], `workflow_complete: unknown param "state"`)
	assert.Contains(t, joined[5], "takes no params")
}

func TestValidateConfigAssertionParams_ListParamGivenScalar(t *testing.T) {
	f := paramFindings(t, arenaconfig.AssertionConfig{
		Type: "contains", Params: map[string]any{"patterns": "refund"},
	})
	require.Len(t, f.Errors, 1)
	assert.Contains(t, f.Errors[0], "params.patterns must be a list (got string)")
}

func TestValidateConfigAssertionParams_HandlerValidateParams(t *testing.T) {
	// guardrail_triggered needs validator_type; `guardrail` is not a param.
	f := paramFindings(t, arenaconfig.AssertionConfig{
		Type: "guardrail_triggered", Params: map[string]any{"guardrail": "banned_words"},
	})
	require.Len(t, f.Errors, 2)
	assert.Contains(t, f.Errors[0], "requires a non-empty 'validator_type'")
	assert.Contains(t, f.Errors[1], `unknown param "guardrail"`)
}

func TestValidateConfigAssertionParams_WrapperChecksInnerParams(t *testing.T) {
	f := paramFindings(t, arenaconfig.AssertionConfig{
		Type: "assertion", Params: map[string]any{
			"eval_type":   "llm_judge",
			"eval_params": map[string]any{"criteria": "c", "prompt": "p"},
			"min_score":   0.8,
			"bogus":       1,
		},
	})
	require.Len(t, f.Errors, 2)
	assert.Contains(t, f.Errors[0], `s: turns[0].assertions.eval_params: llm_judge: unknown param "prompt"`)
	assert.Contains(t, f.Errors[1], `s: turns[0].assertions: assertion: unknown param "bogus"`)
}

func TestValidateConfigAssertionParams_EveryLocation(t *testing.T) {
	bad := []arenaconfig.AssertionConfig{{Type: "contains", Params: map[string]any{"x": 1}}}
	cfg := &arenaconfig.Config{
		Globals: &arenaconfig.Globals{ConversationAssertions: bad},
		LoadedScenarios: map[string]*arenaconfig.Scenario{
			"s": {
				ConversationAssertions: bad,
				Turns: []arenaconfig.TurnDefinition{{Objective: &arenaconfig.ObjectiveConfig{
					Success: bad, Rewind: &arenaconfig.ObjectiveRewindConfig{When: bad},
				}}},
			},
			"nil": nil,
		},
		LoadedEvals: map[string]*arenaconfig.Eval{
			"e": {
				ConversationAssertions: bad,
				Turns: []arenaconfig.EvalTurnConfig{
					{AllTurns: &arenaconfig.EvalAllTurnsConfig{Assertions: bad}}, {},
				},
			},
			"nil": nil,
		},
		LoadedPersonas: map[string]*arenaconfig.UserPersonaPack{
			"p":   {Transitions: []arenaconfig.PersonaTransition{{When: bad}}},
			"nil": nil,
		},
	}
	f := ValidateConfigAssertionParams(cfg, evals.NewEvalTypeRegistry())
	require.Len(t, f.Errors, 7)
	for _, want := range []string{
		"globals: conversation_assertions",
		"s: conversation_assertions",
		"s: turns[0].objective.success",
		"s: turns[0].objective.rewind.when",
		"eval e: conversation_assertions",
		"eval e: turns[0].all_turns.assertions",
		"persona p: transitions[0].when",
	} {
		found := false
		for _, e := range f.Errors {
			if len(e) >= len(want) && e[:len(want)] == want {
				found = true
			}
		}
		assert.True(t, found, "missing %q in %v", want, f.Errors)
	}
	assert.Empty(t, ValidateConfigAssertionParams(nil, evals.NewEvalTypeRegistry()).Errors)
}

func TestValidatePromptValidatorParams(t *testing.T) {
	prompts := map[string]*arenaconfig.PromptConfigData{
		"support": {Config: &prompt.Config{Spec: prompt.Spec{Validators: []prompt.ValidatorConfig{
			{Type: "max_sentences", Params: map[string]any{"max_sentences": 3}},
			{Type: "banned_words", Params: map[string]any{"words": []any{"x"}, "case_sensitive": false}},
			{Type: "banned_words", Params: map[string]any{"words": []any{"x"}}},
			{Type: "not_registered"},
		}}}},
		"other": {Config: "not a prompt config"},
		"nil":   nil,
	}
	warns := ValidatePromptValidatorParams(prompts, evals.NewEvalTypeRegistry())
	require.Len(t, warns, 2)
	assert.Contains(t, warns[0], "prompt support: validators[0] (max_sentences): sentence_count measures")
	assert.Contains(t, warns[0], "as a guardrail it never fires")
	assert.Contains(t, warns[1], "prompt support: validators[1] (banned_words): case_sensitive is not supported")
}
