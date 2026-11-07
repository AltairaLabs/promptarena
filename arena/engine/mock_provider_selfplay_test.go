package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
)

// engineWithSelfPlay builds an engine whose self-play role "user" is served by
// the provider "user-sim", wired the way BuildEngineComponents wires it: a
// CompositeConversationExecutor whose executors share one self-play registry
// holding its own copy of the provider instance. It returns the self-play
// registry's provider registry so a test can see which instance a role uses.
func engineWithSelfPlay(t *testing.T, dir string) (*Engine, *providers.Registry) {
	t.Helper()
	cfg := &arenaconfig.Config{
		ConfigDir: dir,
		LoadedProviders: map[string]*config.Provider{
			"assistant": {ID: "assistant", Type: "openai", Model: "gpt-4o-mini"},
			"user-sim":  {ID: "user-sim", Type: "openai", Model: "gpt-4o-mini"},
		},
	}

	// Stand-ins for the real providers built at startup.
	realUser := mock.NewProvider("user-sim", "real-model", false)
	main := providers.NewRegistry()
	main.Register(mock.NewProvider("assistant", "real-model", false))
	main.Register(realUser)

	selfPlayProviders := providers.NewRegistry()
	selfPlayProviders.Register(realUser)
	reg := selfplay.NewRegistry(selfPlayProviders, map[string]string{"user": "user-sim"}, nil,
		[]arenaconfig.SelfPlayRoleGroup{{ID: "user", Provider: "user-sim"}})
	exec := NewCompositeConversationExecutor(
		&DefaultConversationExecutor{selfPlayRegistry: reg},
		&DuplexConversationExecutor{selfPlayRegistry: reg},
		nil,
	)

	eng, err := NewEngine(cfg, main, nil, nil, exec, nil, nil)
	require.NoError(t, err)
	return eng, selfPlayProviders
}

// TestEnableMockProviderMode_MocksSelfPlayRoles pins #446 item 2: --mock-provider
// swapped only the engine's provider registry, so a self-play role kept the real
// provider instance and called the real API (a 401 with no key).
func TestEnableMockProviderMode_MocksSelfPlayRoles(t *testing.T) {
	eng, selfPlayProviders := engineWithSelfPlay(t, t.TempDir())
	before, _ := selfPlayProviders.Get("user-sim")

	require.NoError(t, eng.EnableMockProviderMode(""))

	mocked, ok := eng.providerRegistry.Get("user-sim")
	require.True(t, ok)
	got, ok := selfPlayProviders.Get("user-sim")
	require.True(t, ok)
	assert.NotSame(t, before, got, "self-play role still holds the real provider")
	assert.Same(t, mocked, got, "self-play role must use the engine's mock")
	assert.Equal(t, genericMockResponse, predictWith(t, got))
}

// TestEnableMockProviderMode_SelfPlayHonorsMockConfigFlag checks that the
// self-play role takes the --mock-config responses too, as the assistant does.
func TestEnableMockProviderMode_SelfPlayHonorsMockConfigFlag(t *testing.T) {
	dir := t.TempDir()
	const flagResponse = "Response from the --mock-config flag"
	path := writeMockConfig(t, dir, flagResponse)
	eng, selfPlayProviders := engineWithSelfPlay(t, dir)

	require.NoError(t, eng.EnableMockProviderMode(path))

	got, ok := selfPlayProviders.Get("user-sim")
	require.True(t, ok)
	assert.Equal(t, flagResponse, predictWith(t, got))
}

func TestEngineSelfPlayRegistry(t *testing.T) {
	reg := selfplay.NewRegistry(providers.NewRegistry(), nil, nil, nil)

	assert.Nil(t, (&Engine{}).selfPlayRegistry())
	assert.Nil(t, (&Engine{conversationExecutor: &CompositeConversationExecutor{}}).selfPlayRegistry())
	assert.Same(t, reg, (&Engine{conversationExecutor: &DefaultConversationExecutor{selfPlayRegistry: reg}}).selfPlayRegistry())
	assert.Same(t, reg, (&Engine{conversationExecutor: NewCompositeConversationExecutor(
		&DefaultConversationExecutor{selfPlayRegistry: reg}, nil, nil)}).selfPlayRegistry())
}
