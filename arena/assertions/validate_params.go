package assertions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// Param names that recur in the tables below.
const (
	pPatterns     = "patterns"
	pPattern      = "pattern"
	pToolNames    = "tool_names"
	pTools        = "tools"
	pTool         = "tool"
	pToolName     = "tool_name"
	pExpectedArgs = "expected_args"
	pMinCalls     = "min_calls"
	pMaxCalls     = "max_calls"
	pMin          = "min"
	pMax          = "max"
	pState        = "state"
	pSkillNames   = "skill_names"
	pAgents       = "agents"
	pFields       = "fields"
	pOccurrence   = "occurrence"
	pSequence     = "sequence"
	pAllowWrapped = "allow_wrapped"
	pExtractJSON  = "extract_json"
	pMessage      = "message"
	pJudge        = "judge"
	pProvider     = "provider"
	pCaseSens     = "case_sensitive"
	pEvalType     = "eval_type"
	pEvalParams   = "eval_params"

	typeSentenceCount   = "sentence_count"
	typeContentExcludes = "content_excludes"
	typeContains        = "contains"
)

// judgeParams are the params every LLM-judge handler reads, plus Arena's own
// `judge`, which the loader resolves to `provider`.
var judgeParams = []string{"criteria", "rubric", "system_prompt", "extra", pProvider, pJudge}

// knownAssertionParams lists, per canonical eval type, every param its handler
// reads (after PromptKit's legacy-name normalization). A type listed here has
// its params checked by `promptarena validate`: an unknown key is an error,
// because the handler would silently ignore it. Types not listed are not
// checked. Keep each entry evidenced against the handler source in the pinned
// runtime (evals/handlers/<type>.go); an entry that omits a real param turns a
// valid config into a validate error.
var knownAssertionParams = map[string][]string{
	typeContains:                 {pPatterns},
	"contains_any":               {pPatterns},
	typeContentExcludes:          {pPatterns, "match_mode"},
	"regex":                      {pPattern, "expect_match"},
	"json_valid":                 {pAllowWrapped, pExtractJSON},
	"json_schema":                {"schema", pAllowWrapped, pExtractJSON},
	"tools_called":               {pToolNames, pTools, pMinCalls, pMaxCalls, "ignore_validation", "require_args"},
	"tools_not_called":           {pToolNames, pTools},
	"tools_offered":              {pToolNames, pTools, "exact", "absent"},
	"tools_called_session":       {pToolNames, pTools, pMinCalls},
	"tools_not_called_session":   {pToolNames, pTools},
	"tool_args":                  {pToolName, pExpectedArgs},
	"tool_args_session":          {pToolName, pExpectedArgs},
	"tool_args_excluded_session": {pToolName, "excluded_args", "forbidden_args"},
	"tool_calls_with_args": {
		pToolName, pExpectedArgs, "args_match", "result_includes", "result_matches", "no_error",
	},
	"tool_call_count":         {pTool, pMin, pMax},
	"tool_call_sequence":      {pSequence},
	"tool_result_includes":    {pTool, pPatterns, pOccurrence},
	"tool_result_matches":     {pTool, pPattern, pOccurrence},
	"no_tool_errors":          {pTools},
	"tool_no_repeat":          {pTools, "max_repeats"},
	"tool_efficiency":         {pMaxCalls, "max_errors", "max_error_rate"},
	"tool_anti_pattern":       {pPatterns},
	"max_length":              {pMax, "max_tokens"},
	"min_length":              {pMin},
	"latency_budget":          {"max_ms"},
	"cost_budget":             {"max_cost_usd", "max_input_tokens", "max_output_tokens", "max_total_tokens"},
	"guardrail_triggered":     {"validator_type", "validator", "should_trigger", "direction"},
	"state_is":                {pState},
	"transitioned_to":         {pState},
	"workflow_complete":       {},
	"skill_activated":         {pSkillNames, pMinCalls},
	"skill_not_activated":     {pSkillNames},
	"field_presence":          {pFields},
	"agent_invoked":           {pAgents},
	"agent_not_invoked":       {pAgents},
	"agent_response_contains": {"agent", typeContains},
	typeSentenceCount:         {},
	"llm_judge":               judgeParams,
	"llm_judge_session":       judgeParams,
	"llm_judge_tool_calls":    append(append([]string{}, judgeParams...), pTools),
}

// wrapperTypes are the eval types that wrap another: their params name the
// inner type (eval_type) and carry its params (eval_params), which are checked
// as that type's.
var wrapperTypes = map[string][]string{
	evals.WrapperTypeAssertion: {pEvalType, pEvalParams},
	evals.WrapperTypeGuardrail: {pEvalType, pEvalParams, "action"},
}

// listParams are params whose handler reads a list and silently treats any
// other value as absent, so `patterns: "x"` fails at run time with "no
// patterns specified".
var listParams = map[string]bool{
	pPatterns: true, pToolNames: true, pTools: true, pAgents: true, pSkillNames: true, pFields: true,
	pSequence: true,
}

