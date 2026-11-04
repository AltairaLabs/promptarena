package turnexecutors

import (
	"context"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSelfPlayMetadata_MergesFields(t *testing.T) {
	e := &SelfPlayExecutor{}
	req := TurnRequest{SelfPlayRole: "challenger"}
	exec := &pipeline.ExecutionResult{Metadata: map[string]interface{}{"persona": "p1", "custom": 123}}

	meta := e.buildSelfPlayMetadata(&req, exec)
	if meta["role"] != "challenger" || meta["self_play_execution"] != true {
		t.Fatalf("missing base metadata: %+v", meta)
	}
	if meta["persona"] != "p1" || meta["custom"].(int) != 123 {
		t.Fatalf("did not merge exec metadata: %+v", meta)
	}
}

func TestBuildUserMessageFromResult_SetsLatencyAndMeta(t *testing.T) {
	e := &SelfPlayExecutor{}
	req := TurnRequest{SelfPlayRole: "challenger"}
	start := time.Now().Add(-1500 * time.Millisecond)
	end := time.Now()

	exec := &pipeline.ExecutionResult{
		Response: &pipeline.Response{Content: "hello"},
		Trace:    pipeline.ExecutionTrace{CompletedAt: &end, StartedAt: start},
		CostInfo: types.CostInfo{InputTokens: 1, OutputTokens: 2},
		Metadata: map[string]interface{}{"persona": "p1"},
	}

	msg := e.buildUserMessageFromResult(&req, exec)
	if msg.Role != "user" || msg.Content != "hello" {
		t.Fatalf("unexpected message: %+v", msg)
	}
	if msg.LatencyMs <= 0 {
		t.Fatalf("expected latency to be set, got %d", msg.LatencyMs)
	}
	raw, ok := msg.Meta["raw_response"].(map[string]interface{})
	if !ok || raw["role"] != "challenger" || raw["persona"] != "p1" {
		t.Fatalf("raw_response metadata missing/incorrect: %+v", msg.Meta)
	}
}

func TestTransformUserMessage(t *testing.T) {
	req := &TurnRequest{
		SelfPlayPersona:    "p",
		SelfPlayVars:       map[string]string{selfplay.VarExchange: "3"},
		SelfPlayTransforms: []arenaconfig.MessageTransform{{Template: "{{message}} (sent from my phone)"}},
	}
	msg := types.Message{Role: "user", Content: "refund please"}
	require.NoError(t, transformUserMessage(context.Background(), req, &msg))
	assert.Equal(t, "refund please (sent from my phone)", msg.Content)
	assert.Equal(t, "refund please", msg.Meta[selfplay.MetaPersonaOriginal])

	plain := types.Message{Role: "user", Content: "hi"}
	require.NoError(t, transformUserMessage(context.Background(), &TurnRequest{}, &plain))
	assert.Nil(t, plain.Meta, "no transforms, no original kept")

	ended := types.Message{Role: "user", Content: "  "}
	require.NoError(t, transformUserMessage(context.Background(), req, &ended))
	assert.Equal(t, "  ", ended.Content, "a message that was only the completion marker isn't transformed")

	req.SelfPlayTransforms = []arenaconfig.MessageTransform{{Exec: &arenaconfig.TransformExec{Command: "/no/such/program"}}}
	err := transformUserMessage(context.Background(), req, &msg)
	assert.ErrorContains(t, err, "persona p message: transforms[0]")
}
