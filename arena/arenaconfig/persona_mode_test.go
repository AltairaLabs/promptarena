package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAndReturnPersona_ModeAndMaxTokens(t *testing.T) {
	base := func() *UserPersonaPack {
		return &UserPersonaPack{ID: "p", SystemPrompt: "x"}
	}

	t.Run("empty and known modes are accepted", func(t *testing.T) {
		for _, mode := range []string{"", PersonaModeUser, PersonaModeAdversarial} {
			p := base()
			p.Mode = mode
			_, err := validateAndReturnPersona(p, "f.yaml")
			assert.NoError(t, err, mode)
		}
	})

	t.Run("unknown mode is rejected", func(t *testing.T) {
		p := base()
		p.Mode = "chaotic"
		_, err := validateAndReturnPersona(p, "f.yaml")
		assert.ErrorContains(t, err, "invalid mode")
	})

	t.Run("non-positive max_tokens is rejected", func(t *testing.T) {
		p := base()
		zero := 0
		p.Defaults.MaxTokens = &zero
		_, err := validateAndReturnPersona(p, "f.yaml")
		assert.ErrorContains(t, err, "max_tokens")
	})

	t.Run("IsAdversarial", func(t *testing.T) {
		p := base()
		assert.False(t, p.IsAdversarial())
		p.Mode = PersonaModeAdversarial
		assert.True(t, p.IsAdversarial())
	})
}

func TestLoadSelfPlayResources_ValidatesInlinePersonaSpecs(t *testing.T) {
	negative := -50
	tests := []struct {
		name string
		spec *UserPersonaPack
		want string
	}{
		{name: "bad mode", spec: &UserPersonaPack{SystemPrompt: "x", Mode: "Adversarial"}, want: "invalid mode"},
		{name: "bad max_tokens", spec: &UserPersonaPack{SystemPrompt: "x", Defaults: PersonaDefaults{MaxTokens: &negative}}, want: "max_tokens"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				SelfPlay:       &SelfPlayConfig{PersonaSpecs: map[string]*UserPersonaPack{"inline": tt.spec}},
				LoadedPersonas: map[string]*UserPersonaPack{},
			}
			err := c.loadSelfPlayResources("arena.yaml")
			assert.ErrorContains(t, err, tt.want)
		})
	}
}