// paramHints explains an unknown param better than an edit-distance guess:
// the right home for a value that is not a handler param at all.
var paramHints = map[string]string{
	pMessage:         "move it out of params to sit beside type (the assertion's own message)",
	"judge_provider": "select a judge with judge: <name from spec.judges>",
	"model": "a judge grades with its provider's model, so select a different judge " +
		"with judge: <name from spec.judges>",
	pCaseSens: "case_sensitive is not supported: matching is always case-insensitive",
}

// sentenceCountNote is reported for a sentence_count / max_sentences check
// given a bound: the handler measures and always scores 1.0.
const sentenceCountNote = "sentence_count measures and always scores 1.0, so it never enforces a maximum " +
	"(the max / max_sentences param is ignored)"

// ParamFindings is what ValidateConfigAssertionParams reports. Errors change
// what a check does (an ignored param, a scalar where a list is needed, a
// missing required param); warnings do not change pass or fail.
type ParamFindings struct {
	Errors   []string
	Warnings []string
}

// ValidateConfigAssertionParams checks the params of every assertion the
// loaded config evaluates (globals, scenarios, evals and persona transitions)
// against what its handler reads: unknown or misplaced keys, a scalar where the
// handler needs a list, and the handler's own ValidateParams (required params
// such as guardrail_triggered's validator_type). Types the registry does not
// know are reported by ValidateAssertionTypes, not here.
//
// A `message` inside params is a warning: the assertion still runs, only its
// description is lost. Every other unknown key is an error, because the
// handler silently ignores it and the check is not the one that was written.
func ValidateConfigAssertionParams(cfg *arenaconfig.Config, registry *evals.EvalTypeRegistry) ParamFindings {
	var f ParamFindings
	if cfg == nil {
		return f
	}
	visit := func(owner, loc string, list []arenaconfig.AssertionConfig) {
		for i := range list {
			f.check(owner, loc, list[i].Type, list[i].Params, registry)
		}
	}
	if cfg.Globals != nil {
		visit("globals", "conversation_assertions", cfg.Globals.ConversationAssertions)
	}
	for name, s := range cfg.LoadedScenarios {
		if s != nil {
			forEachScenarioAssertionList(s, func(loc string, list []arenaconfig.AssertionConfig) {
				visit(name, loc, list)
			})
		}
	}
	for name, ev := range cfg.LoadedEvals {
		forEachEvalAssertionList(ev, func(loc string, list []arenaconfig.AssertionConfig) {
			visit("eval "+name, loc, list)
		})
	}
	for name, p := range cfg.LoadedPersonas {
		forEachPersonaAssertionList(p, func(loc string, list []arenaconfig.AssertionConfig) {
			visit("persona "+name, loc, list)
		})
	}
	sort.Strings(f.Errors)
	sort.Strings(f.Warnings)
	return f
}

// forEachEvalAssertionList calls fn with every assertion list an eval declares.
func forEachEvalAssertionList(ev *arenaconfig.Eval, fn func(loc string, list []arenaconfig.AssertionConfig)) {
	if ev == nil {
		return
	}
	fn("conversation_assertions", ev.ConversationAssertions)
	for i := range ev.Turns {
		if ev.Turns[i].AllTurns != nil {
			fn(fmt.Sprintf("turns[%d].all_turns.assertions", i), ev.Turns[i].AllTurns.Assertions)
		}
	}
}

// forEachPersonaAssertionList calls fn with each persona transition's assertions.
func forEachPersonaAssertionList(
	p *arenaconfig.UserPersonaPack, fn func(loc string, list []arenaconfig.AssertionConfig),
) {
	if p == nil {
		return
	}
	for i := range p.Transitions {
		fn(fmt.Sprintf("transitions[%d].when", i), p.Transitions[i].When)
	}
}

// forEachScenarioAssertionList calls fn with every assertion list a scenario
// declares and where it is declared.
func forEachScenarioAssertionList(s *arenaconfig.Scenario, fn func(loc string, list []arenaconfig.AssertionConfig)) {
	fn("conversation_assertions", s.ConversationAssertions)
	for i := range s.Turns {
		turn := &s.Turns[i]
		fn(fmt.Sprintf("turns[%d].assertions", i), turn.Assertions)
		if obj := turn.Objective; obj != nil {
			fn(fmt.Sprintf("turns[%d].objective.success", i), obj.Success)
			if obj.Rewind != nil {
				fn(fmt.Sprintf("turns[%d].objective.rewind.when", i), obj.Rewind.When)
			}
		}
	}
}

