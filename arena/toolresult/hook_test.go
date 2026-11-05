package toolresult

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/hooks"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

func TestHook_RewritesOnlyTheNamedTool(t *testing.T) {
	h := NewHook([]arenaconfig.ToolResultTransform{
		{Tool: "lookup_order", Transforms: []arenaconfig.MessageTransform{
			{Template: "{{message}}\nNOTE: planted"}, {Converter: "rot13"},
		}},
	}, Run{ConfigDir: "", Scenario: "s"})
	var _ hooks.ToolHook = h
	var _ hooks.ToolResultRewriter = h

	got, err := h.RewriteResult(context.Background(), hooks.ToolRequest{Name: "lookup_order"}, `{"status":"shipped"}`)
	require.NoError(t, err)
	assert.Equal(t, "{\"fgnghf\":\"fuvccrq\"}\nABGR: cynagrq", got, "in order: the note, then rot13")

	got, err = h.RewriteResult(context.Background(), hooks.ToolRequest{Name: "other"}, "unchanged")
	require.NoError(t, err)
	assert.Equal(t, "unchanged", got)

	assert.True(t, h.BeforeExecution(context.Background(), hooks.ToolRequest{}).Allow)
	assert.True(t, h.AfterExecution(context.Background(), hooks.ToolRequest{}, hooks.ToolResponse{}).Allow)
}

func TestHook_RulesForTheSameToolRunInTurn(t *testing.T) {
	h := NewHook([]arenaconfig.ToolResultTransform{
		{Tool: "t", Transforms: []arenaconfig.MessageTransform{{Template: "[{{message}}]"}}},
		{Tool: "t", Transforms: []arenaconfig.MessageTransform{{Template: "<{{message}}>"}}},
	}, Run{})
	got, err := h.RewriteResult(context.Background(), hooks.ToolRequest{Name: "t"}, "r")
	require.NoError(t, err)
	assert.Equal(t, "<[r]>", got)
}

func TestHook_ExecGetsTheToolAndItsArgs(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "echo.sh")
	// Writes back what it received, as the message.
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\npython3 -c 'import json,sys; r=json.load(sys.stdin); "+
		"print(json.dumps({\"message\": r[\"tool\"]+\" \"+json.dumps(r[\"tool_args\"])+\" \"+r[\"message\"]+\" \"+str(r[\"trial\"])+\" \"+r[\"tool_call_id\"]}))'\n"), 0o755))
	h := NewHook([]arenaconfig.ToolResultTransform{
		{Tool: "search", Transforms: []arenaconfig.MessageTransform{{Exec: &arenaconfig.TransformExec{Command: script}}}},
	}, Run{Scenario: "s", Trial: 2})
	got, err := h.RewriteResult(context.Background(),
		hooks.ToolRequest{Name: "search", Args: json.RawMessage(`{"q":"refunds"}`), CallID: "call-7"}, "results")
	require.NoError(t, err)
	assert.Equal(t, `search {"q": "refunds"} results 2 call-7`, got)
}

func TestHook_FailingTransformIsAnErrorTheRunSees(t *testing.T) {
	h := NewHook([]arenaconfig.ToolResultTransform{
		{Tool: "t", Transforms: []arenaconfig.MessageTransform{{Exec: &arenaconfig.TransformExec{Command: "/no/such/program"}}}},
	}, Run{})
	ctx, failed := WithFailureLog(context.Background())
	require.NoError(t, failed())
	_, err := h.RewriteResult(ctx, hooks.ToolRequest{Name: "t"}, "r")
	assert.ErrorContains(t, err, "tool_result_transforms[0] on t: transforms[0]")
	assert.ErrorContains(t, failed(), "tool_result_transforms[0] on t", "the run's log has the failure")

	_, err = h.RewriteResult(context.Background(), hooks.ToolRequest{Name: "t"}, "r")
	assert.Error(t, err, "without a log the error is still returned")
}

func TestNewHook_NilWithoutRules(t *testing.T) {
	assert.Nil(t, NewHook(nil, Run{}))
}

func TestHook_EachCallDrawsItsOwnRandomVariant(t *testing.T) {
	h := NewHook([]arenaconfig.ToolResultTransform{
		{Tool: "t", Transforms: []arenaconfig.MessageTransform{{Converter: "best_of_n"}}},
	}, Run{Scenario: "s"})
	result := "the same result from both calls, long enough to scramble"
	a, err := h.RewriteResult(context.Background(), hooks.ToolRequest{Name: "t", CallID: "c1"}, result)
	require.NoError(t, err)
	b, err := h.RewriteResult(context.Background(), hooks.ToolRequest{Name: "t", CallID: "c2"}, result)
	require.NoError(t, err)
	assert.NotEqual(t, a, b, "two calls with the same result get different variants")
}
