package arenaconfig

import (
	"fmt"
	"sort"
)

const (
	paramJudge      = "judge"
	paramProvider   = "provider"
	paramEvalParams = "eval_params"
)

// resolveJudgeParams makes a judge-backed check's `judge: <name>` select that
// judge. PromptKit's judge handlers pick a judge by the `provider` param,
// which the engine's provider binding resolves (judge names are aliases onto
// their providers' ids); without one they fall back to the lowest-sorted
// judge, so `judge:` used to be ignored whenever several judges were
// configured. A `judge` naming no configured judge is a load error. An
// explicit `provider` is left alone.
func (c *Config) resolveJudgeParams() error {
	for _, set := range c.assertionSets() {
		for i := range set.assertions {
			a := &set.assertions[i]
			if err := c.resolveJudgeParam(a.Params); err != nil {
				return fmt.Errorf("%s: %s assertion: %w", set.where, a.Type, err)
			}
			if inner, ok := a.Params[paramEvalParams].(map[string]interface{}); ok {
				if err := c.resolveJudgeParam(inner); err != nil {
					return fmt.Errorf("%s: %s assertion: eval_params: %w", set.where, a.Type, err)
				}
			}
		}
	}
	return nil
}

func (c *Config) resolveJudgeParam(params map[string]interface{}) error {
	name, ok := params[paramJudge].(string)
	if !ok || name == "" {
		return nil
	}
	if _, known := c.LoadedJudges[name]; !known {
		return fmt.Errorf("judge %q is not defined in spec.judges", name)
	}
	if _, set := params[paramProvider]; !set {
		params[paramProvider] = name
	}
	return nil
}

type assertionSet struct {
	where      string
	assertions []AssertionConfig
}

// assertionSets lists every assertion list the config evaluates, labeled for
// error messages, in a stable order.
func (c *Config) assertionSets() []assertionSet {
	var sets []assertionSet
	if c.Globals != nil {
		sets = append(sets, assertionSet{"globals.conversation_assertions", c.Globals.ConversationAssertions})
	}
	for _, id := range sortedKeys(c.LoadedScenarios) {
		s := c.LoadedScenarios[id]
		if s == nil {
			continue
		}
		where := "scenario " + id
		sets = append(sets, assertionSet{where + " conversation_assertions", s.ConversationAssertions})
		for i := range s.Turns {
			sets = append(sets, assertionSet{fmt.Sprintf("%s turns[%d]", where, i), s.Turns[i].Assertions})
		}
	}
	for _, id := range sortedKeys(c.LoadedEvals) {
		ev := c.LoadedEvals[id]
		if ev == nil {
			continue
		}
		where := "eval " + id
		sets = append(sets, assertionSet{where + " conversation_assertions", ev.ConversationAssertions})
		for i := range ev.Turns {
			if ev.Turns[i].AllTurns != nil {
				sets = append(sets, assertionSet{fmt.Sprintf("%s turns[%d]", where, i), ev.Turns[i].AllTurns.Assertions})
			}
		}
	}
	return sets
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
