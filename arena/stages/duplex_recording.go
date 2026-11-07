package stages

import (
	"context"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
)

// recordingCopySource marks the content copies duplexRecordingSplitStage makes
// for the recorder, so duplexRecordingDropStage can remove them again.
const recordingCopySource = "arena_duplex_recording_copy"

// DuplexOutputRecordingStages returns the stages that record a duplex
// session's output: a split stage, PromptKit's RecordingStage, and a drop
// stage, in that order.
//
// RecordingStage skips every control element, but DuplexProviderStage puts each
// assistant turn's Message (and the turn's last audio chunk) on the element
// that also carries EndOfStream. Recorded as-is, a duplex recording held the
// user turns and the audio but none of the assistant messages, so it could not
// be replayed. The split stage puts a plain copy of that content just ahead of
// the control element, the recorder records the copy, and the drop stage
// removes it, so every stage after these sees exactly the stream it saw
// before.
func DuplexOutputRecordingStages(store events.EventStore, cfg stage.RecordingStageConfig) []stage.Stage {
	return []stage.Stage{
		&duplexRecordingSplitStage{BaseStage: stage.NewBaseStage("duplex_recording_split", stage.StageTypeTransform)},
		stage.NewRecordingStage(store, cfg),
		&duplexRecordingDropStage{BaseStage: stage.NewBaseStage("duplex_recording_drop", stage.StageTypeTransform)},
	}
}

// duplexRecordingSplitStage emits, ahead of each content-carrying control
// element, one marked copy per content field so RecordingStage records it.
type duplexRecordingSplitStage struct {
	stage.BaseStage
}

// Process forwards every element, preceded by the copies recordableCopies makes.
func (s *duplexRecordingSplitStage) Process(
	ctx context.Context, input <-chan stage.StreamElement, output chan<- stage.StreamElement,
) error {
	defer close(output)
	var fwd cancelForwarder
	for elem := range input {
		copies := recordableCopies(&elem)
		for i := range copies {
			fwd.forward(ctx, output, copies[i])
		}
		fwd.forward(ctx, output, elem)
	}
	return ctx.Err()
}

// recordableCopies returns the content RecordingStage would drop from elem:
// nothing for a content element or an error (both are recorded as they are),
// else a copy of its text, audio and message, each on an element of its own
// because the recorder records one content field per element.
func recordableCopies(elem *stage.StreamElement) []stage.StreamElement {
	if !elem.IsControl() || elem.Error != nil {
		return nil
	}
	var copies []stage.StreamElement
	add := func(c stage.StreamElement) {
		c.Timestamp = elem.Timestamp
		c.Source = recordingCopySource
		copies = append(copies, c)
	}
	if elem.Text != nil {
		add(stage.StreamElement{Text: elem.Text})
	}
	if elem.Audio != nil {
		add(stage.StreamElement{Audio: elem.Audio})
	}
	if elem.Message != nil {
		add(stage.StreamElement{Message: elem.Message})
	}
	return copies
}

// duplexRecordingDropStage removes the copies the split stage made.
type duplexRecordingDropStage struct {
	stage.BaseStage
}

// Process forwards every element except the split stage's copies.
func (s *duplexRecordingDropStage) Process(
	ctx context.Context, input <-chan stage.StreamElement, output chan<- stage.StreamElement,
) error {
	defer close(output)
	var fwd cancelForwarder
	for elem := range input {
		if elem.Source == recordingCopySource {
			continue
		}
		fwd.forward(ctx, output, elem)
	}
	return ctx.Err()
}
