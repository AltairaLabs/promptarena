package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/PromptKit/runtime/workflow"
)

func transitionToolMsg(jsonContent string) *types.Message {
	tr := types.NewTextToolResult("tc-1", "workflow__transition", jsonContent)
	return &types.Message{Role: "tool", ToolResult: &tr}
}

func TestBuildTransitionMeta(t *testing.T) {
	e := &Engine{
		workflowSpec: &workflow.Spec{
			States: map[string]*workflow.State{
				"review": {Description: "reviewing", OnEvent: map[string]string{"approve": "done"}},
				"done":   {Description: "finished"}, // no OnEvent → terminal
				"nodesc": {},
			},
		},
	}

	t.Run("returns empty on unparseable content", func(t *testing.T) {
		state, ws := e.buildTransitionMeta(transitionToolMsg("not json"), "start")
		assert.Empty(t, state)
		assert.Nil(t, ws)
	})

	t.Run("returns empty when new_state missing", func(t *testing.T) {
		state, ws := e.buildTransitionMeta(transitionToolMsg(`{"event":"approve"}`), "start")
		assert.Empty(t, state)
		assert.Nil(t, ws)
	})

	t.Run("builds metadata for a non-terminal state", func(t *testing.T) {
		state, ws := e.buildTransitionMeta(
			transitionToolMsg(`{"new_state":"review","event":"submit"}`), "start")
		assert.Equal(t, "review", state)
		assert.Equal(t, "review", ws["current_state"])
		assert.Equal(t, "start", ws["previous_state"])
		assert.Equal(t, "submit", ws["transition"])
		assert.Equal(t, "reviewing", ws["description"])
		assert.Equal(t, false, ws["terminal"])
	})

	t.Run("marks a state with no events terminal", func(t *testing.T) {
		state, ws := e.buildTransitionMeta(
			transitionToolMsg(`{"new_state":"done","event":"approve"}`), "review")
		require.Equal(t, "done", state)
		assert.Equal(t, true, ws["terminal"])
		assert.Equal(t, "finished", ws["description"])
	})

	t.Run("omits description when empty", func(t *testing.T) {
		_, ws := e.buildTransitionMeta(
			transitionToolMsg(`{"new_state":"nodesc","event":"go"}`), "review")
		assert.NotContains(t, ws, "description")
		assert.Equal(t, true, ws["terminal"])
	})
}

func TestBuildEntryStateMeta(t *testing.T) {
	e := &Engine{
		workflowSpec: &workflow.Spec{
			States: map[string]*workflow.State{
				"start": {Description: "the beginning", OnEvent: map[string]string{"go": "next"}},
				"bare":  {},
			},
		},
	}

	t.Run("includes description and available events", func(t *testing.T) {
		ws := e.buildEntryStateMeta("start")
		assert.Equal(t, "start", ws["current_state"])
		assert.Equal(t, "the beginning", ws["description"])
		assert.Equal(t, map[string]string{"go": "next"}, ws["available_events"])
	})

	t.Run("bare state only carries current_state", func(t *testing.T) {
		ws := e.buildEntryStateMeta("bare")
		assert.Equal(t, "bare", ws["current_state"])
		assert.NotContains(t, ws, "description")
		assert.NotContains(t, ws, "available_events")
	})

	t.Run("unknown state only carries current_state", func(t *testing.T) {
		ws := e.buildEntryStateMeta("missing")
		assert.Equal(t, "missing", ws["current_state"])
		assert.Len(t, ws, 1)
	})
}
