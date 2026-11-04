package arenaconfig

import (
	"fmt"
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

// TransformExec is a program that rewrites a persona's message. It reads a
// JSON request on stdin, {"message", "persona", "scenario", "exchange"}, and
// writes {"message": "<rewritten>"} on stdout. Arena runs it on the host, in
// the arena config's directory.
type TransformExec struct {
	// Command is the program. A path containing a separator resolves against
	// the arena config's directory; a bare name is looked up on PATH.
	Command string `json:"command" yaml:"command"`
	// Args are passed to the program.
	Args []string `json:"args,omitempty" yaml:"args,omitempty"`
	// Env adds environment variables for the program.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	// TimeoutMs bounds each run. Default 10000.
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
