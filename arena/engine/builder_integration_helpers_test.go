package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
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

// tunedSpec builds the provider spec Arena hands the runtime factory for p.
// The tuning fields now come from PromptKit's config.ApplyProviderTuning; the
// tests below pin its behaviour as Arena relies on it.
func tunedSpec(p *config.Provider) providers.ProviderSpec {
	return buildProviderSpec(p, nil, "", nil)
}

// TestBuildProviderSpec_StreamRetryPolicy covers stream_retry. The retry
// window decides whether a mid-stream failure is retried after the model has
// already emitted tokens — retrying there duplicates output rather than
// recovering, so an unknown value must fall back to the conservative
// pre-first-chunk window, not to "always".
func TestBuildProviderSpec_StreamRetryPolicy(t *testing.T) {
	retry := func(cfg *config.StreamRetryConfig) providers.StreamRetryPolicy {
		return tunedSpec(&config.Provider{ID: "p", StreamRetry: cfg}).StreamRetry
	}

	t.Run("nil config disables retries", func(t *testing.T) {
		got := retry(nil)
		assert.False(t, got.Enabled)
	})

	t.Run("disabled config disables retries", func(t *testing.T) {
		got := retry(&config.StreamRetryConfig{Enabled: false, MaxAttempts: 5})
		assert.False(t, got.Enabled)
		assert.Zero(t, got.MaxAttempts, "a disabled policy must not carry attempts")
	})

	t.Run("durations and attempts carry through", func(t *testing.T) {
		got := retry(&config.StreamRetryConfig{
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
		got := retry(&config.StreamRetryConfig{
			Enabled:     true,
			RetryWindow: string(providers.StreamRetryWindowAlways),
		})
		assert.Equal(t, providers.StreamRetryWindowAlways, got.Window)
	})

	t.Run("an unknown window falls back to pre_first_chunk, not always", func(t *testing.T) {
		got := retry(&config.StreamRetryConfig{
			Enabled:     true,
			RetryWindow: "whenever-you-like",
		})
		assert.Equal(t, providers.StreamRetryWindowPreFirstChunk, got.Window)
	})

	t.Run("a malformed duration is ignored rather than fatal", func(t *testing.T) {
		got := retry(&config.StreamRetryConfig{
			Enabled:      true,
			InitialDelay: "not-a-duration",
		})
		assert.True(t, got.Enabled)
		assert.Zero(t, got.InitialDelay, "the provider default applies instead")
	})
}

// TestBuildProviderSpec_StreamRetryBudgetNilMeansUnbounded covers the retry
// budget. nil is load-bearing: the runtime reads it as "no budget", so a
// zero-rate budget instead would silently forbid every retry.
func TestBuildProviderSpec_StreamRetryBudgetNilMeansUnbounded(t *testing.T) {
	budget := func(cfg *config.StreamRetryConfig) *providers.RetryBudget {
		return tunedSpec(&config.Provider{ID: "p", StreamRetry: cfg}).StreamRetryBudget
	}

	assert.Nil(t, budget(nil))
	assert.Nil(t, budget(&config.StreamRetryConfig{Enabled: false,
		Budget: &config.StreamRetryBudgetConfig{RatePerSec: 1, Burst: 1}}))
	assert.Nil(t, budget(&config.StreamRetryConfig{Enabled: true}),
		"enabled but with no budget block is still unbounded")

	got := budget(&config.StreamRetryConfig{
		Enabled: true,
		Budget:  &config.StreamRetryBudgetConfig{RatePerSec: 2, Burst: 5},
	})
	assert.NotNil(t, got)
}

// TestBuildProviderSpec_HTTPTransportZeroValueMeansDefaults covers
// http_transport. The zero value is what tells the runtime to use its own
// defaults, so a nil config must not be turned into explicit zeros that would
// cap connections at zero.
func TestBuildProviderSpec_HTTPTransportZeroValueMeansDefaults(t *testing.T) {
	transport := func(cfg *config.HTTPTransportConfig) providers.HTTPTransportOptions {
		return tunedSpec(&config.Provider{ID: "p", HTTPTransport: cfg}).HTTPTransport
	}

	assert.Equal(t, providers.HTTPTransportOptions{}, transport(nil))

	got := transport(&config.HTTPTransportConfig{
		MaxConnsPerHost:     16,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     "90s",
	})
	assert.Equal(t, 16, got.MaxConnsPerHost)
	assert.Equal(t, 8, got.MaxIdleConnsPerHost)
	assert.Equal(t, 90*time.Second, got.IdleConnTimeout)

	bad := transport(&config.HTTPTransportConfig{IdleConnTimeout: "-5s"})
	assert.Zero(t, bad.IdleConnTimeout, "a non-positive timeout falls back to the default")
}

// TestBuildProviderSpec_CarriesHeaders pins that a provider file's custom
// headers reach the runtime. Arena used to drop them while the SDK honored
// them, so the same provider file sent different requests in each.
func TestBuildProviderSpec_CarriesHeaders(t *testing.T) {
	headers := map[string]string{"X-Tenant": "acme", "OpenAI-Organization": "org-1"}
	got := tunedSpec(&config.Provider{ID: "p", Headers: headers})
	assert.Equal(t, headers, got.Headers)

	assert.Nil(t, tunedSpec(&config.Provider{ID: "p"}).Headers)
}

// TestBuildProviderSpec_TuningAndDefaults covers the remaining tuning fields
// and checks the non-tuning defaults survive alongside them.
func TestBuildProviderSpec_TuningAndDefaults(t *testing.T) {
	off := false
	got := tunedSpec(&config.Provider{
		ID:                  "p",
		RequestTimeout:      "45s",
		StreamIdleTimeout:   "10s",
		StreamMaxConcurrent: 4,
		Defaults: config.ProviderDefaults{
			Temperature:   0.2,
			TopP:          0.9,
			MaxTokens:     256,
			PromptCaching: &off,
		},
		Pricing: config.Pricing{InputCostPer1K: 0.5, OutputCostPer1K: 1.5},
	})

	assert.Equal(t, 45*time.Second, got.RequestTimeout)
	assert.Equal(t, 10*time.Second, got.StreamIdleTimeout)
	assert.Equal(t, 4, got.StreamMaxConcurrent)
	assert.True(t, got.Defaults.DisablePromptCaching)
	assert.InDelta(t, 0.2, got.Defaults.Temperature, 1e-6)
	assert.InDelta(t, 0.9, got.Defaults.TopP, 1e-6)
	assert.Equal(t, 256, got.Defaults.MaxTokens)
	assert.InDelta(t, 0.5, got.Defaults.Pricing.InputCostPer1K, 1e-9)
	assert.InDelta(t, 1.5, got.Defaults.Pricing.OutputCostPer1K, 1e-9)

	assert.False(t, tunedSpec(&config.Provider{ID: "p"}).Defaults.DisablePromptCaching,
		"prompt caching stays on unless explicitly disabled")
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
