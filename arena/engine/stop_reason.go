package engine

import (
	"errors"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
)

// StopReasonMaxRounds marks a run whose tool loop hit tool_policy.max_rounds:
// the model was still calling tools when the round limit ended the turn.
const StopReasonMaxRounds = "max_rounds"

// stopReasonFor classifies a turn error into a machine-readable stop reason,
// so results can separate a run cut off by a cap from one that broke. Empty
// when the error is not a recognized cap.
func stopReasonFor(err error) string {
	if errors.Is(err, stage.ErrMaxRoundsExceeded) {
		return StopReasonMaxRounds
	}
	return ""
}
