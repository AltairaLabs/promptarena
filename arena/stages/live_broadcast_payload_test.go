package stages

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/PromptKit/runtime/types"

	"github.com/AltairaLabs/PromptKit/runtime/pipeline"
	"github.com/AltairaLabs/PromptKit/runtime/pipeline/stage"
	"github.com/AltairaLabs/promptarena/arena/statestore"
)

// Arena builds its own message.created payload because it needs the
// TRANSCRIPT-ABSOLUTE index, which MessageBroadcastStage's per-Process counter
// cannot give it. What it must NOT do is hand-roll the payload SHAPE — it did,
// and the copy drifted: it stripped msg.Parts and passed ToolResult.Parts
// through untouched, so a tool returning an image or audio blob put those bytes
// straight onto the bus feeding the TUI and the web SSE relay.
//
// These tests pin the two things that drift silently, because nothing else
// would notice: the payload renders no binary, and it renders text wherever
// the message happens to carry it.

func liveStage(t *testing.T, convID string) (*ArenaStateStoreSaveStage, *syncBus) {
	t.Helper()
	store := statestore.NewArenaStateStore()
	cfg := &pipeline.StateStoreConfig{Store: store, ConversationID: convID}
	turnState := stage.NewTurnState()
	s := NewArenaStateStoreSaveStageWithTurnState(cfg, turnState)
	bus := &syncBus{}
	return s.WithLiveMessages(bus, "exec", "sess", convID), bus
}

func firstCreated(t *testing.T, bus *syncBus, role string) events.MessageCreatedData {
	t.Helper()
	for _, data := range bus.created {
		if data.Role == role {
			return data
		}
	}
	t.Fatalf("no message.created for role %q reached the bus", role)
	return events.MessageCreatedData{}
}

func strPtr(s string) *string { return &s }

// TestLiveBroadcast_ToolResultBinaryNeverReachesTheBus is the drift this
// refactor removed. The hand-rolled payload stripped one field and not the
// other, and the difference is invisible until a tool returns media.
func TestLiveBroadcast_ToolResultBinaryNeverReachesTheBus(t *testing.T) {
	s, bus := liveStage(t, "live-binary")

	blob := strPtr("/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJ")
	toolRes := types.MessageToolResult{
		ID:   "call-1",
		Name: "screenshot",
		Parts: []types.ContentPart{{
			Type:  "image",
			Media: &types.MediaContent{MIMEType: "image/jpeg", Data: blob},
		}},
	}
	msg := &types.Message{Role: "tool"}
	msg.ToolResult = &toolRes

	runStage(t, s, []stage.StreamElement{stage.NewMessageElement(msg)})

	data := firstCreated(t, bus, "tool")
	require.NotNil(t, data.ToolResult)
	require.Len(t, data.ToolResult.Parts, 1)

	media := data.ToolResult.Parts[0].Media
	require.NotNil(t, media, "the media reference itself must survive — only the bytes go")
	assert.Nil(t, media.Data,
		"a tool result's binary reached the live bus. Arena's consumers are a TUI "+
			"and an SSE relay; neither wants the bytes, and the recording route is "+
			"where binary belongs")
	assert.Equal(t, "image/jpeg", media.MIMEType,
		"stripping must leave the metadata a consumer renders from")
}

// TestLiveBroadcast_MessageBinaryNeverReachesTheBus is the half that already
// worked, kept so a refactor cannot fix one and break the other.
func TestLiveBroadcast_MessageBinaryNeverReachesTheBus(t *testing.T) {
	s, bus := liveStage(t, "live-binary-parts")

	msg := &types.Message{
		Role: "user",
		Parts: []types.ContentPart{{
			Type:  "image",
			Media: &types.MediaContent{MIMEType: "image/png", Data: strPtr("AQIDBA==")},
		}},
	}
	runStage(t, s, []stage.StreamElement{stage.NewMessageElement(msg)})

	data := firstCreated(t, bus, "user")
	require.Len(t, data.Parts, 1)
	require.NotNil(t, data.Parts[0].Media)
	assert.Nil(t, data.Parts[0].Media.Data, "message binary reached the live bus")
}

// TestLiveBroadcast_TextInPartsIsReadable covers the other half of the payload
// contract, and the reason the adapters call GetContent().
//
// A user turn carries its text in Parts with Content empty; an assistant turn
// does the reverse. A consumer reading .Content directly renders every
// Parts-carrying message as an empty bubble — which is what the TUI and the SSE
// relay were doing.
func TestLiveBroadcast_TextInPartsIsReadable(t *testing.T) {
	s, bus := liveStage(t, "live-parts-text")

	msg := &types.Message{
		Role:  "user",
		Parts: []types.ContentPart{{Type: "text", Text: strPtr("where is my order")}},
	}
	runStage(t, s, []stage.StreamElement{stage.NewMessageElement(msg)})

	data := firstCreated(t, bus, "user")
	assert.Empty(t, data.Content, "fixture check: this message's text is in Parts, not Content")
	assert.Equal(t, "where is my order", data.GetContent(),
		"the text a user typed was not recoverable from the payload")
}
