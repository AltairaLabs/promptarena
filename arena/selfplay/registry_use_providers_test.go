package selfplay

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
)

func TestRegistryUseProviders(t *testing.T) {
	realUser := mock.NewProvider("user-sim", "real", false)
	realJudge := mock.NewProvider("judge-sim", "real", false)
	own := providers.NewRegistry()
	own.Register(realUser)
	own.Register(realJudge)
	persona := &arenaconfig.UserPersonaPack{ID: "p"}
	reg := NewRegistry(own,
		map[string]string{"user": "user-sim", "judge": "judge-sim"},
		map[string]*arenaconfig.UserPersonaPack{"p": persona},
		[]arenaconfig.SelfPlayRoleGroup{{ID: "user", Provider: "user-sim"}, {ID: "judge", Provider: "judge-sim"}})

	_, err := reg.GetContentGenerator("user", "p")
	require.NoError(t, err)
	require.Equal(t, 1, reg.GetCacheStats().Size)

	reg.UseProviders(nil) // no-op
	require.Equal(t, 1, reg.GetCacheStats().Size)

	replacement := mock.NewProvider("user-sim", "mock", false)
	swap := providers.NewRegistry()
	swap.Register(replacement) // holds no judge-sim
	reg.UseProviders(swap)

	got, err := reg.getProviderForRole("user")
	require.NoError(t, err)
	assert.Same(t, replacement, got)
	kept, err := reg.getProviderForRole("judge")
	require.NoError(t, err)
	assert.Same(t, realJudge, kept, "a role the new registry does not cover keeps its provider")
	assert.Equal(t, 0, reg.GetCacheStats().Size, "generators built on the old provider must be dropped")
}
