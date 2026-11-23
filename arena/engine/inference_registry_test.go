package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/inference"
)

// hfProvider returns a Hugging Face inference provider with a literal key, the
// cleanest seam: env-var resolution is the credentials package's to test.
func hfProvider(id string) *config.Provider {
	return &config.Provider{
		ID:         id,
		Type:       "huggingface",
		Role:       config.RoleInference,
		Credential: &config.CredentialConfig{APIKey: "test-token"},
	}
}

func inferenceCfg(providers ...*config.Provider) *arenaconfig.Config {
	m := make(map[string]*config.Provider, len(providers))
	for _, p := range providers {
		m[p.ID] = p
	}
	return &arenaconfig.Config{LoadedInferenceProviders: m}
}

// Every type PromptKit can call a topic classifier with must build: before
// this, nvidia-topic-control built only as a classify backend (which the
// checks cannot use) and openai/systemone did not build at all.
func TestBuildInferenceRegistry_BuildsEveryTopicClassifierType(t *testing.T) {
	cfg := inferenceCfg(
		&config.Provider{ID: "nim", Type: "nvidia-topic-control", Role: config.RoleInference,
			BaseURL: "http://topic-control:8000/v1"},
		&config.Provider{ID: "oai", Type: "openai", Role: config.RoleInference, Model: "gpt-4o-mini",
			Credential: &config.CredentialConfig{APIKey: "sk-test"}},
		hfProvider("hf"),
	)
	reg, err := buildInferenceRegistry(cfg)
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.Equal(t, []string{"hf", "nim", "oai"}, reg.IDs())
}

func TestBuildInferenceRegistry_DefaultIsFirstByIDUnlessConfigured(t *testing.T) {
	cfg := inferenceCfg(hfProvider("b-hf"), hfProvider("a-hf"))
	reg, err := buildInferenceRegistry(cfg)
	require.NoError(t, err)
	first, err := reg.Get("a-hf")
	require.NoError(t, err)
	def, err := reg.Get("")
	require.NoError(t, err)
	assert.Same(t, first, def, "with no defaults.inference the first id is the default")

	cfg.Defaults.Inference = &config.InferenceDefaults{TextClassifier: "b-hf"}
	reg, err = buildInferenceRegistry(cfg)
	require.NoError(t, err)
	named, err := reg.Get("b-hf")
	require.NoError(t, err)
	def, err = reg.Get("")
	require.NoError(t, err)
	assert.Same(t, named, def, "defaults.inference names the default")
}

func TestBuildInferenceRegistry_UnknownDefaultIsReported(t *testing.T) {
	cfg := inferenceCfg(hfProvider("hf"))
	cfg.Defaults.Inference = &config.InferenceDefaults{AudioClassifier: "doesnotexist"}

	reg, err := buildInferenceRegistry(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "doesnotexist")
	require.NotNil(t, reg, "the providers that built stay usable")
}

// One unusable entry must not take every other classifier down with it, which
// is what failing the whole registry used to do.
func TestBuildInferenceRegistry_BadEntryIsSkippedNotFatal(t *testing.T) {
	cfg := inferenceCfg(hfProvider("hf"),
		&config.Provider{ID: "x", Type: "bogus", Role: config.RoleInference,
			Credential: &config.CredentialConfig{APIKey: "k"}})

	reg, err := buildInferenceRegistry(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inference provider x")
	require.NotNil(t, reg)
	assert.Equal(t, []string{"hf"}, reg.IDs())
}

// A vendor endpoint with no key would fail every call; refusing it keeps a
// check that names no provider skipping, so demos run without HF_TOKEN.
func TestBuildInferenceRegistry_VendorEndpointWithoutKeyIsRefused(t *testing.T) {
	t.Setenv("HF_TOKEN", "")
	t.Setenv("HUGGING_FACE_HUB_TOKEN", "")
	cfg := inferenceCfg(&config.Provider{ID: "hf", Type: "huggingface", Role: config.RoleInference})

	reg, err := buildInferenceRegistry(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no api key")
	assert.Nil(t, reg)
}

func TestBuildInferenceRegistry_VendorKeyFromEnv(t *testing.T) {
	t.Setenv("HF_TOKEN", "from-env")
	cfg := inferenceCfg(&config.Provider{ID: "hf", Type: "huggingface", Role: config.RoleInference})

	reg, err := buildInferenceRegistry(cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"hf"}, reg.IDs())
}

// The vendor's env key must never be sent to another host: a self-hosted
// classifier at a custom base_url gets no Authorization header from
// OPENAI_API_KEY.
func TestBuildInferenceRegistry_VendorEnvKeyNotSentToAnotherHost(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-must-not-leak")
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "on"},
				"logprobs": map[string]any{"content": []map[string]any{{
					"token": "on", "logprob": -0.01,
					"top_logprobs": []map[string]any{{"token": "on", "logprob": -0.01}, {"token": "off", "logprob": -4.6}},
				}}},
			}},
		})
	}))
	defer srv.Close()

	cfg := inferenceCfg(&config.Provider{ID: "topic", Type: "openai", Role: config.RoleInference,
		Model: "local", BaseURL: srv.URL})
	reg, err := buildInferenceRegistry(cfg)
	require.NoError(t, err)
	p, err := reg.Get("topic")
	require.NoError(t, err)

	resp, err := p.Infer(context.Background(), inference.Request{
		Prompt: "Is this on topic?", Labels: []string{"on-topic", "off-topic"},
	})
	require.NoError(t, err)
	onTopic, _ := resp.Score("on-topic")
	assert.Greater(t, onTopic, 0.9, "the provider must have read the server's answer")
	assert.NotContains(t, gotAuth, "sk-must-not-leak")
}

// request_timeout and the other HTTP tuning fields reach an inference provider,
// and a completion-only field on one is an error rather than silently ignored.
func TestBuildInferenceRegistry_AppliesCapabilityTuning(t *testing.T) {
	bad := &config.Provider{ID: "nim", Type: "nvidia-topic-control", Role: config.RoleInference,
		BaseURL: "http://nim:8000/v1", StreamIdleTimeout: "90s"}
	reg, err := buildInferenceRegistry(inferenceCfg(bad))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream_idle_timeout")
	assert.Nil(t, reg)

	// additional_config.timeout_seconds plus request_timeout is a conflict the
	// backend rejects; it can only see request_timeout if the tuning reached it.
	both := &config.Provider{ID: "nim", Type: "nvidia-topic-control", Role: config.RoleInference,
		BaseURL: "http://nim:8000/v1", RequestTimeout: "5s",
		AdditionalConfig: map[string]any{"timeout_seconds": 10}}
	_, err = buildInferenceRegistry(inferenceCfg(both))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "either additional_config.timeout_seconds or request_timeout")
}

func TestEnvKeyAllowedFor(t *testing.T) {
	cases := []struct {
		typ, baseURL string
		want         bool
	}{
		{"openai", "", true},
		{"openai", "https://api.openai.com/v1", true},
		{"openai", "http://localhost:8000/v1", false},
		{"huggingface", "https://x.endpoints.huggingface.cloud", true},
		{"huggingface", "https://evil.example/huggingface.cloud", false},
		{"nvidia-topic-control", "http://nim:8000/v1", true},
		{"openai", "://bad", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, envKeyAllowedFor(c.typ, c.baseURL), "%s %s", c.typ, c.baseURL)
	}
}
