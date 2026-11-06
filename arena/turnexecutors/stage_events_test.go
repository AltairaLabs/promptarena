package turnexecutors

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/composition"
	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// TestBuildStagePipeline_EmitsStageEvents guards the emitter wiring: without
// WithEventEmitter the pipeline runs silently and the TUI log, web UI and
// OTel listener never see a stage.* event.
func TestBuildStagePipeline_EmitsStageEvents(t *testing.T) {
	reg := tools.NewRegistry()
	require.NoError(t, reg.Register(&tools.ToolDescriptor{Name: "echo", Description: "echoes args", Mode: "echo"}))
	reg.RegisterExecutor(&echoExecutor{})

	bus := events.NewEventBus()
	completed := make(chan *events.StageEventData, 16)
	bus.Subscribe(events.EventStageCompleted, func(e *events.Event) {
		if d, ok := e.Data.(*events.StageEventData); ok {
			completed <- d
		}
	})

	req := &TurnRequest{
		Provider: &mockNonMockProvider{},
		Scenario: &arenaconfig.Scenario{TaskType: "unused"},
		ActiveComposition: &composition.Composition{
			Version: 1,
			Steps: []*composition.Step{{
				ID: "greet", Kind: composition.KindTool, Tool: "echo",
				Args: map[string]any{"message": "hi"},
			}},
		},
		BaseDir:        t.TempDir(),
		EventBus:       bus,
		RunID:          "run-1",
		ConversationID: "conv-1",
	}

	pipeline, err := NewPipelineExecutor(reg, nil).buildStagePipeline(req, map[string]string{})
	require.NoError(t, err)
	_, err = pipeline.ExecuteSync(context.Background(),
		stage.StreamElement{Message: &types.Message{Role: "user", Content: `{"input":"hi"}`}})
	require.NoError(t, err)

	select {
	case d := <-completed:
		require.NotEmpty(t, d.Name)
	case <-time.After(5 * time.Second):
		t.Fatal("no stage.completed event reached the bus")
	}
}
