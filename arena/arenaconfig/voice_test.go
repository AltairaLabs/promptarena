package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

func TestArenaSpec_TTSProvidersAndVoices(t *testing.T) {
	src := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: t
spec:
  providers:
    - file: providers/llm.provider.yaml
  tts_providers:
    - file: providers/cartesia-confident-man.provider.yaml
    - file: providers/mock-tts.provider.yaml
  stt_providers: []
  voices:
    - id: confident-man
      provider: cartesia-confident-man
`
	var arena struct {
		Spec Config `yaml:"spec"`
	}
	if err := yaml.Unmarshal([]byte(src), &arena); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(arena.Spec.TTSProviders) != 2 {
		t.Fatalf("tts_providers: got %d entries, want 2", len(arena.Spec.TTSProviders))
	}
	if len(arena.Spec.Voices) != 1 {
		t.Fatalf("voices: got %d entries, want 1", len(arena.Spec.Voices))
	}
	if arena.Spec.Voices[0].ID != "confident-man" {
		t.Fatalf("voices[0].id: got %q", arena.Spec.Voices[0].ID)
	}
	if arena.Spec.Voices[0].Provider != "cartesia-confident-man" {
		t.Fatalf("voices[0].provider: got %q", arena.Spec.Voices[0].Provider)
	}
}

func TestConfig_ResolveVoice(t *testing.T) {
	cfg := &Config{
		Voices: []config.VoiceBinding{{ID: "confident-man", Provider: "cartesia-confident-man"}},
		LoadedTTSProviders: map[string]*config.Provider{
			"cartesia-confident-man": {ID: "cartesia-confident-man", Type: "cartesia", Voice: "vid-1", Role: "tts"},
		},
	}
	p, err := cfg.ResolveVoice("confident-man")
	if err != nil {
		t.Fatalf("ResolveVoice: %v", err)
	}
	if p.Voice != "vid-1" {
		t.Fatalf("provider voice: got %q", p.Voice)
	}
}

func TestConfig_ResolveVoice_UnknownID(t *testing.T) {
	cfg := &Config{}
	if _, err := cfg.ResolveVoice("nope"); err == nil {
		t.Fatal("expected error for unknown voice id")
	}
}

func TestConfig_ResolveVoice_BindingMissingProvider(t *testing.T) {
	cfg := &Config{
		Voices:             []config.VoiceBinding{{ID: "confident-man", Provider: "ghost"}},
		LoadedTTSProviders: map[string]*config.Provider{},
	}
	if _, err := cfg.ResolveVoice("confident-man"); err == nil {
		t.Fatal("expected error when binding's provider is not loaded")
	}
}

func TestScenario_TrialVoice(t *testing.T) {
	s := &Scenario{Voices: []string{"a", "b", "c"}}
	got := []string{}
	for trial := 0; trial < 5; trial++ {
		got = append(got, s.TrialVoice(trial))
	}
	assert.Equal(t, []string{"a", "b", "c", "a", "b"}, got)
	assert.Empty(t, (&Scenario{Voice: "x"}).TrialVoice(2), "voice alone sets no trial voice")
	assert.Empty(t, (*Scenario)(nil).TrialVoice(0))
}

func TestValidateScenarioVoices_Voices(t *testing.T) {
	cfg := func(s *Scenario) *Config {
		return &Config{
			Voices: []config.VoiceBinding{{ID: "low", Provider: "p"}, {ID: "high", Provider: "p"}},
			LoadedTTSProviders: map[string]*config.Provider{"p": {ID: "p", Role: "tts"}},
			LoadedScenarios:    map[string]*Scenario{"s": s},
		}
	}
	assert.NoError(t, cfg(&Scenario{Duplex: &DuplexConfig{}, Voices: []string{"low", "high"}}).validateScenarioVoices())
	for want, s := range map[string]*Scenario{
		"set voice or voices, not both": {Duplex: &DuplexConfig{}, Voice: "low", Voices: []string{"high"}},
		"voices need a duplex scenario": {Voices: []string{"low"}},
		`voice id "ghost" not found`:    {Duplex: &DuplexConfig{}, Voices: []string{"low", "ghost"}},
		`voice id "" not found`:         {Duplex: &DuplexConfig{}, Voices: []string{""}},
	} {
		assert.ErrorContains(t, cfg(s).validateScenarioVoices(), want)
	}
}
