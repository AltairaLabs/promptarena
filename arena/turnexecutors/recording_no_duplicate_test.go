package turnexecutors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// A session recording used to hold every user message twice: once from the
// input recording stage, with a zero timestamp, and once from the output
// stage, which sees the user message the provider stage forwards.
func TestRecording_RecordsEachUserMessageOnce(t *testing.T) {
	for name, provider := range map[string]providers.Provider{
		"streaming":     &MockStreamingProvider{streamChunks: []providers.StreamChunk{{Content: "hi", Delta: "hi"}}},
		"non-streaming": mock.NewProvider("mock-id", "mock-model", false),
	} {
		t.Run(name, func(t *testing.T) {
			store, err := events.NewFileEventStore(t.TempDir())
			require.NoError(t, err)
			_, cfg := newInterruptedArenaStore(t)
			req := interruptedReq(provider, cfg, nil)
			req.RunID = "run-1"
			rc := stage.DefaultRecordingStageConfig()
			req.RecordingConfig = &rc
			req.EventStore = store
			user := types.Message{Role: "user", Content: "hello"}

			require.NoError(t, NewPipelineExecutor(tools.NewRegistry(), nil).Execute(context.Background(), &req, &user))

			got, err := store.Query(context.Background(), &events.EventFilter{SessionID: "run-1"})
			require.NoError(t, err)
			users := 0
			for _, ev := range got {
				if d, ok := ev.Data.(*events.MessageCreatedData); ok && d.Role == "user" {
					users++
					assert.False(t, ev.Timestamp.IsZero(), "the recorded user message carries a timestamp")
				}
			}
			assert.Equal(t, 1, users)
		})
	}
}
