package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
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

// TestBuildStreamRetryPolicy_TranslatesConfig covers buildStreamRetryPolicy,
// which sat at 15.4%. The retry window decides whether a mid-stream failure is
// retried after the model has already emitted tokens — retrying there
// duplicates output rather than recovering, so an unknown value must fall back
// to the conservative pre-first-chunk window, not to "always".
func TestBuildStreamRetryPolicy_TranslatesConfig(t *testing.T) {
	t.Run("nil config disables retries", func(t *testing.T) {
		got := buildStreamRetryPolicy("p", nil)
		assert.False(t, got.Enabled)
	})

	t.Run("disabled config disables retries", func(t *testing.T) {
		got := buildStreamRetryPolicy("p", &config.StreamRetryConfig{Enabled: false, MaxAttempts: 5})
		assert.False(t, got.Enabled)
		assert.Zero(t, got.MaxAttempts, "a disabled policy must not carry attempts")
	})

	t.Run("durations and attempts carry through", func(t *testing.T) {
		got := buildStreamRetryPolicy("p", &config.StreamRetryConfig{
			Enabled:      true,
			MaxAttempts:  3,
			InitialDelay: "250ms",
			MaxDelay:     "2s",
		})
		assert.True(t, got.Enabled)
		assert.Equal(t, 3, got.MaxAttempts)
		assert.Equal(t, 250*time.Millisecond, got.InitialDelay)
		assert.Equal(t, 2*time.Second, got.MaxDelay)
		assert.Equal(t, providers.StreamRetryWindowPreFirstChunk, got.Window,
			"an unset window must default to pre_first_chunk")
	})

	t.Run("always window is honored when asked for explicitly", func(t *testing.T) {
		got := buildStreamRetryPolicy("p", &config.StreamRetryConfig{
			Enabled:     true,
			RetryWindow: string(providers.StreamRetryWindowAlways),
		})
		assert.Equal(t, providers.StreamRetryWindowAlways, got.Window)
	})

	t.Run("an unknown window falls back to pre_first_chunk, not always", func(t *testing.T) {
		got := buildStreamRetryPolicy("p", &config.StreamRetryConfig{
			Enabled:     true,
			RetryWindow: "whenever-you-like",
		})
		assert.Equal(t, providers.StreamRetryWindowPreFirstChunk, got.Window)
	})

	t.Run("a malformed duration is ignored rather than fatal", func(t *testing.T) {
		got := buildStreamRetryPolicy("p", &config.StreamRetryConfig{
			Enabled:      true,
			InitialDelay: "not-a-duration",
		})
		assert.True(t, got.Enabled)
		assert.Zero(t, got.InitialDelay, "the provider default applies instead")
	})
}

// TestBuildStreamRetryBudget_NilMeansUnbounded covers buildStreamRetryBudget.
// nil is load-bearing: the caller reads it as "no budget", so returning a
// zero-rate budget instead would silently forbid every retry.
func TestBuildStreamRetryBudget_NilMeansUnbounded(t *testing.T) {
	assert.Nil(t, buildStreamRetryBudget(nil))
	assert.Nil(t, buildStreamRetryBudget(&config.StreamRetryConfig{Enabled: false,
		Budget: &config.StreamRetryBudgetConfig{RatePerSec: 1, Burst: 1}}))
	assert.Nil(t, buildStreamRetryBudget(&config.StreamRetryConfig{Enabled: true}),
		"enabled but with no budget block is still unbounded")

	got := buildStreamRetryBudget(&config.StreamRetryConfig{
		Enabled: true,
		Budget:  &config.StreamRetryBudgetConfig{RatePerSec: 2, Burst: 5},
	})
	assert.NotNil(t, got)
}

// TestBuildHTTPTransportOptions_ZeroValueMeansDefaults covers
// buildHTTPTransportOptions. The zero value is what tells the runtime to use
// its own defaults, so a nil config must not be turned into explicit zeros
// that would cap connections at zero.
func TestBuildHTTPTransportOptions_ZeroValueMeansDefaults(t *testing.T) {
	assert.Equal(t, providers.HTTPTransportOptions{}, buildHTTPTransportOptions("p", nil))

	got := buildHTTPTransportOptions("p", &config.HTTPTransportConfig{
		MaxConnsPerHost:     16,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     "90s",
	})
	assert.Equal(t, 16, got.MaxConnsPerHost)
	assert.Equal(t, 8, got.MaxIdleConnsPerHost)
	assert.Equal(t, 90*time.Second, got.IdleConnTimeout)

	bad := buildHTTPTransportOptions("p", &config.HTTPTransportConfig{IdleConnTimeout: "-5s"})
	assert.Zero(t, bad.IdleConnTimeout, "a non-positive timeout falls back to the default")
}

// TestBuildStateStore_SelectsBackend covers buildStateStore, which sat at
// 37.5%. Choosing the wrong backend is not a visible failure — a run simply
// records its telemetry somewhere nobody looks — so the type switch and its
// rejections are worth pinning.
func TestBuildStateStore_SelectsBackend(t *testing.T) {
	t.Run("no config defaults to the in-memory arena store", func(t *testing.T) {
		got, err := buildStateStore(&arenaconfig.Config{})
		require.NoError(t, err)
		assert.NotNil(t, got)
	})

	t.Run("an empty type defaults the same way", func(t *testing.T) {
		got, err := buildStateStore(&arenaconfig.Config{
			StateStore: &config.StateStoreConfig{},
		})
		require.NoError(t, err)
		assert.NotNil(t, got)
	})

	t.Run("redis without a redis block is rejected, not silently defaulted", func(t *testing.T) {
		_, err := buildStateStore(&arenaconfig.Config{
			StateStore: &config.StateStoreConfig{Type: "redis"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "redis configuration is required")
	})

	t.Run("redis is constructed without dialing", func(t *testing.T) {
		got, err := buildStateStore(&arenaconfig.Config{
			StateStore: &config.StateStoreConfig{
				Type: "redis",
				Redis: &config.RedisConfig{
					Address: "127.0.0.1:6379", TTL: "30m", Prefix: "arena:",
				},
			},
		})
		require.NoError(t, err)
		assert.NotNil(t, got)
	})

	t.Run("a malformed TTL fails the build rather than being ignored", func(t *testing.T) {
		_, err := buildStateStore(&arenaconfig.Config{
			StateStore: &config.StateStoreConfig{
				Type:  "redis",
				Redis: &config.RedisConfig{Address: "127.0.0.1:6379", TTL: "half an hour"},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid TTL duration")
	})

	t.Run("an unknown type is rejected", func(t *testing.T) {
		_, err := buildStateStore(&arenaconfig.Config{
			StateStore: &config.StateStoreConfig{Type: "postgres"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported state store type")
	})
}
