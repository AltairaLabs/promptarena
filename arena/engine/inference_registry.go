package engine

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	_ "github.com/AltairaLabs/promptarena/v2/arena/inferencemock" // registers the keyless "mock" inference type

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/credentials"
	"github.com/AltairaLabs/PromptKit/runtime/v2/inference"
	_ "github.com/AltairaLabs/PromptKit/runtime/v2/inference/all" // registers inference backend factories via init()
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/base"
)

// buildInferenceRegistry builds the inference providers every classifier-backed
// check resolves (topic_policy, audio_emotion, image_moderation, the text
// classifiers): one per `providers:` entry with role: inference. Those checks
// ask for an inference.Provider, through the binding when they name a key and
// through the registry's default when they name none.
//
// It mirrors the SDK's host side. The default is the provider a
// defaults.inference entry names, else the first by id. A provider that cannot
// be built is skipped and reported in the returned error rather than failing
// the rest, so one bad entry does not disable every other classifier; a check
// that names the skipped one then reports it unbound. Returns (nil, nil) when
// no inference providers are configured.
func buildInferenceRegistry(cfg *arenaconfig.Config) (*inference.Registry, error) {
	if cfg == nil || len(cfg.LoadedInferenceProviders) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(cfg.LoadedInferenceProviders))
	for id := range cfg.LoadedInferenceProviders {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	reg := inference.NewRegistry()
	var errs []error
	for _, id := range ids {
		p, err := buildInferenceProvider(cfg.ConfigDir, id, cfg.LoadedInferenceProviders[id])
		if err != nil {
			errs = append(errs, fmt.Errorf("inference provider %s: %w", id, err))
			continue
		}
		if err := reg.Register(id, p); err != nil {
			errs = append(errs, err)
		}
	}
	if def := defaultInferenceID(cfg.Defaults.Inference); def != "" {
		if err := reg.SetDefault(def); err != nil {
			errs = append(errs, fmt.Errorf("defaults.inference: %w", err))
		}
	}
	if len(reg.IDs()) == 0 {
		return nil, errors.Join(errs...)
	}
	return reg, errors.Join(errs...)
}

// buildInferenceProvider resolves the provider's credential and constructs it
// for its type, instrumented so every call reports inference metrics.
func buildInferenceProvider(configDir, id string, p *config.Provider) (inference.Provider, error) {
	if p == nil {
		return nil, errors.New("no provider definition")
	}
	var cred credentials.Credential
	if p.Credential != nil || envKeyAllowedFor(p.Type, p.BaseURL) {
		resolved, err := inference.ResolveCredential(context.Background(), p.Type, configDir, p.Credential)
		if err != nil {
			return nil, fmt.Errorf("resolving credential: %w", err)
		}
		cred = resolved
	}
	// A vendor endpoint without a key would fail every call, and a check that
	// names no provider would then error instead of skipping. Refusing it here
	// keeps a kit that declares, say, a Hugging Face classifier runnable
	// without HF_TOKEN. A self-hosted endpoint (base_url set) may need no key.
	if _, vendor := vendorKeyHosts[p.Type]; vendor && strings.TrimSpace(p.BaseURL) == "" &&
		base.APIKeyFromCredential(cred) == "" {
		return nil, errors.New("no api key configured (set credential.api_key or credential.credential_env, " +
			"or export the provider's default key variable such as OPENAI_API_KEY or HF_TOKEN)")
	}
	// headers, request_timeout, http_transport and rate_limit apply to an
	// inference provider as to any capability provider.
	tuning, err := config.CapabilityHTTPTuning(p)
	if err != nil {
		return nil, err
	}
	built, err := inference.CreateFromSpec(inference.ProviderSpec{
		ID:               id,
		Type:             p.Type,
		Model:            p.Model,
		BaseURL:          p.BaseURL,
		Credential:       cred,
		AdditionalConfig: p.AdditionalConfig,
		Tuning:           tuning,
	})
	if err != nil {
		return nil, err
	}
	return inference.Instrument(built, id, p.Type), nil
}

// defaultInferenceID picks the provider a check that names none resolves to.
// The registry holds one default, so the first per-task default set wins.
func defaultInferenceID(d *config.InferenceDefaults) string {
	if d == nil {
		return ""
	}
	for _, id := range []string{d.TextClassifier, d.AudioClassifier, d.ImageClassifier, d.VideoClassifier, d.Embedder} {
		if id != "" {
			return id
		}
	}
	return ""
}

// vendorKeyHosts lists, for types whose default key env var belongs to one
// vendor, the hosts that key may be sent to. Kept in step with the SDK.
var vendorKeyHosts = map[string][]string{
	"openai":      {"api.openai.com"},
	"huggingface": {"huggingface.co", "huggingface.cloud"},
}

// envKeyAllowedFor reports whether a type's default key env var may be used for
// baseURL: always for the vendor's own endpoint (an empty base_url means the
// default), never for another host. A gateway or self-hosted classifier gets an
// explicit credential or none, so OPENAI_API_KEY is never sent to a local NIM.
func envKeyAllowedFor(providerType, baseURL string) bool {
	hosts, ok := vendorKeyHosts[providerType]
	if !ok || strings.TrimSpace(baseURL) == "" {
		return true
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range hosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}
