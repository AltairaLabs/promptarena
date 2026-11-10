package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
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
	return predictWith(t, prov)
}

// predictWith drives prov once and returns its reply text.
func predictWith(t *testing.T, prov providers.Provider) string {
	t.Helper()
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

// TestEnableMockProviderMode_RelativeConfigDirResolvesOnce covers a relative
// --config (`promptarena run examples/x/config.arena.yaml`): ConfigDir is then
// relative too, and the provider was already constructed, so createProviderImpl
// has rewritten mock_config to ConfigDir/mock-responses.yaml. Joining ConfigDir
// again gave examples/x/examples/x/mock-responses.yaml (#432 item 1).
func TestEnableMockProviderMode_RelativeConfigDirResolvesOnce(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	configDir := filepath.Join("kits", "demo")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeMockConfig(t, configDir, declaredMockResponse)

	eng := engineWithMockProvider(t, configDir, "mock-responses.yaml")
	if _, err := createProviderImpl(configDir, eng.providers["declared-mock"]); err != nil {
		t.Fatalf("createProviderImpl failed: %v", err)
	}
	if err := eng.EnableMockProviderMode(""); err != nil {
		t.Fatalf("EnableMockProviderMode failed: %v", err)
	}

	if got := predictOnce(t, eng, "declared-mock"); got != declaredMockResponse {
		t.Errorf("relative mock_config was resolved wrongly:\n got: %q\nwant: %q", got, declaredMockResponse)
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

// TestEnableMockProviderMode_DuplexSessionsReply pins #510.
//
// The --mock-provider replacement was built from a repository alone, so its
// duplex sessions never replied and every voice turn waited out the 10-minute
// turn deadline (examples/voice-refund-demo took hours). That hit both a
// declared mock-duplex provider, whose auto_respond was dropped, and the
// stand-ins for real realtime providers, which declare nothing.
func TestEnableMockProviderMode_DuplexSessionsReply(t *testing.T) {
	const declaredReply = "Thanks for calling. How can I help?"
	tests := []struct {
		name            string
		additional      map[string]any
		wantReply       string // "" means the session must stay silent
		wantInterrupted bool
	}{
		{
			name:       "declared auto_respond keeps its response_text",
			additional: map[string]any{"auto_respond": true, "response_text": declaredReply},
			wantReply:  declaredReply,
		},
		{
			name:       "real provider stand-in replies by default",
			additional: map[string]any{},
			wantReply:  mock.DefaultMockStreamingResponse,
		},
		{
			name:       "explicit auto_respond false stays silent",
			additional: map[string]any{"auto_respond": false},
		},
		{
			// YAML decodes numbers as float64 and may leave bools as strings.
			name:       "close_after_turns with close_no_response ends the session silently",
			additional: map[string]any{"close_after_turns": float64(1), "close_no_response": "true"},
		},
		{
			name:            "interrupt_on_turn interrupts that turn",
			additional:      map[string]any{"interrupt_on_turn": int64(1)},
			wantInterrupted: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := engineWithMockProvider(t, t.TempDir(), "")
			for k, v := range tt.additional {
				eng.providers["declared-mock"].AdditionalConfig[k] = v
			}
			if err := eng.EnableMockProviderMode(""); err != nil {
				t.Fatalf("EnableMockProviderMode failed: %v", err)
			}
			got, replied, interrupted := firstDuplexReply(t, eng, "declared-mock")
			switch {
			case tt.wantInterrupted:
				if !interrupted {
					t.Errorf("interrupt_on_turn: 1 was not applied: reply %q arrived uninterrupted", got)
				}
			case tt.wantReply == "" && replied:
				t.Errorf("auto_respond: false must keep the session silent, got %q", got)
			case tt.wantReply != "" && !replied:
				t.Fatal("duplex session never replied: auto_respond was not applied")
			case tt.wantReply != "" && got != tt.wantReply:
				t.Errorf("reply = %q, want %q", got, tt.wantReply)
			}
		})
	}
}

// firstDuplexReply opens a duplex session on providerID, sends one text turn
// and returns the accumulated reply, whether any arrived, and whether the turn
// was interrupted.
func firstDuplexReply(t *testing.T, eng *Engine, providerID string) (reply string, replied, interrupted bool) {
	t.Helper()
	prov, _ := eng.providerRegistry.Get(providerID)
	streamer, ok := prov.(providers.StreamInputSupport)
	if !ok {
		t.Fatalf("mock replacement %T does not support duplex sessions", prov)
	}
	session, err := streamer.CreateStreamSession(context.Background(), &providers.StreamingInputConfig{})
	if err != nil {
		t.Fatalf("CreateStreamSession failed: %v", err)
	}
	defer session.Close()
	if err := session.SendText(context.Background(), "hello"); err != nil {
		t.Fatalf("SendText failed: %v", err)
	}
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case chunk, open := <-session.Response():
			if !open {
				return reply, reply != "", interrupted
			}
			reply += chunk.Delta
			interrupted = interrupted || chunk.Interrupted
			if chunk.FinishReason != nil || chunk.Interrupted {
				return reply, true, interrupted
			}
		case <-timeout:
			return reply, reply != "", interrupted
		}
	}
}
