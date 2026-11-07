package assertions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
)

// Param keys the checks below treat specially.
const (
	pPatterns   = "patterns"
	pToolNames  = "tool_names"
	pTools      = "tools"
	pAgents     = "agents"
	pSkillNames = "skill_names"
	pFields     = "fields"
	pSequence   = "sequence"
	pMessage    = "message"
	pEvalType   = "eval_type"
	pEvalParams = "eval_params"
)

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
}

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
// such as guardrail_triggered's validator_type). The params each handler reads
// come from the registry (EvalTypeRegistry.ParamNames); a handler that does
// not list them has its keys left unchecked. Types the registry does not know
// are reported by ValidateAssertionTypes, not here.
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
	known, listed := registry.ParamNames(typ)
	if isWrapper(handler.Type()) {
		if listed {
			f.checkKeys(prefix, handler.Type(), params, known)
		}
		if inner, ok := params[pEvalType].(string); ok && inner != "" {
			innerParams, _ := params[pEvalParams].(map[string]any)
			f.check(owner, loc+"."+pEvalParams, inner, innerParams, registry)
		}
		return
	}
	normalized := evals.NormalizeParams(typ, params)
	if listed {
		f.checkKeys(prefix, handler.Type(), normalized, known)
	}
	if pv, ok := handler.(evals.ParamValidator); ok {
		inner := evals.StripScoreThresholds(evals.ApplyDefaults(typ, normalized))
		if verr := pv.ValidateParams(inner); verr != nil {
			f.Errors = append(f.Errors, fmt.Sprintf("%s: %v", prefix, verr))
		}
	}
}

// isWrapper reports whether a canonical eval type wraps another: its params
// name the inner type (eval_type) and carry its params (eval_params), which
// are checked as that type's.
func isWrapper(canonical string) bool {
	return canonical == evals.WrapperTypeAssertion || canonical == evals.WrapperTypeGuardrail
}

// checkKeys reports each param key the handler does not read (known is the
// handler's own list, from the eval registry), and each list param given a
// scalar.
func (f *ParamFindings) checkKeys(prefix, canonical string, params map[string]any, known []string) {
	allowed := make(map[string]bool, len(known))
	for _, k := range known {
		allowed[k] = true
	}
	for _, key := range sortedParamKeys(params) {
		switch {
		case assertionWrapperKeys[key]:
			// Thresholds belong to the assertion wrapper.
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

func unknownParamMessage(canonical, key string, known []string) string {
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
