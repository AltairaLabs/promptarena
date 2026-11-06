package stages

import (
	"context"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
)

// processInstructionStage is the shared pipeline body for stages that append a
// fixed instruction string to the system prompt
// (CompletionInstructionStage, SkillInstructionStage). The instruction is
// appended exactly once per Turn into TurnState.SystemPrompt, regardless of
// how many elements flow through, once upstream has finished with it (see
// appendInstructionViaTurnState).
//
// An empty instruction string short-circuits to a pure passthrough — no
// element is mutated and TurnState is not modified.
//
//nolint:lll // Channel signature cannot be shortened
func processInstructionStage(ctx context.Context, input <-chan stage.StreamElement, output chan<- stage.StreamElement, instruction string, turnState *stage.TurnState) error {
	defer close(output)

	if instruction == "" || turnState == nil {
		return forwardAllInstructionElements(ctx, input, output)
	}

	return appendInstructionViaTurnState(ctx, input, output, instruction, turnState)
}

func forwardAllInstructionElements(
	ctx context.Context, input <-chan stage.StreamElement, output chan<- stage.StreamElement,
) error {
	for elem := range input {
		select {
		case output <- elem:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// appendInstructionViaTurnState appends the instruction exactly once into
// TurnState.SystemPrompt, then forwards the turn's elements unmodified.
//
// The append waits until upstream has closed its output. SystemPrompt is not
// this stage's alone: PromptKit's TemplateStage, which runs upstream, owns it
// until it has seen its last element, and it re-reads it on every element to
// decide whether the render is cached. Appending on the first element — while
// TemplateStage is still processing the turn's remaining history elements —
// is an unsynchronised write against those reads. Upstream closing its output
// is the hand-off: every upstream access happens-before that close, and every
// downstream reader (ContextBuilderStage, ProviderStage) still sees the
// appended prompt before its first element, because nothing is forwarded
// until the append is done.
//
// Holding the elements is safe because this stage only runs in per-turn
// pipelines, whose input is finite (history plus the turn's message). A turn
// with no elements appends nothing, as before.
func appendInstructionViaTurnState(
	ctx context.Context, input <-chan stage.StreamElement, output chan<- stage.StreamElement,
	instruction string, turnState *stage.TurnState,
) error {
	var held []stage.StreamElement
	for elem := range input {
		held = append(held, elem)
	}
	if len(held) == 0 {
		return nil
	}

	turnState.SystemPrompt += instruction

	for i := range held {
		select {
		case output <- held[i]:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
