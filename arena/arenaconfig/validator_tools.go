package arenaconfig

import (
	"fmt"
	"maps"
	"slices"

	"github.com/AltairaLabs/PromptKit/runtime/v2/skills"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

// Assertion types whose tool list names tools the model must call.
var toolCallAssertionTypes = map[string]bool{
	"tools_called":         true,
	"tools_called_session": true,
}

// validateToolUsage warns about tools the model is never offered. A prompt
// offers the model only the tools in its allowed_tools, plus whatever an
// active skill grants. A real model never calls a tool the config defines but
// no prompt or skill lists, and when a prompt offers no tools at all even a
// scripted mock call does not run, so a tools_called assertion fails with
// "got 0" — which reads like a mock problem rather than the missing
// allowed_tools entry it is.
func (v *ConfigValidator) validateToolUsage() {
	if len(v.config.LoadedPromptConfigs) == 0 {
		return // prompts come from a pack built elsewhere; nothing to compare
	}
	defined := v.getDefinedToolNames()
	if len(defined) == 0 {
		return
	}
	granted, known := v.getSkillGrantedToolNames()
	if !known {
		// A skill could not be read, so any tool might be granted at run
		// time. Saying a tool is unreachable would be a guess.
		return
	}
	if v.hasDuplexScenario() {
		// A duplex streaming session hands the model every registered tool,
		// whatever allowed_tools says, so no tool is unreachable.
		v.validateAssertedToolsOffered(defined, granted)
		return
	}
	allowed := v.getAllowedToolNames()

	check := ValidationCheck{Name: "Tools are offered by prompts", Passed: true}
	for _, name := range slices.Sorted(maps.Keys(defined)) {
		if allowed[name] || granted[name] {
			continue
		}
		issue := fmt.Sprintf(
			"tool %q is defined but no prompt lists it in allowed_tools, so the model is never offered it", name)
		check.Passed = false
		check.Warning = true
		check.Issues = append(check.Issues, issue)
		v.warns = append(v.warns, issue)
	}
	v.checks = append(v.checks, check)

	v.validateAssertedToolsOffered(defined, granted)
}

// validateAssertedToolsOffered warns when a scenario asserts that the model
// calls a tool its prompt does not offer. A mock can still pass such a
// scenario, because it executes whatever calls it scripts once the turn
// offers any tool at all; a real model cannot. Only tools the config defines
// are checked: capability, MCP and A2A tools are offered by other means.
func (v *ConfigValidator) validateAssertedToolsOffered(defined, granted map[string]bool) {
	for _, name := range slices.Sorted(maps.Keys(v.config.LoadedScenarios)) {
		scenario := v.config.LoadedScenarios[name]
		if scenario == nil || scenario.Duplex != nil {
			continue // a duplex session offers every registered tool
		}
		offered, prompts := v.toolsOfferedToScenario(scenario)
		if prompts == "" {
			continue // no prompt matches the task type; reported elsewhere
		}
		for _, a := range v.scenarioAssertions(scenario) {
			if !toolCallAssertionTypes[a.Type] {
				continue
			}
			for _, tool := range assertedToolNames(a.Params) {
				if !defined[tool] || offered[tool] || granted[tool] {
					continue
				}
				v.warns = append(v.warns, fmt.Sprintf(
					"scenario %q asserts %s on tool %q, which is not in the allowed_tools of %s; "+
						"a real model is never offered it, so only a mock scripting the call can satisfy the assertion",
					name, a.Type, tool, prompts))
			}
		}
	}
}

// toolsOfferedToScenario returns the tools the scenario's prompts list in
// allowed_tools, and those prompts described for a message ("" when none
// match). A workflow,
// composition or multi-agent config moves a conversation between prompts, so
// every prompt counts there; otherwise it is the prompts sharing the
// scenario's task_type.
func (v *ConfigValidator) toolsOfferedToScenario(scenario *Scenario) (map[string]bool, string) {
	if v.config.WorkflowSpec != nil || len(v.config.CompositionSpecs) > 0 || v.config.Agents != nil {
		return v.getAllowedToolNames(), "any prompt"
	}
	offered := make(map[string]bool)
	var ids []string
	for _, id := range slices.Sorted(maps.Keys(v.config.LoadedPromptConfigs)) {
		pc := v.config.LoadedPromptConfigs[id]
		if pc == nil || pc.Config == nil || promptTaskType(pc) != scenario.TaskType {
			continue
		}
		ids = append(ids, id)
		for _, tool := range promptAllowedTools(pc) {
			offered[tool] = true
		}
	}
	switch len(ids) {
	case 0:
		return offered, ""
	case 1:
		return offered, fmt.Sprintf("prompt %q (task_type %q)", ids[0], scenario.TaskType)
	default:
		return offered, fmt.Sprintf("any prompt with task_type %q", scenario.TaskType)
	}
}

func (v *ConfigValidator) hasDuplexScenario() bool {
	for _, s := range v.config.LoadedScenarios {
		if s != nil && s.Duplex != nil {
			return true
		}
	}
	return false
}

// getDefinedToolNames returns the names of the tools the config defines, as
// the runtime registers them.
func (v *ConfigValidator) getDefinedToolNames() map[string]bool {
	reg := tools.NewRegistry()
	for _, t := range v.config.LoadedTools {
		// A tool that does not parse is reported by the load stage.
		_ = reg.LoadToolFromBytes(t.FilePath, t.Data)
	}
	names := make(map[string]bool)
	for name := range reg.GetTools() {
		names[name] = true
	}
	return names
}

// getAllowedToolNames returns every tool any prompt lists in allowed_tools.
func (v *ConfigValidator) getAllowedToolNames() map[string]bool {
	allowed := make(map[string]bool)
	for _, pc := range v.config.LoadedPromptConfigs {
		for _, tool := range promptAllowedTools(pc) {
			allowed[tool] = true
		}
	}
	return allowed
}

// getSkillGrantedToolNames returns the tools the config's skills may grant on
// activation. known is false when the skills could not be discovered.
func (v *ConfigValidator) getSkillGrantedToolNames() (granted map[string]bool, known bool) {
	granted = make(map[string]bool)
	if len(v.config.LoadedSkillSources) == 0 {
		return granted, true
	}
	reg := skills.NewRegistry()
	if err := reg.Discover(v.config.SkillSources()); err != nil {
		return nil, false
	}
	for _, sk := range reg.List() {
		for _, tool := range sk.AllowedTools {
			granted[tool] = true
		}
	}
	return granted, true
}

func promptAllowedTools(pc *PromptConfigData) []string {
	if pc == nil || pc.Config == nil {
		return nil
	}
	if cfg, ok := pc.Config.(interface{ GetAllowedTools() []string }); ok {
		return cfg.GetAllowedTools()
	}
	return nil
}

func promptTaskType(pc *PromptConfigData) string {
	if cfg, ok := pc.Config.(interface{ GetTaskType() string }); ok {
		return cfg.GetTaskType()
	}
	return pc.TaskType
}

// scenarioAssertions returns the assertions evaluated for the scenario: the
// arena-level global conversation assertions, its own, and its turns'.
func (v *ConfigValidator) scenarioAssertions(s *Scenario) []AssertionConfig {
	var all []AssertionConfig
	if v.config.Globals != nil {
		all = append(all, v.config.Globals.ConversationAssertions...)
	}
	all = append(all, s.ConversationAssertions...)
	for i := range s.Turns {
		all = append(all, s.Turns[i].Assertions...)
	}
	return all
}

// assertedToolNames reads the tool list a tools_called assertion checks:
// tool_names, or tools when tool_names is absent, as the handlers do.
func assertedToolNames(params map[string]interface{}) []string {
	names := stringList(params["tool_names"])
	if len(names) == 0 {
		names = stringList(params["tools"])
	}
	return names
}

func stringList(v interface{}) []string {
	switch list := v.(type) {
	case []string:
		return list
	case []interface{}:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
