package engine

import (
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// duplexTurnAssertionRecorder holds the turn-level assertion outcome of each
// assistant reply in one duplex conversation attempt, keyed by the reply's
// position among the conversation's assistant messages.
//
// The outcomes cannot live on the stored messages while the session runs: the
// duplex pipeline is one long-running ArenaStateStoreSaveStage.Process, and
// every save rewrites the whole transcript from the messages it collected off
// the stream, so metadata written to the store between saves is dropped. The
// recorder keeps the outcomes beside the run and stamps them onto the final
// transcript once the pipeline has finished saving (#446 item 1).
type duplexTurnAssertionRecorder struct {
	mu      sync.Mutex
	byReply map[int]map[string]interface{}
}

// record stores the assertion summary for the assistant reply at ordinal.
func (r *duplexTurnAssertionRecorder) record(ordinal int, summary map[string]interface{}) {
	if r == nil || ordinal < 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byReply == nil {
		r.byReply = make(map[int]map[string]interface{})
	}
	r.byReply[ordinal] = summary
}

// apply writes each recorded summary to Meta["assertions"] of the matching
// assistant message and reports whether it wrote any. Meta is copied before
// it is written, so a map shared with the state store is never mutated.
func (r *duplexTurnAssertionRecorder) apply(messages []types.Message) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stamped := false
	ordinal := 0
	for i := range messages {
		if messages[i].Role != roleAssistant {
			continue
		}
		if summary, ok := r.byReply[ordinal]; ok {
			meta := make(map[string]interface{}, len(messages[i].Meta)+1)
			for k, v := range messages[i].Meta {
				meta[k] = v
			}
			meta["assertions"] = summary
			messages[i].Meta = meta
			stamped = true
		}
		ordinal++
	}
	return stamped
}

// lastAssistantOrdinal returns the position of the last assistant message
// among the assistant messages in messages, or -1 when there is none.
func lastAssistantOrdinal(messages []types.Message) int {
	ordinal := -1
	for i := range messages {
		if messages[i].Role == roleAssistant {
			ordinal++
		}
	}
	return ordinal
}
