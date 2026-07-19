package engine

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

// Voice capability is normally answered by asking each provider whether it
// accepts streaming audio input. --mock-provider replaces every provider with
// a mock, and the mock claims every capability unconditionally — so in mock
// mode that question reported any config as voice-capable, and the interactive
// console offered a voice call on a plain text demo.
// providerIDsClaimingAudio asks the engine's CURRENT registry directly, the way
// VoiceProviderIDs does outside mock mode. Used to prove the mock registry does
// claim realtime audio, so the mock-mode assertions are not vacuous.
func providerIDsClaimingAudio(e *Engine) []string {
	out := []string{}
	for _, id := range e.ProviderIDs() {
		if p, ok := e.providerRegistry.Get(id); ok && providerSupportsRealtimeAudio(p) {
			out = append(out, id)
		}
	}
	return out
}

func TestVoiceCapability_MockMode(t *testing.T) {
	textOnly := map[string]*arenaconfig.Scenario{"chat": {}}

	// Real providers, then mock mode switched on the way --mock-provider does
	// it. Without the mock-mode gate these providers report voice support and
	// the console offers a call, so this subtest fails on the old behaviour
	// rather than passing vacuously on an empty provider list.
	t.Run("mock mode without a duplex scenario offers no voice", func(t *testing.T) {
		loaded := map[string]*config.Provider{
			"gpt4":   {ID: "gpt4", Type: "openai", Model: "gpt-4"},
			"claude": {ID: "claude", Type: "anthropic", Model: "claude-sonnet"},
		}
		e := &Engine{
			config: &arenaconfig.Config{
				LoadedScenarios: textOnly,
				LoadedProviders: loaded,
			},
			providers: loaded,
			// Registry with nothing voice-capable in it — the stand-in for a
			// config whose real providers are plain text models.
			providerRegistry: providers.NewRegistry(),
		}

		if err := e.EnableMockProviderMode(""); err != nil {
			t.Fatalf("EnableMockProviderMode: %v", err)
		}

		// Sanity: the mocks now in the registry DO claim realtime audio, so an
		// answer derived from them would be non-empty. That is the behaviour
		// being corrected, and it keeps the assertion below from being vacuous.
		if len(providerIDsClaimingAudio(e)) == 0 {
			t.Fatal("expected the mock registry to claim voice support; the assertion below would be vacuous")
		}

		if got := e.VoiceProviderIDs(); len(got) != 0 {
			t.Errorf("VoiceProviderIDs() = %v, want none in mock mode on a text-only config", got)
		}
		if e.SupportsVoice() {
			t.Error("SupportsVoice() = true, want false in mock mode on a text-only config")
		}
	})

	// A genuinely voice-capable config must keep working under --mock-provider:
	// the capability captured from the real providers is what answers.
	t.Run("a realtime-capable provider still reports voice in mock mode", func(t *testing.T) {
		e := &Engine{
			config:               &arenaconfig.Config{LoadedScenarios: textOnly},
			mockProviderMode:     true,
			realVoiceProviderIDs: []string{"realtime-one"},
		}
		if got := e.VoiceProviderIDs(); len(got) != 1 || got[0] != "realtime-one" {
			t.Errorf("VoiceProviderIDs() = %v, want [realtime-one]", got)
		}
		if !e.SupportsVoice() {
			t.Error("SupportsVoice() = false, want true — a real provider supported realtime audio")
		}
	})

	t.Run("a duplex scenario reports voice regardless of providers", func(t *testing.T) {
		e := &Engine{
			config: &arenaconfig.Config{LoadedScenarios: map[string]*arenaconfig.Scenario{
				"call": {Duplex: &arenaconfig.DuplexConfig{}},
			}},
			mockProviderMode: true,
		}
		if !e.SupportsVoice() {
			t.Error("SupportsVoice() = false, want true — the config declares a duplex scenario")
		}
	})

	t.Run("outside mock mode the providers are still asked", func(t *testing.T) {
		// No providers loaded, so nothing can report voice support; the point
		// is that the mock-mode short circuit does not apply here.
		e := &Engine{
			config: &arenaconfig.Config{
				LoadedScenarios: textOnly,
				LoadedProviders: map[string]*config.Provider{},
			},
			providerRegistry: nil,
		}
		if got := e.VoiceProviderIDs(); len(got) != 0 {
			t.Errorf("VoiceProviderIDs() = %v, want none", got)
		}
	})
}
