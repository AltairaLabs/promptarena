package arenaconfig

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTransforms(t *testing.T) {
	selfPlay := func(ts ...MessageTransform) Scenario {
		return Scenario{ID: "s", Turns: []TurnDefinition{{Role: "attacker", Persona: "p", Transforms: ts}}}
	}
	ok := selfPlay(MessageTransform{Template: "{{message}}!"}, MessageTransform{Exec: &TransformExec{Command: "t.py"}})
	assert.NoError(t, ok.Validate())

	cases := map[string]Scenario{
		"set exactly one of template or exec": selfPlay(MessageTransform{}),
		"template must contain {{message}}":   selfPlay(MessageTransform{Template: "hi"}),
		"exec.command is required":            selfPlay(MessageTransform{Exec: &TransformExec{}}),
		"timeout_ms must not be negative":     selfPlay(MessageTransform{Exec: &TransformExec{Command: "x", TimeoutMs: -1}}),
		"transforms need a self-play turn": {ID: "s", Turns: []TurnDefinition{{Role: "user", Content: "hi",
			Transforms: []MessageTransform{{Template: "{{message}}"}}}}},
	}
	for want, s := range cases {
		err := s.Validate()
		require.Error(t, err, want)
		assert.Contains(t, err.Error(), want)
	}
	both := selfPlay(MessageTransform{Template: "{{message}}", Exec: &TransformExec{Command: "x"}})
	assert.ErrorContains(t, both.Validate(), "set exactly one")
}

func TestValidateAudioTransforms(t *testing.T) {
	g, n, loud := -6.0, -30.0, 3.0
	duplex := func(ts ...AudioTransform) Scenario {
		return Scenario{ID: "s", Duplex: &DuplexConfig{}, Turns: []TurnDefinition{{Role: "user", Content: "hi", AudioTransforms: ts}}}
	}
	ok := duplex(AudioTransform{GainDB: &g}, AudioTransform{NoiseDBFS: &n}, AudioTransform{Exec: &TransformExec{Command: "sox"}})
	assert.NoError(t, validateAudioTransforms(&ok, &ok.Turns[0], 0))

	cases := map[string]Scenario{
		"set exactly one":                 duplex(AudioTransform{}),
		"noise_dbfs must be 0 or below":   duplex(AudioTransform{NoiseDBFS: &loud}),
		"exec.command is required":        duplex(AudioTransform{Exec: &TransformExec{}}),
		"timeout_ms must not be negative": duplex(AudioTransform{Exec: &TransformExec{Command: "x", TimeoutMs: -1}}),
		"audio transforms need a duplex scenario": {ID: "s", Turns: []TurnDefinition{{Role: "user", Content: "hi",
			AudioTransforms: []AudioTransform{{GainDB: &g}}}}},
	}
	for want, s := range cases {
		err := validateAudioTransforms(&s, &s.Turns[0], 0)
		require.Error(t, err, want)
		assert.Contains(t, err.Error(), want)
	}
	nan, inf := math.NaN(), math.Inf(1)
	for want, s := range map[string]Scenario{
		"gain_db must be a finite number":   duplex(AudioTransform{GainDB: &nan}),
		"noise_dbfs must be a number of dB": duplex(AudioTransform{NoiseDBFS: &inf}),
		"audio transforms apply to synthesized speech": {ID: "s", Duplex: &DuplexConfig{}, Turns: []TurnDefinition{{
			Role: "user", Parts: []TurnContentPart{{Type: "audio"}}, AudioTransforms: []AudioTransform{{GainDB: &g}},
		}}},
	} {
		err := validateAudioTransforms(&s, &s.Turns[0], 0)
		require.Error(t, err, want)
		assert.Contains(t, err.Error(), want)
	}
	both := duplex(AudioTransform{GainDB: &g, NoiseDBFS: &n})
	assert.ErrorContains(t, validateAudioTransforms(&both, &both.Turns[0], 0), "set exactly one")
}
