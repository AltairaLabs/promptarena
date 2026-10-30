package arenaconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
	"github.com/AltairaLabs/PromptKit/runtime/v2/workflow"
)

// stubPrompt implements the duck-typed accessors the validator reads from a
// loaded prompt config.
type stubPrompt struct {
	taskType string
	tools    []string
}

func (s stubPrompt) GetAllowedTools() []string { return s.tools }
func (s stubPrompt) GetTaskType() string       { return s.taskType }

func toolData(name string) config.ToolData {
	return config.ToolData{
		FilePath: name + ".tool.yaml",
		Data: []byte(fmt.Sprintf(`apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: %[1]s
spec:
  name: %[1]s
  description: test tool
  input_schema:
    type: object
  output_schema:
    type: object
  mode: mock
  mock_result: {}
`, name)),
	}
}

func scenarioAsserting(taskType, assertionType string, tools ...string) *Scenario {
	list := make([]interface{}, len(tools))
	for i, t := range tools {
		list[i] = t
	}
	return &Scenario{
		TaskType: taskType,
		Turns: []TurnDefinition{{
			Role: "user",
			Assertions: []AssertionConfig{
				{Type: assertionType, Params: map[string]interface{}{"tools": list}},
			},
		}},
	}
}

// The kit from the bug report: two tools defined, scripted in the mock and
// asserted, but the prompt has no allowed_tools.
func TestValidateToolUsage_PromptWithoutAllowedTools(t *testing.T) {
	cfg := &Config{
		LoadedTools: []config.ToolData{toolData("lookup_order"), toolData("refund_order")},
		LoadedPromptConfigs: map[string]*PromptConfigData{
			"support": {Config: stubPrompt{taskType: "support"}},
		},
		LoadedScenarios: map[string]*Scenario{
			"refund": scenarioAsserting("support", "tools_called_session", "lookup_order", "refund_order"),
		},
	}
	v := NewConfigValidatorWithPath(cfg, "")
	v.validateToolUsage()

	warns := v.GetWarnings()
	require.Len(t, warns, 4)
	assert.Contains(t, warns[0], `tool "lookup_order" is defined but no prompt lists it in allowed_tools`)
	assert.Contains(t, warns[1], `tool "refund_order"`)
	assert.Contains(t, warns[2],
		`scenario "refund" asserts tools_called_session on tool "lookup_order", which is not in the allowed_tools of prompt "support" (task_type "support")`)
	assert.Contains(t, warns[3], `"refund_order"`)

	checks := v.GetChecks()
	require.Len(t, checks, 1)
	assert.False(t, checks[0].Passed)
	assert.True(t, checks[0].Warning)
}

func TestValidateToolUsage_AllowedToolsClearsWarnings(t *testing.T) {
	cfg := &Config{
		LoadedTools: []config.ToolData{toolData("lookup_order")},
		LoadedPromptConfigs: map[string]*PromptConfigData{
			"support": {Config: stubPrompt{taskType: "support", tools: []string{"lookup_order"}}},
		},
		LoadedScenarios: map[string]*Scenario{
			"lookup": scenarioAsserting("support", "tools_called", "lookup_order"),
		},
	}
	v := NewConfigValidatorWithPath(cfg, "")
	v.validateToolUsage()

	assert.Empty(t, v.GetWarnings())
	require.Len(t, v.GetChecks(), 1)
	assert.True(t, v.GetChecks()[0].Passed)
}

// A tool another prompt offers is not dead, but a scenario on a prompt that
// does not offer it still cannot see it called.
func TestValidateToolUsage_ToolOfferedByAnotherPrompt(t *testing.T) {
	cfg := &Config{
		LoadedTools: []config.ToolData{toolData("refund_order")},
		LoadedPromptConfigs: map[string]*PromptConfigData{
			"triage":  {Config: stubPrompt{taskType: "triage"}},
			"billing": {Config: stubPrompt{taskType: "billing", tools: []string{"refund_order"}}},
		},
		LoadedScenarios: map[string]*Scenario{
			"triage-refund":  scenarioAsserting("triage", "tools_called", "refund_order"),
			"billing-refund": scenarioAsserting("billing", "tools_called", "refund_order"),
		},
	}
	v := NewConfigValidatorWithPath(cfg, "")
	v.validateToolUsage()

	warns := v.GetWarnings()
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], `scenario "triage-refund"`)
	assert.Contains(t, warns[0], `prompt "triage"`)
}

// tool_names is the canonical param; tools is the alias the handlers accept.
func TestValidateToolUsage_ReadsToolNamesParam(t *testing.T) {
	scenario := &Scenario{
		TaskType: "support",
		ConversationAssertions: []AssertionConfig{{
			Type:   "tools_called_session",
			Params: map[string]interface{}{"tool_names": []string{"lookup_order"}},
		}},
	}
	cfg := &Config{
		LoadedTools: []config.ToolData{toolData("lookup_order")},
		LoadedPromptConfigs: map[string]*PromptConfigData{
			"support": {Config: stubPrompt{taskType: "support"}},
			"other":   {Config: stubPrompt{taskType: "other", tools: []string{"lookup_order"}}},
		},
		LoadedScenarios: map[string]*Scenario{"s": scenario},
	}
	v := NewConfigValidatorWithPath(cfg, "")
	v.validateToolUsage()

	require.Len(t, v.GetWarnings(), 1)
	assert.Contains(t, v.GetWarnings()[0], `scenario "s" asserts tools_called_session on tool "lookup_order"`)
}

