package agentkb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvalLevel(t *testing.T) {
	assert.Equal(t, EvalLevelTurn, EvalLevel("tools_called"))
	assert.Equal(t, EvalLevelTurn, EvalLevel("tool_call_count"))
	assert.Equal(t, EvalLevelSession, EvalLevel("tools_called_session"))
	assert.Equal(t, EvalLevelSession, EvalLevel("tool_call_sequence"))
	assert.Equal(t, EvalLevelTurn, EvalLevel("tool_called"), "an alias reports its target's level")
	assert.Equal(t, EvalLevelSession, EvalLevel("tools_not_called_with_args"),
		"an alias of a session-level type is session-level")
	assert.Equal(t, "", EvalLevel("not_a_registered_eval"))
}
