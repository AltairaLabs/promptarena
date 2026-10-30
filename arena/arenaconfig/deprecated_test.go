package arenaconfig

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDeprecatedFields_UntypedValuesFillTypedFields(t *testing.T) {
	cfg := &Config{
		Workflow: map[string]any{
			"version": 1,
			"entry":   "start",
			"states":  map[string]any{"start": map[string]any{"prompt_task": "start", "terminal": true}},
		},
		Compositions: map[string]any{
			"flow": map[string]any{
				"version": 1,
				"steps":   []any{map[string]any{"id": "s1", "kind": "tool", "tool": "echo"}},
			},
		},
	}
	require.NoError(t, cfg.ResolveDeprecatedFields())
	require.NotNil(t, cfg.WorkflowSpec)
	assert.Equal(t, "start", cfg.WorkflowSpec.Entry)
	require.Contains(t, cfg.CompositionSpecs, "flow")
	assert.Len(t, cfg.CompositionSpecs["flow"].Steps, 1)
}

func TestResolveDeprecatedFields_TypedFieldsWin(t *testing.T) {
	typed := &workflow.Spec{Version: 1, Entry: "typed"}
	cfg := &Config{WorkflowSpec: typed, Workflow: map[string]any{"entry": "untyped"}}
	require.NoError(t, cfg.ResolveDeprecatedFields())
	assert.Same(t, typed, cfg.WorkflowSpec)
}

func TestResolveDeprecatedFields_RejectsUnparseableValues(t *testing.T) {
	assert.Error(t, (&Config{Workflow: "not a workflow"}).ResolveDeprecatedFields())
	assert.Error(t, (&Config{Compositions: "not compositions"}).ResolveDeprecatedFields())
	assert.NoError(t, (&Config{}).ResolveDeprecatedFields())
}