// Tools the config does not define (capability, MCP, A2A) are offered by other
// means, and other assertion types do not claim the tool is called.
func TestValidateToolUsage_IgnoresUndefinedToolsAndOtherAssertions(t *testing.T) {
	cfg := &Config{
		LoadedTools: []config.ToolData{toolData("lookup_order")},
		LoadedPromptConfigs: map[string]*PromptConfigData{
			"support": {Config: stubPrompt{taskType: "support", tools: []string{"lookup_order"}}},
			"quiet":   {Config: stubPrompt{taskType: "quiet"}},
		},
		LoadedScenarios: map[string]*Scenario{
			"memory": scenarioAsserting("support", "tools_called", "memory__recall"),
			"absent": scenarioAsserting("quiet", "tools_not_called", "lookup_order"),
		},
	}
	v := NewConfigValidatorWithPath(cfg, "")
	v.validateToolUsage()
	assert.Empty(t, v.GetWarnings())
}

// A workflow moves the conversation between prompts, so any prompt offering
// the tool counts for every scenario.
func TestValidateToolUsage_WorkflowUsesEveryPrompt(t *testing.T) {
	cfg := &Config{
		WorkflowSpec: &workflow.Spec{},
		LoadedTools:  []config.ToolData{toolData("refund_order")},
		LoadedPromptConfigs: map[string]*PromptConfigData{
			"intake":  {Config: stubPrompt{taskType: "intake"}},
			"billing": {Config: stubPrompt{taskType: "billing", tools: []string{"refund_order"}}},
		},
		LoadedScenarios: map[string]*Scenario{
			"refund": scenarioAsserting("intake", "tools_called_session", "refund_order"),
		},
	}
	v := NewConfigValidatorWithPath(cfg, "")
	v.validateToolUsage()
	assert.Empty(t, v.GetWarnings())
}

// A tool only a skill grants is reachable once the skill is activated.
func TestValidateToolUsage_SkillGrantCountsAsOffered(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "refunds")
	require.NoError(t, os.MkdirAll(skillDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: refunds
description: Process refunds.
allowed-tools: [refund_order]
---
Refund carefully.
`), 0o600))

	cfg := &Config{
		LoadedTools:        []config.ToolData{toolData("refund_order")},
		LoadedSkillSources: []prompt.SkillSourceConfig{{Path: dir}},
		LoadedPromptConfigs: map[string]*PromptConfigData{
			"support": {Config: stubPrompt{taskType: "support"}},
		},
		LoadedScenarios: map[string]*Scenario{
			"refund": scenarioAsserting("support", "tools_called_session", "refund_order"),
		},
	}
	v := NewConfigValidatorWithPath(cfg, "")
	v.validateToolUsage()
	assert.Empty(t, v.GetWarnings())
}

// When skills cannot be read, any tool might be granted, so nothing is
// reported as unreachable.
func TestValidateToolUsage_UnreadableSkillsSuppressWarnings(t *testing.T) {
	cfg := &Config{
		LoadedTools:        []config.ToolData{toolData("refund_order")},
		LoadedSkillSources: []prompt.SkillSourceConfig{{Path: filepath.Join(t.TempDir(), "missing")}},
		LoadedPromptConfigs: map[string]*PromptConfigData{
			"support": {Config: stubPrompt{taskType: "support"}},
		},
	}
	v := NewConfigValidatorWithPath(cfg, "")
	v.validateToolUsage()
	assert.Empty(t, v.GetWarnings())
}

// A duplex streaming session offers the model every registered tool, so
// neither warning applies to it — but a text scenario alongside one still
// gets the scenario warning.
func TestValidateToolUsage_DuplexScenarioOffersEveryTool(t *testing.T) {
	voice := scenarioAsserting("support", "tools_called", "lookup_order")
	voice.Duplex = &DuplexConfig{}
	cfg := &Config{
		LoadedTools: []config.ToolData{toolData("lookup_order")},
		LoadedPromptConfigs: map[string]*PromptConfigData{
			"support": {Config: stubPrompt{taskType: "support"}},
		},
		LoadedScenarios: map[string]*Scenario{
			"voice": voice,
			"text":  scenarioAsserting("support", "tools_called", "lookup_order"),
		},
	}
	v := NewConfigValidatorWithPath(cfg, "")
	v.validateToolUsage()

	require.Len(t, v.GetWarnings(), 1)
	assert.Contains(t, v.GetWarnings()[0], `scenario "text"`)
}

// With no prompt configs the prompts come from a pack built elsewhere, so
// there is no allowed_tools to compare against.
func TestValidateToolUsage_NoPromptConfigs(t *testing.T) {
	cfg := &Config{LoadedTools: []config.ToolData{toolData("lookup_order")}}
	v := NewConfigValidatorWithPath(cfg, "")
	v.validateToolUsage()
	assert.Empty(t, v.GetWarnings())
}

// Global conversation assertions run for every scenario, so they are checked
// against each scenario's prompt too.
func TestValidateToolUsage_GlobalAssertions(t *testing.T) {
	cfg := &Config{
		Globals: &Globals{ConversationAssertions: []AssertionConfig{{
			Type:   "tools_called_session",
			Params: map[string]interface{}{"tools": []interface{}{"audit_log"}},
		}}},
		LoadedTools: []config.ToolData{toolData("audit_log")},
		LoadedPromptConfigs: map[string]*PromptConfigData{
			"support": {Config: stubPrompt{taskType: "support"}},
			"audit":   {Config: stubPrompt{taskType: "audit", tools: []string{"audit_log"}}},
		},
		LoadedScenarios: map[string]*Scenario{
			"chat":  {TaskType: "support"},
			"audit": {TaskType: "audit"},
		},
	}
	v := NewConfigValidatorWithPath(cfg, "")
	v.validateToolUsage()

	require.Len(t, v.GetWarnings(), 1)
	assert.Contains(t, v.GetWarnings()[0], `scenario "chat" asserts tools_called_session on tool "audit_log"`)
}
