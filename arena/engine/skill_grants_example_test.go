package engine

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/skills"
	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

// The grant ceiling has to come from the tools the runtime can actually
// execute, and arena configs declare those under `tools:` — not in a compiled
// pack. LoadedPack is only populated from an explicit `pack:` file, so deriving
// the ceiling from it alone leaves skill grants inert for every arena-native
// config, which is all of them in examples/.
//
// This drives the real loader over the real example rather than a hand-built
// config, because that is where the gap hides: every hand-built config in these
// tests sets PackTools explicitly and therefore cannot catch it.
func TestSkillGrants_CeilingCoversArenaDeclaredTools(t *testing.T) {
	cfgPath := filepath.Join("..", "..", "examples", "workflow-skills", "config.arena.yaml")

	cfg, err := arenaconfig.LoadConfig(cfgPath)
	require.NoError(t, err)
	require.NotEmpty(t, cfg.LoadedSkillSources, "the example must declare skill sources")

	// The example declares its tools inline, not via a compiled pack.
	require.Nil(t, cfg.LoadedPack, "example has no pack file — this is the shape that was broken")
	require.NotEmpty(t, cfg.LoadedTools, "the example must declare tools")

	toolRegistry, err := buildToolRegistry(cfg)
	require.NoError(t, err)
	require.Contains(t, toolRegistry.GetTools(), "refund",
		"the example declares a refund tool")

	factory, _, err := discoverAndRegisterSkillTools(cfg, toolRegistry)
	require.NoError(t, err)
	require.NotNil(t, factory)

	// refund-processing declares allowed-tools: [refund]. Activating it in a
	// run must grant that tool.
	set := skills.NewActiveSet()
	activation, activateErr := factory.executor.ActivateIn(set, "refund-processing")
	require.NoError(t, activateErr)

	assert.Equal(t, []string{"refund"}, activation.AddedTools,
		"activating refund-processing must grant the refund tool")
	assert.Equal(t, []string{"refund"}, factory.executor.ToolsFor(set))
}

// A skill must never grant a tool the runtime cannot execute, however the
// ceiling is assembled.
func TestSkillGrants_CeilingExcludesUndeclaredTools(t *testing.T) {
	cfgPath := filepath.Join("..", "..", "examples", "workflow-skills", "config.arena.yaml")

	cfg, err := arenaconfig.LoadConfig(cfgPath)
	require.NoError(t, err)

	toolRegistry, err := buildToolRegistry(cfg)
	require.NoError(t, err)

	factory, _, err := discoverAndRegisterSkillTools(cfg, toolRegistry)
	require.NoError(t, err)
	granted := factory.executor.ToolsFor(skills.NewActiveSet())
	registered := toolRegistry.GetTools()
	for _, name := range granted {
		assert.Contains(t, registered, name,
			"granted tool %q is not registered, so the model could never call it", name)
	}

	assert.NotContains(t, allGrantableNames(t, factory), "definitely_not_a_real_tool")
}

// allGrantableNames activates every discovered skill in a throwaway executor
// and returns the union of what they granted.
func allGrantableNames(t *testing.T, factory *SkillsFactory) []string {
	t.Helper()
	set := skills.NewActiveSet()
	for _, meta := range factory.catalog.List() {
		_, _ = factory.executor.ActivateIn(set, meta.Name)
	}
	return factory.executor.ToolsFor(set)
}
