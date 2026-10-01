package arenaconfig

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/AltairaLabs/PromptKit/runtime/v2/workflow"
)

// workflowFromMap decodes a raw workflow document into the typed spec the
// config carries, so tests can keep writing states as literal maps.
func workflowFromMap(raw map[string]any) *workflow.Spec {
	spec, err := workflow.ParseConfig(raw)
	if err != nil {
		panic(err)
	}
	return spec
}

// workflowValidatorConfig returns a config whose loaded prompts cover the
// task types the workflow references.
func workflowValidatorConfig(states map[string]any) *Config {
	return &Config{
		LoadedPromptConfigs: map[string]*PromptConfigData{
			"route":   {TaskType: "route"},
			"confirm": {TaskType: "confirm"},
		},
		Workflow: workflowFromMap(map[string]any{
			"version": 2,
			"entry":   "route",
			"states":  states,
		}),
	}
}

// hasWarningContaining reports whether any validator warning mentions s. The
// validator also emits baseline warnings for a sparse config (no providers,
// no scenarios), so tests look for their own warning rather than the only one.
func hasWarningContaining(v *ConfigValidator, s string) bool {
	for _, w := range v.GetWarnings() {
		if strings.Contains(w, s) {
			return true
		}
	}
	return false
}

func TestConfigValidator_Workflow_AbsentIsNoop(t *testing.T) {
	v := NewConfigValidatorWithPath(&Config{}, "")
	require.NoError(t, v.Validate())
	for _, w := range v.GetWarnings() {
		assert.NotContains(t, w, "workflow")
	}
}

// The workflow is typed, so a malformed shape no longer reaches the validator:
// it fails decoding the config, before any business-logic check runs.
func TestConfig_Workflow_MalformedFailsDecoding(t *testing.T) {
	var cfg Config
	err := yaml.Unmarshal([]byte("workflow:\n  states: not-a-map\n"), &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-map")
}

// RFC 0014: both spec values validate cleanly; a typo is an error rather than
// a value silently resolved to a default.
func TestConfigValidator_Workflow_ControlValues(t *testing.T) {
	build := func(control string) *Config {
		return workflowValidatorConfig(map[string]any{
			"route": map[string]any{
				"prompt_task": "route",
				"control":     control,
				"on_event":    map[string]any{"Next": "confirm"},
			},
			"confirm": map[string]any{
				"prompt_task": "confirm",
				"terminal":    true,
			},
		})
	}

	for _, control := range []string{"user", "agent"} {
		t.Run(control+" is accepted", func(t *testing.T) {
			v := NewConfigValidatorWithPath(build(control), "")
			require.NoError(t, v.Validate())
		})
	}

	t.Run("unrecognized value is an error", func(t *testing.T) {
		v := NewConfigValidatorWithPath(build("agent_but_typoed"), "")
		err := v.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent_but_typoed")
		assert.Contains(t, err.Error(), "control")
	})
}

// RFC 0014 rule 3: terminal wins, so control agent beside it does nothing.
func TestConfigValidator_Workflow_WarnsOnAgentPlusTerminal(t *testing.T) {
	cfg := workflowValidatorConfig(map[string]any{
		"route": map[string]any{
			"prompt_task": "route",
			"on_event":    map[string]any{"Next": "confirm"},
		},
		"confirm": map[string]any{
			"prompt_task": "confirm",
			"control":     "agent",
			"terminal":    true,
		},
	})
	v := NewConfigValidatorWithPath(cfg, "")
	require.NoError(t, v.Validate(), "agent + terminal is a warning, not an error")
	assert.True(t, hasWarningContaining(v, "has no effect"), "warnings: %v", v.GetWarnings())
}

// RFC 0014 rule 4: two agent states pointing at each other with nothing that
// yields, ends, or caps visits will talk to themselves until a budget stops it.
func TestConfigValidator_Workflow_WarnsOnUnboundedAgentCycle(t *testing.T) {
	cfg := workflowValidatorConfig(map[string]any{
		"route": map[string]any{
			"prompt_task": "route",
			"control":     "agent",
			"on_event":    map[string]any{"Next": "confirm"},
		},
		"confirm": map[string]any{
			"prompt_task": "confirm",
			"control":     "agent",
			"on_event":    map[string]any{"Back": "route"},
		},
	})
	v := NewConfigValidatorWithPath(cfg, "")
	require.NoError(t, v.Validate())
	assert.True(t, hasWarningContaining(v, "agent-controlled cycle"), "warnings: %v", v.GetWarnings())
}

// prompt_task must resolve against the loaded prompt configs, which is what
// the engine binds a workflow state to at run time.
func TestConfigValidator_Workflow_UnknownPromptTaskIsAnError(t *testing.T) {
	cfg := workflowValidatorConfig(map[string]any{
		"route": map[string]any{
			"prompt_task": "does-not-exist",
			"terminal":    true,
		},
	})
	v := NewConfigValidatorWithPath(cfg, "")
	err := v.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}