func (f *ParamFindings) check(owner, loc, typ string, params map[string]any, registry *evals.EvalTypeRegistry) {
	if typ == "" {
		return
	}
	handler, err := registry.Get(typ)
	if err != nil {
		return // unknown type: ValidateAssertionTypes reports it
	}
	prefix := fmt.Sprintf("%s: %s: %s", owner, loc, typ)
	if keys, wrapper := wrapperTypes[handler.Type()]; wrapper {
		f.checkKeys(prefix, handler.Type(), params, keys)
		if inner, ok := params[pEvalType].(string); ok && inner != "" {
			innerParams, _ := params[pEvalParams].(map[string]any)
			f.check(owner, loc+"."+pEvalParams, inner, innerParams, registry)
		}
		return
	}
	normalized := evals.NormalizeParams(typ, params)
	if known, listed := knownAssertionParams[handler.Type()]; listed {
		f.checkKeys(prefix, handler.Type(), normalized, known)
	}
	if pv, ok := handler.(evals.ParamValidator); ok {
		inner := evals.StripScoreThresholds(evals.ApplyDefaults(typ, normalized))
		if verr := pv.ValidateParams(inner); verr != nil {
			f.Errors = append(f.Errors, fmt.Sprintf("%s: %v", prefix, verr))
		}
	}
}

// checkKeys reports each param key the handler does not read, and each list
// param given a scalar.
func (f *ParamFindings) checkKeys(prefix, canonical string, params map[string]any, known []string) {
	allowed := make(map[string]bool, len(known))
	for _, k := range known {
		allowed[k] = true
	}
	for _, key := range sortedParamKeys(params) {
		switch {
		case assertionWrapperKeys[key] || loaderAddedProvider(key, params):
			// Thresholds belong to the assertion wrapper; a loader-added
			// provider is reported once, as the judge key it came from.
		case key == pMessage && !allowed[key]:
			f.Warnings = append(f.Warnings,
				fmt.Sprintf("%s: params.message is ignored; %s", prefix, paramHints[pMessage]))
		case !allowed[key]:
			f.Errors = append(f.Errors, fmt.Sprintf("%s: %s", prefix, unknownParamMessage(canonical, key, known)))
		case listParams[key] && !isList(params[key]):
			f.Errors = append(f.Errors, fmt.Sprintf("%s: params.%s must be a list (got %T); write %s: [\"...\"]",
				prefix, key, params[key], key))
		}
	}
}

// loaderAddedProvider reports a `provider` the loader copied from `judge`
// (resolveJudgeParams).
func loaderAddedProvider(key string, params map[string]any) bool {
	return key == pProvider && params[pJudge] != nil && params[pJudge] == params[pProvider]
}

func unknownParamMessage(canonical, key string, known []string) string {
	if canonical == typeSentenceCount && key == pMax {
		return sentenceCountNote
	}
	msg := fmt.Sprintf("unknown param %q", key)
	if hint, ok := paramHints[key]; ok {
		return msg + ": " + hint
	}
	if suggestion := closestMatch(key, known); suggestion != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", suggestion)
	}
	if len(known) == 0 {
		return msg + "; " + canonical + " takes no params"
	}
	return msg + "; " + canonical + " accepts: " + strings.Join(known, ", ")
}

// ValidatePromptValidatorParams returns warnings for prompt validators
// (guardrails) whose params the handler ignores in a way that defeats the
// guardrail: a sentence_count / max_sentences guardrail, which never fires,
// and case_sensitive on a content_excludes / banned_words guardrail, which is
// never read. They are warnings, not errors: the runtime loads such a pack, and
// a pack built for a newer runtime may carry params this one does not know.
func ValidatePromptValidatorParams(
	prompts map[string]*arenaconfig.PromptConfigData, registry *evals.EvalTypeRegistry,
) []string {
	var warns []string
	for id, pc := range prompts {
		if pc == nil {
			continue
		}
		cfg, ok := pc.Config.(*prompt.Config)
		if !ok || cfg == nil {
			continue
		}
		for i := range cfg.Spec.Validators {
			if w := validatorWarning(&cfg.Spec.Validators[i], registry); w != "" {
				warns = append(warns, fmt.Sprintf("prompt %s: validators[%d] (%s): %s", id, i, cfg.Spec.Validators[i].Type, w))
			}
		}
	}
	sort.Strings(warns)
	return warns
}

// validatorWarning is the warning for one prompt validator, or "".
func validatorWarning(v *prompt.ValidatorConfig, registry *evals.EvalTypeRegistry) string {
	handler, err := registry.Get(v.Type)
	if err != nil {
		return "" // the pack stage reports an unknown validator type
	}
	switch handler.Type() {
	case typeSentenceCount:
		return sentenceCountNote + "; as a guardrail it never fires"
	case typeContentExcludes:
		if _, set := v.Params[pCaseSens]; set {
			return paramHints[pCaseSens] + "; remove the param"
		}
	}
	return ""
}

func isList(v any) bool {
	switch v.(type) {
	case []any, []string:
		return true
	}
	return false
}

func sortedParamKeys(params map[string]any) []string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
