package arenaconfig

import (
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
