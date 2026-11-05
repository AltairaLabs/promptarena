package arenaconfig

import (
	"fmt"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

// ResolveVoice returns the TTS provider config bound to the given voice id.
// Returns an error if the voice id is not in spec.voices, or if the binding
// points to a provider that wasn't loaded.
func (c *Config) ResolveVoice(voiceID string) (*config.Provider, error) {
	if c == nil {
		return nil, fmt.Errorf("config is nil")
	}
	for i := range c.Voices {
		if c.Voices[i].ID != voiceID {
			continue
		}
		providerID := c.Voices[i].Provider
		if p, ok := c.LoadedTTSProviders[providerID]; ok && p != nil {
			return p, nil
		}
		return nil, fmt.Errorf("voice %q binds to provider %q which is not loaded in spec.tts_providers",
			voiceID, providerID)
	}
	return nil, fmt.Errorf("voice id %q not found in spec.voices", voiceID)
}

// TrialVoice returns the voice id the scenario's voices give a repetition of
// the scenario (a trial, counted once per cycle of perturbation variants), or
// "" when it sets none.
func (s *Scenario) TrialVoice(repetition int) string {
	if s == nil || len(s.Voices) == 0 {
		return ""
	}
	return s.Voices[repetition%len(s.Voices)]
}
