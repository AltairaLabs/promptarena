package arenaconfig

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// TransformMessagePlaceholder is replaced with the persona's message in a
// template transform.
const TransformMessagePlaceholder = "{{message}}"

// MessageTransform rewrites a user message before the agent under test
// receives it: a self-play persona's, after it is generated, or a scripted
// turn's text. Set exactly one of
// Template, Converter or Exec.
type MessageTransform struct {
	// Template is the message to send, with {{message}} standing for the
	// persona's message, e.g. "{{message}}\n\nSent from my phone".
	Template string `json:"template,omitempty" yaml:"template,omitempty"`
	// Converter rewrites the message with a built-in converter: base64,
	// rot13, leetspeak, reverse, homoglyph (Cyrillic look-alikes),
	// unicode_tags (invisible tag characters), zero_width (zero-width spaces
	// inside words) or best_of_n (Best-of-N's random scrambling, case flips
	// and character noise, seeded per scenario, trial and exchange).
	Converter string `json:"converter,omitempty" yaml:"converter,omitempty" jsonschema:"enum=base64,enum=rot13,enum=leetspeak,enum=reverse,enum=homoglyph,enum=unicode_tags,enum=zero_width,enum=best_of_n"`
	// Exec runs a program that rewrites the message.
	Exec *TransformExec `json:"exec,omitempty" yaml:"exec,omitempty"`
}

// TransformExec is a program that rewrites a message or its audio. Arena runs
// it on the host, in the arena config's directory. What it reads and writes
// depends on where it is used: see MessageTransform and AudioTransform.
type TransformExec struct {
	// Command is the program. A path containing a separator resolves against
	// the arena config's directory; a bare name is looked up on PATH.
	Command string `json:"command" yaml:"command"`
	// Args are passed to the program.
	Args []string `json:"args,omitempty" yaml:"args,omitempty"`
	// Env adds environment variables for the program.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	// TimeoutMs bounds each run. A message transform defaults to 10000; an
	// audio transform has no default, since it runs as long as the speech.
	TimeoutMs int `json:"timeout_ms,omitempty" yaml:"timeout_ms,omitempty" jsonschema:"minimum=0"`
}

// TransformConverters are the built-in converters a message transform can
// name.
var TransformConverters = []string{
	"base64", "rot13", "leetspeak", "reverse", "homoglyph", "unicode_tags", "zero_width", "best_of_n",
}

// DefaultTransformTimeoutMs bounds an exec transform when timeout_ms is not
// set.
const DefaultTransformTimeoutMs = 10000

// validateTransforms checks the turn at turnIndex's transforms.
func validateTransforms(s *Scenario, turn *TurnDefinition, turnIndex int) error {
	if len(turn.Transforms) > 0 && turn.Persona == "" {
		switch {
		case turn.Role != "user":
			return fmt.Errorf("turns[%d].transforms: transforms rewrite user messages: a self-play turn "+
				"(one with a persona) or a scripted user turn", turnIndex)
		case len(turn.Parts) > 0:
			return fmt.Errorf("turns[%d].transforms: transforms rewrite a scripted turn's content, "+
				"not its parts", turnIndex)
		case strings.TrimSpace(turn.Content) == "":
			return fmt.Errorf("turns[%d].transforms: the scripted turn has no content to transform", turnIndex)
		}
	}
	for i := range turn.Transforms {
		t := &turn.Transforms[i]
		where := fmt.Sprintf("turns[%d].transforms[%d]", turnIndex, i)
		switch {
		case countSet(t.Template != "", t.Converter != "", t.Exec != nil) != 1:
			return fmt.Errorf("%s: set exactly one of template, converter or exec", where)
		case t.Converter != "" && s.Duplex != nil:
			// The converters rewrite text for reading: spoken, an encoding is
			// noise and tag characters are silence, so the agent would pass an
			// attack it never heard.
			return fmt.Errorf("%s.converter: converters rewrite text, not speech, so a duplex turn can't use them; "+
				"use audio_transforms", where)
		case t.Converter != "" && !slices.Contains(TransformConverters, t.Converter):
			return fmt.Errorf("%s.converter %q is not one of %s", where, t.Converter,
				strings.Join(TransformConverters, ", "))
		case t.Template != "" && !strings.Contains(t.Template, TransformMessagePlaceholder):
			return fmt.Errorf("%s.template must contain %s", where, TransformMessagePlaceholder)
		case t.Exec != nil && strings.TrimSpace(t.Exec.Command) == "":
			return fmt.Errorf("%s.exec.command is required", where)
		case t.Exec != nil && t.Exec.TimeoutMs < 0:
			return fmt.Errorf("%s.exec.timeout_ms must not be negative", where)
		}
	}
	return nil
}

