package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

func TestBuildProviderFilterSet(t *testing.T) {
	t.Run("empty filter returns empty set (load all)", func(t *testing.T) {
		cfg := &arenaconfig.Config{}
		got := buildProviderFilterSet(cfg, nil)
		assert.Empty(t, got)
	})

	t.Run("empty filter ignores selfplay roles", func(t *testing.T) {
		cfg := &arenaconfig.Config{
			SelfPlay: &arenaconfig.SelfPlayConfig{
				Roles: []arenaconfig.SelfPlayRoleGroup{{ID: "u", Provider: "sp"}},
			},
		}
		got := buildProviderFilterSet(cfg, nil)
		assert.Empty(t, got, "empty filter means load all; selfplay must not narrow it")
	})

	t.Run("active filter includes selfplay role providers", func(t *testing.T) {
		cfg := &arenaconfig.Config{
			SelfPlay: &arenaconfig.SelfPlayConfig{
				Roles: []arenaconfig.SelfPlayRoleGroup{
					{ID: "u", Provider: "sp"},
					{ID: "blank", Provider: ""},
				},
			},
		}
		got := buildProviderFilterSet(cfg, []string{"mock"})
		assert.True(t, got["mock"])
		assert.True(t, got["sp"], "selfplay providers must be loadable even outside the filter")
		assert.NotContains(t, got, "", "blank selfplay providers are skipped")
	})

	t.Run("active filter with no selfplay leaves filter intact", func(t *testing.T) {
		cfg := &arenaconfig.Config{}
		got := buildProviderFilterSet(cfg, []string{"a", "b"})
		assert.True(t, got["a"])
		assert.True(t, got["b"])
		assert.Len(t, got, 2)
	})
}

func TestConfigureOrchestratorMetadata(t *testing.T) {
	t.Run("nil orchestrator is a no-op", func(t *testing.T) {
		assert.NotPanics(t, func() {
			configureOrchestratorMetadata(nil, &arenaconfig.Config{}, nil, nil)
		})
	})

	t.Run("sets metadata and classify registry", func(t *testing.T) {
		orch := &EvalOrchestrator{}
		configureOrchestratorMetadata(orch, &arenaconfig.Config{}, nil, nil)
		assert.NotNil(t, orch.metadata)
	})
}
