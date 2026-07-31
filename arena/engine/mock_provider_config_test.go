package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

// declaredMockResponse is the reply the fixture below serves. It is deliberately
// nothing like genericMockResponse, so a test can tell which repository actually
// backed the provider.
const declaredMockResponse = "Response from the provider's own mock_config"

// writeMockConfig writes a mock-responses fixture and returns its path.
func writeMockConfig(t *testing.T, dir, response string) string {
	t.Helper()
	path := filepath.Join(dir, "mock-responses.yaml")
	content := "defaultResponse: \"" + response + "\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write mock config: %v", err)
	}
	return path
}

// engineWithMockProvider builds an engine holding a single mock-type provider
// whose additional_config carries the given mock_config value (omitted when
// empty). configDir is what a relative mock_config resolves against.
func engineWithMockProvider(t *testing.T, configDir, mockConfig string) *Engine {
	t.Helper()

	additional := map[string]interface{}{}
	if mockConfig != "" {
		additional["mock_config"] = mockConfig
	}

	cfg := &arenaconfig.Config{
		ConfigDir: configDir,
		LoadedProviders: map[string]*config.Provider{
			"declared-mock": {
				ID:               "declared-mock",
				Type:             "mock",
				Model:            "mock-model",
				AdditionalConfig: additional,
			},
		},
	}

	registry := providers.NewRegistry()
	registry.Register(mock.NewProvider("declared-mock", "mock-model", false))

	eng, err := NewEngine(cfg, registry, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	return eng
}

// predictOnce drives the registered provider and returns its reply text.
func predictOnce(t *testing.T, eng *Engine, providerID string) string {
	t.Helper()
	prov, ok := eng.providerRegistry.Get(providerID)
	if !ok {
		t.Fatalf("provider %q not found after enabling mock mode", providerID)
	}
	resp, err := prov.Predict(context.Background(), providers.PredictionRequest{
		Messages:  []types.Message{{Role: "user", Content: "hello"}},
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}
	return resp.Content
}

// TestEnableMockProviderMode_HonorsProviderDeclaredConfig pins the fix for #80.
//
// --mock-provider used to substitute a generic repository for every provider and
// throw away each one's declared additional_config.mock_config, with no warning.
// A provider that was already a mock, with responses on disk, got replaced by one
// that ignored them — so every content-dependent assertion silently inverted. On
// examples/guardrails-test that made the guardrail suite report scenarios as
// passing when nothing had been checked at all.
//
// Asserting on the reply text rather than on provider identity is the point: the
// pre-fix code registered a provider under the right ID too, which is why the
// existing tests here could not catch this.
func TestEnableMockProviderMode_HonorsProviderDeclaredConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeMockConfig(t, dir, declaredMockResponse)

	eng := engineWithMockProvider(t, dir, path)
	if err := eng.EnableMockProviderMode(""); err != nil {
		t.Fatalf("EnableMockProviderMode failed: %v", err)
	}

	if got := predictOnce(t, eng, "declared-mock"); got != declaredMockResponse {
		t.Errorf("provider's declared mock_config was discarded:\n got: %q\nwant: %q", got, declaredMockResponse)
	}
}

// TestEnableMockProviderMode_RelativeConfigResolvesAgainstConfigDir covers the
// path a real arena config takes: mock_config is written relative to the config
// directory. Only providers that were actually constructed get their path
// rewritten to absolute by createProviderImpl, so a provider skipped by a
// --provider filter still reaches EnableMockProviderMode with a relative path.
// Resolving it here rather than relying on the process working directory is what
// makes that case work.
func TestEnableMockProviderMode_RelativeConfigResolvesAgainstConfigDir(t *testing.T) {
	dir := t.TempDir()
	writeMockConfig(t, dir, declaredMockResponse)

	eng := engineWithMockProvider(t, dir, "mock-responses.yaml")
	if err := eng.EnableMockProviderMode(""); err != nil {
		t.Fatalf("EnableMockProviderMode failed: %v", err)
	}

	if got := predictOnce(t, eng, "declared-mock"); got != declaredMockResponse {
		t.Errorf("relative mock_config was not resolved against ConfigDir:\n got: %q\nwant: %q",
			got, declaredMockResponse)
	}
}

// TestEnableMockProviderMode_FlagOverridesDeclaredConfig pins the precedence:
// an explicit --mock-config is an operator override and wins over what any
// provider declares. Without this the fix could satisfy the test above by simply
// always preferring the provider's file, which would silently ignore the flag.
func TestEnableMockProviderMode_FlagOverridesDeclaredConfig(t *testing.T) {
	dir := t.TempDir()
	writeMockConfig(t, dir, declaredMockResponse)

	overrideDir := t.TempDir()
	const overrideResponse = "Response from the --mock-config flag"
	overridePath := writeMockConfig(t, overrideDir, overrideResponse)

	eng := engineWithMockProvider(t, dir, filepath.Join(dir, "mock-responses.yaml"))
	if err := eng.EnableMockProviderMode(overridePath); err != nil {
		t.Fatalf("EnableMockProviderMode failed: %v", err)
	}

	if got := predictOnce(t, eng, "declared-mock"); got != overrideResponse {
		t.Errorf("--mock-config must override a provider's declaration:\n got: %q\nwant: %q",
			got, overrideResponse)
	}
}

// TestEnableMockProviderMode_NoConfigAnywhereUsesGeneric keeps the original
// behavior for a provider that declares nothing: the generic in-memory reply.
// Without it, a fix that errored or fell back to something else when no
// mock_config exists would go unnoticed.
func TestEnableMockProviderMode_NoConfigAnywhereUsesGeneric(t *testing.T) {
	eng := engineWithMockProvider(t, t.TempDir(), "")
	if err := eng.EnableMockProviderMode(""); err != nil {
		t.Fatalf("EnableMockProviderMode failed: %v", err)
	}

	if got := predictOnce(t, eng, "declared-mock"); got != genericMockResponse {
		t.Errorf("expected the generic response with no mock_config anywhere:\n got: %q\nwant: %q",
			got, genericMockResponse)
	}
}