// AudioTransform rewrites the audio of a duplex turn's spoken user message
// after it is synthesized (and after the TTS cache) and before the agent
// hears it. Set exactly one field.
type AudioTransform struct {
	// GainDB scales the audio by this many decibels: negative is quieter,
	// positive louder (clipped at full scale).
	GainDB *float64 `json:"gain_db,omitempty" yaml:"gain_db,omitempty"`
	// NoiseDBFS adds white noise at this level, in dB relative to full scale
	// (for example -30), as on a noisy line. The noise is seeded from the
	// message text, so a run is repeatable.
	NoiseDBFS *float64 `json:"noise_dbfs,omitempty" yaml:"noise_dbfs,omitempty" jsonschema:"maximum=0"`
	// Speed changes the tempo and keeps the pitch: 1.25 is a quarter faster,
	// 0.8 slower. From 0.25 to 4.
	Speed *float64 `json:"speed,omitempty" yaml:"speed,omitempty" jsonschema:"minimum=0.25,maximum=4"`
	// PitchSemitones shifts the pitch by this many semitones and keeps the
	// duration: 12 is an octave up, -3 a minor third down. From -24 to 24.
	PitchSemitones *float64 `json:"pitch_semitones,omitempty" yaml:"pitch_semitones,omitempty" jsonschema:"minimum=-24,maximum=24"` //nolint:lll
	// BestOfN applies Best-of-N's audio augmentations (arXiv 2412.03556): a
	// random tempo (0.8 to 1.25), pitch (up to 3 semitones either way), volume
	// (up to 6 dB either way) and, half the time, background noise (-45 to
	// -25 dBFS). The draw is seeded per scenario, turn, trial, defaults.seed
	// and message, so each trial hears its own variant and a run repeats.
	BestOfN bool `json:"best_of_n,omitempty" yaml:"best_of_n,omitempty"`
	// Exec pipes the audio through a program: raw 16-bit little-endian mono
	// PCM on stdin, the same on stdout. ARENA_SAMPLE_RATE carries the sample
	// rate. timeout_ms, when set, bounds the whole utterance.
	Exec *TransformExec `json:"exec,omitempty" yaml:"exec,omitempty"`
}

// The bounds on an audio transform's speed and pitch.
const (
	minAudioSpeed     = 0.25
	maxAudioSpeed     = 4
	maxPitchSemitones = 24
)

// validateAudioTransforms checks the turn at turnIndex's audio transforms.
func validateAudioTransforms(s *Scenario, turn *TurnDefinition, turnIndex int) error {
	if len(turn.AudioTransforms) > 0 && s.Duplex == nil {
		return fmt.Errorf("turns[%d].audio_transforms: audio transforms need a duplex scenario", turnIndex)
	}
	for _, p := range turn.Parts {
		if len(turn.AudioTransforms) > 0 && p.Type == "audio" {
			return fmt.Errorf("turns[%d].audio_transforms: a turn with an audio part streams it as recorded; "+
				"audio transforms apply to synthesized speech", turnIndex)
		}
	}
	for i := range turn.AudioTransforms {
		t := &turn.AudioTransforms[i]
		where := fmt.Sprintf("turns[%d].audio_transforms[%d]", turnIndex, i)
		switch {
		case countSet(t.GainDB != nil, t.NoiseDBFS != nil, t.Speed != nil, t.PitchSemitones != nil,
			t.BestOfN, t.Exec != nil) != 1:
			return fmt.Errorf("%s: set exactly one of gain_db, noise_dbfs, speed, pitch_semitones, best_of_n or exec",
				where)
		case t.Speed != nil && !(*t.Speed >= minAudioSpeed && *t.Speed <= maxAudioSpeed):
			return fmt.Errorf("%s.speed must be from %v to %v, got %v", where, minAudioSpeed, maxAudioSpeed, *t.Speed)
		case t.PitchSemitones != nil && !(math.Abs(*t.PitchSemitones) <= maxPitchSemitones):
			return fmt.Errorf("%s.pitch_semitones must be from -%v to %v, got %v", where,
				maxPitchSemitones, maxPitchSemitones, *t.PitchSemitones)
		case t.GainDB != nil && (math.IsNaN(*t.GainDB) || math.IsInf(*t.GainDB, 0)):
			return fmt.Errorf("%s.gain_db must be a finite number", where)
		case t.NoiseDBFS != nil && (math.IsNaN(*t.NoiseDBFS) || math.IsInf(*t.NoiseDBFS, 1)):
			return fmt.Errorf("%s.noise_dbfs must be a number of dB, got %v", where, *t.NoiseDBFS)
		case t.NoiseDBFS != nil && *t.NoiseDBFS > 0:
			return fmt.Errorf("%s.noise_dbfs must be 0 or below, got %v", where, *t.NoiseDBFS)
		case t.Exec != nil && strings.TrimSpace(t.Exec.Command) == "":
			return fmt.Errorf("%s.exec.command is required", where)
		case t.Exec != nil && t.Exec.TimeoutMs < 0:
			return fmt.Errorf("%s.exec.timeout_ms must not be negative", where)
		}
	}
	return nil
}

// countSet counts the fields that are set.
func countSet(set ...bool) int {
	n := 0
	for _, isSet := range set {
		if isSet {
			n++
		}
	}
	return n
}
