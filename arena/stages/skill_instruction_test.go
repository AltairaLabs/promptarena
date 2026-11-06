package stages

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// SystemPrompt belongs to the upstream TemplateStage until it closes its
// output: it re-reads the field on every element. The instruction stage must
// neither write it nor forward anything while upstream is still open.
func TestSkillInstructionStage_WaitsForUpstreamBeforeAppending(t *testing.T) {
	turnState := stage.NewTurnState()
	turnState.SystemPrompt = "Base."
	s := NewSkillInstructionStageWithTurnState(" Skills.", turnState)

	input := make(chan stage.StreamElement, 2)
	output := make(chan stage.StreamElement, 2)
	done := make(chan error, 1)
	go func() { done <- s.Process(context.Background(), input, output) }()

	input <- stage.StreamElement{}
	select {
	case <-output:
		t.Fatal("forwarded an element before upstream closed")
	case <-time.After(50 * time.Millisecond):
	}

	input <- stage.StreamElement{}
	close(input)
	require.NoError(t, <-done)
	assert.Len(t, output, 2)
	assert.Equal(t, "Base. Skills.", turnState.SystemPrompt)
}

func TestSkillInstructionStage_NoElementsAppendsNothing(t *testing.T) {
	turnState := stage.NewTurnState()
	turnState.SystemPrompt = "Base."
	s := NewSkillInstructionStageWithTurnState(" Skills.", turnState)

	results := runStage(t, s, nil)
	assert.Empty(t, results)
	assert.Equal(t, "Base.", turnState.SystemPrompt)
}

// The production shape: TemplateStage renders into the shared TurnState and
// re-reads it for every history element while the instruction stage appends.
// Under -race this is the regression test for that data race.
func TestSkillInstructionStage_AfterTemplateStageMultiElementTurn(t *testing.T) {
	turnState := stage.NewTurnState()
	turnState.Template = &prompt.Template{RawTemplate: "Hello {{name}}.", TaskType: "chat"}
	turnState.Variables = map[string]string{"name": "Ada"}

	pl, err := stage.NewPipelineBuilder().Chain(
		stage.NewTemplateStageWithTurnState(nil, turnState),
		NewSkillInstructionStageWithTurnState(" Skills.", turnState),
	).Build()
	require.NoError(t, err)

	input := make(chan stage.StreamElement, 5)
	for range 5 {
		input <- stage.StreamElement{}
	}
	close(input)

	out, err := pl.Execute(context.Background(), input)
	require.NoError(t, err)
	count := 0
	for range out {
		count++
	}
	assert.Equal(t, 5, count)
	assert.Equal(t, "Hello Ada. Skills.", turnState.SystemPrompt)
}

func TestSkillInstructionStage_TurnStateAppendsOnce(t *testing.T) {
	turnState := stage.NewTurnState()
	turnState.SystemPrompt = "Base prompt."
	s := NewSkillInstructionStageWithTurnState("\n\n# Skills\nFoo", turnState)

	inputs := []stage.StreamElement{{}, {}}
	results := runStage(t, s, inputs)
	require.Len(t, results, 2)

	expected := "Base prompt.\n\n# Skills\nFoo"
	assert.Equal(t, expected, turnState.SystemPrompt, "appended exactly once into TurnState")
}

func TestSkillInstructionStage_AppendsToTurnStateSystemPrompt(t *testing.T) {
	turnState := stage.NewTurnState()
	turnState.SystemPrompt = "You are a helpful assistant."
	s := NewSkillInstructionStageWithTurnState("\n\n# Active Skills\n\n## memory-protocol\n\nCall memory__recall first.\n", turnState)

	results := runStage(t, s, []stage.StreamElement{{}})
	require.Len(t, results, 1)
	assert.Equal(t,
		"You are a helpful assistant.\n\n# Active Skills\n\n## memory-protocol\n\nCall memory__recall first.\n",
		turnState.SystemPrompt)
}

func TestSkillInstructionStage_EmptyInstructionsNoOp(t *testing.T) {
	turnState := stage.NewTurnState()
	turnState.SystemPrompt = "You are helpful."
	s := NewSkillInstructionStageWithTurnState("", turnState)

	results := runStage(t, s, []stage.StreamElement{{}})
	require.Len(t, results, 1)
	assert.Equal(t, "You are helpful.", turnState.SystemPrompt)
}

func TestSkillInstructionStage_NilTurnStateNoOp(t *testing.T) {
	s := NewSkillInstructionStageWithTurnState("\nskill instructions", nil)

	results := runStage(t, s, []stage.StreamElement{{}})
	require.Len(t, results, 1)
}
