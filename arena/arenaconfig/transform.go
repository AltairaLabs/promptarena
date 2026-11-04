package arenaconfig

import (
	"fmt"
	"math"
	"strings"
)

// TransformMessagePlaceholder is replaced with the persona's message in a
// template transform.
const TransformMessagePlaceholder = "{{message}}"

// MessageTransform rewrites a self-play persona's message after it is
// generated and before the agent under test receives it. Set exactly one of
// Template or Exec.
type MessageTransform struct {
	// Template is the message to send, with {{message}} standing for the
	// persona's message, e.g. "{{message}}\n\nSent from my phone".
	Template string `json:"template,omitempty" yaml:"template,omitempty"`
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

// DefaultTransformTimeoutMs bounds an exec transform when timeout_ms is not
// set.
const DefaultTransformTimeoutMs = 10000

// validateTransforms checks the turn at turnIndex's transforms.
func validateTransforms(turn *TurnDefinition, turnIndex int) error {
	if len(turn.Transforms) > 0 && turn.Persona == "" {
		return fmt.Errorf("turns[%d].transforms: transforms need a self-play turn (one with a persona)", turnIndex)
	}
	for i := range turn.Transforms {
		t := &turn.Transforms[i]
		where := fmt.Sprintf("turns[%d].transforms[%d]", turnIndex, i)
		switch {
		case (t.Template == "") == (t.Exec == nil):
			return fmt.Errorf("%s: set exactly one of template or exec", where)
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
	// Exec pipes the audio through a program: raw 16-bit little-endian mono
	// PCM on stdin, the same on stdout. ARENA_SAMPLE_RATE carries the sample
	// rate. timeout_ms, when set, bounds the whole utterance.
	Exec *TransformExec `json:"exec,omitempty" yaml:"exec,omitempty"`
}

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
		set := 0
		for _, isSet := range []bool{t.GainDB != nil, t.NoiseDBFS != nil, t.Exec != nil} {
			if isSet {
				set++
			}
		}
		switch {
		case set != 1:
			return fmt.Errorf("%s: set exactly one of gain_db, noise_dbfs or exec", where)
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
