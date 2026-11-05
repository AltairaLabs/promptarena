package arenaconfig

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTransforms(t *testing.T) {
	selfPlay := func(ts ...MessageTransform) Scenario {
		return Scenario{ID: "s", Turns: []TurnDefinition{{Role: "attacker", Persona: "p", Transforms: ts}}}
	}
	scripted := Scenario{ID: "s", Turns: []TurnDefinition{{Role: "user", Content: "hi",
		Transforms: []MessageTransform{{Converter: "best_of_n"}}}}}
	assert.NoError(t, scripted.Validate(), "a scripted user turn takes transforms")

	ok := selfPlay(MessageTransform{Template: "{{message}}!"}, MessageTransform{Converter: "best_of_n"},
		MessageTransform{Exec: &TransformExec{Command: "t.py"}})
	assert.NoError(t, ok.Validate())

	cases := map[string]Scenario{
		"set exactly one of template, converter or exec": selfPlay(MessageTransform{}),
		`converter "caesar" is not one of base64`:        selfPlay(MessageTransform{Converter: "caesar"}),
		"template must contain {{message}}":              selfPlay(MessageTransform{Template: "hi"}),
		"exec.command is required":                       selfPlay(MessageTransform{Exec: &TransformExec{}}),
		"timeout_ms must not be negative":                selfPlay(MessageTransform{Exec: &TransformExec{Command: "x", TimeoutMs: -1}}),
		"transforms rewrite user messages": {ID: "s", Turns: []TurnDefinition{{Role: "assistant", Content: "hi",
			Transforms: []MessageTransform{{Template: "{{message}}"}}}}},
		"no content to transform": {ID: "s", Turns: []TurnDefinition{{Role: "user",
			Transforms: []MessageTransform{{Template: "{{message}}"}}}}},
		"not its parts": {ID: "s", Turns: []TurnDefinition{{Role: "user", Parts: []TurnContentPart{{Type: "text", Text: "hi"}},
			Transforms: []MessageTransform{{Template: "{{message}}"}}}}},
	}
	for want, s := range cases {
		err := s.Validate()
		require.Error(t, err, want)
		assert.Contains(t, err.Error(), want)
	}
	both := selfPlay(MessageTransform{Template: "{{message}}", Exec: &TransformExec{Command: "x"}})
	assert.ErrorContains(t, both.Validate(), "set exactly one")
	both = selfPlay(MessageTransform{Template: "{{message}}", Converter: "rot13"})
	assert.ErrorContains(t, both.Validate(), "set exactly one")

	spoken := Scenario{Duplex: &DuplexConfig{}, Turns: []TurnDefinition{{Role: "attacker", Persona: "p",
		Transforms: []MessageTransform{{Converter: "base64"}}}}}
	assert.ErrorContains(t, validateTransforms(&spoken, &spoken.Turns[0], 0), "converters rewrite text, not speech")
}

func TestTransformConverters_MatchTheSchemaEnum(t *testing.T) {
	field, ok := reflect.TypeOf(MessageTransform{}).FieldByName("Converter")
	require.True(t, ok)
	enum := []string{}
	for _, part := range strings.Split(field.Tag.Get("jsonschema"), ",") {
		if v, found := strings.CutPrefix(part, "enum="); found {
			enum = append(enum, v)
		}
	}
	assert.Equal(t, TransformConverters, enum)
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
	both = duplex(AudioTransform{BestOfN: true, Speed: &g})
	assert.ErrorContains(t, validateAudioTransforms(&both, &both.Turns[0], 0), "set exactly one")
}

func TestValidateAudioTransforms_SpeedAndPitch(t *testing.T) {
	duplex := func(ts ...AudioTransform) Scenario {
		return Scenario{ID: "s", Duplex: &DuplexConfig{}, Turns: []TurnDefinition{{Role: "user", Content: "hi", AudioTransforms: ts}}}
	}
	fast, up := 1.25, -3.0
	ok := duplex(AudioTransform{Speed: &fast}, AudioTransform{PitchSemitones: &up}, AudioTransform{BestOfN: true})
	assert.NoError(t, validateAudioTransforms(&ok, &ok.Turns[0], 0))

	for _, bad := range []float64{0, 0.2, 5, math.NaN(), math.Inf(1)} {
		s := duplex(AudioTransform{Speed: &bad})
		assert.ErrorContains(t, validateAudioTransforms(&s, &s.Turns[0], 0), "speed must be from 0.25 to 4", bad)
	}
	for _, bad := range []float64{25, -30, math.NaN(), math.Inf(-1)} {
		s := duplex(AudioTransform{PitchSemitones: &bad})
		assert.ErrorContains(t, validateAudioTransforms(&s, &s.Turns[0], 0), "pitch_semitones must be from -24 to 24", bad)
	}
}

func TestValidateToolResultTransforms(t *testing.T) {
	note := []MessageTransform{{Template: "{{message}}\nNOTE"}}
	ok := Scenario{ID: "s", ToolResultTransforms: []ToolResultTransform{
		{Tool: "lookup_order", Transforms: note},
		{Tool: "search", Transforms: []MessageTransform{{Converter: "base64"}, {Exec: &TransformExec{Command: "x"}}}},
	}}
	assert.NoError(t, validateToolResultTransforms(&ok))

	for want, s := range map[string]Scenario{
		"tool_result_transforms[0].tool is required": {ToolResultTransforms: []ToolResultTransform{{Transforms: note}}},
		"tool_result_transforms[0].transforms needs at least one transform": {
			ToolResultTransforms: []ToolResultTransform{{Tool: "t"}}},
		"tool_result_transforms[0].transforms[0].template must contain {{message}}": {
			ToolResultTransforms: []ToolResultTransform{{Tool: "t", Transforms: []MessageTransform{{Template: "x"}}}}},
		"tool_result_transforms[0].transforms[0]: set exactly one": {
			ToolResultTransforms: []ToolResultTransform{{Tool: "t", Transforms: []MessageTransform{{}}}}},
		"duplex scenarios run tools outside the turn pipeline": {Duplex: &DuplexConfig{},
			ToolResultTransforms: []ToolResultTransform{{Tool: "t", Transforms: note}}},
	} {
		assert.ErrorContains(t, validateToolResultTransforms(&s), want)
	}
}

func TestValidateToolResultTargets_RejectsClientTools(t *testing.T) {
	note := []MessageTransform{{Template: "{{message}}!"}}
	cfg := func(tool string) *Config {
		return &Config{
			LoadedTools: []config.ToolData{
				{Data: []byte("spec:\n  name: confirm_payment\n  mode: client\n")},
				{Data: []byte("spec:\n  name: lookup\n  mode: mock\n")},
			},
			LoadedScenarios: map[string]*Scenario{"s": {ToolResultTransforms: []ToolResultTransform{
				{Tool: tool, Transforms: note}}}},
		}
	}
	assert.NoError(t, cfg("lookup").validateToolResultTargets())
	assert.NoError(t, cfg("an_mcp_tool").validateToolResultTargets(), "a tool not in a file may come from MCP")
	assert.ErrorContains(t, cfg("confirm_payment").validateToolResultTargets(),
		"confirm_payment is a client tool, whose result the caller supplies")
}
