package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/arena/selfplay"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// fakeSelfplayGenerator is a test double for selfplay.Generator.
type fakeSelfplayGenerator struct {
	result *pipeline.ExecutionResult
	err    error
}

func (f *fakeSelfplayGenerator) NextUserTurn(
	_ context.Context,
	_ []types.Message,
	_ string,
	_ *selfplay.GeneratorOptions,
) (*pipeline.ExecutionResult, error) {
	return f.result, f.err
}

func TestGenerateSelfplayText(t *testing.T) {
	ctx := context.Background()

	t.Run("returns generated text on success", func(t *testing.T) {
		gen := &fakeSelfplayGenerator{
			result: &pipeline.ExecutionResult{Response: &pipeline.Response{Content: "hi there"}},
		}
		text, res, err := generateSelfplayText(ctx, gen, nil, "s1", 1, 0)
		require.NoError(t, err)
		assert.Equal(t, "hi there", text)
		assert.NotNil(t, res)
	})

	t.Run("propagates generator error", func(t *testing.T) {
		gen := &fakeSelfplayGenerator{err: errors.New("boom")}
		_, _, err := generateSelfplayText(ctx, gen, nil, "s1", 1, 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "turn 2")
	})

	t.Run("errors on nil response", func(t *testing.T) {
		gen := &fakeSelfplayGenerator{result: &pipeline.ExecutionResult{}}
		_, _, err := generateSelfplayText(ctx, gen, nil, "s1", 1, 3)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no text response")
	})

	t.Run("errors on empty content", func(t *testing.T) {
		gen := &fakeSelfplayGenerator{
			result: &pipeline.ExecutionResult{Response: &pipeline.Response{Content: ""}},
		}
		_, _, err := generateSelfplayText(ctx, gen, nil, "s1", 1, 4)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty text")
	})
}

// TestGetTurnsToExecute_OnlySelfPlayRepeats covers getTurnsToExecute. `turns: N`
// means "let the persona speak N times"; honoring it for a scripted turn would
// replay the same fixed utterance N times, which looks like a working
// multi-turn conversation while testing nothing new.
func TestGetTurnsToExecute_OnlySelfPlayRepeats(t *testing.T) {
	t.Run("no self-play registry means one turn, whatever turns says", func(t *testing.T) {
		de := &DuplexConversationExecutor{}
		got := de.getTurnsToExecute(&arenaconfig.TurnDefinition{Role: "customer", Turns: 5})
		assert.Equal(t, 1, got)
	})

	t.Run("a scripted turn runs once", func(t *testing.T) {
		de := &DuplexConversationExecutor{}
		assert.Equal(t, 1, de.getTurnsToExecute(&arenaconfig.TurnDefinition{Turns: 3}))
	})

	t.Run("turns: 0 on any role still runs once", func(t *testing.T) {
		de := &DuplexConversationExecutor{}
		assert.Equal(t, 1, de.getTurnsToExecute(&arenaconfig.TurnDefinition{Role: "customer"}))
	})
}

// TestWaitForLocalPlaybackDrained_NoRouterIsANoOp covers the 0% drain helper.
// It runs between turns, so a missing nil guard would panic mid-conversation
// on any run without local audio — which is every non-voice run.
func TestWaitForLocalPlaybackDrained_NoRouterIsANoOp(t *testing.T) {
	de := &DuplexConversationExecutor{}
	ctx := context.Background()

	de.waitForLocalPlaybackDrained(ctx, nil)
	de.waitForLocalPlaybackDrained(ctx, &ConversationRequest{})
}

// TestStampSelfplayDevMeta_OnlyStampsWhatItHas covers stampSelfplayDevMeta.
// Everything it writes is developer-facing debug metadata, so each guard has to
// fall through quietly rather than fail a turn over absent optional data.
func TestStampSelfplayDevMeta_OnlyStampsWhatItHas(t *testing.T) {
	t.Run("nil meta is a no-op, not a panic", func(t *testing.T) {
		stampSelfplayDevMeta(nil, nil, "persona", map[string]interface{}{"system_prompt": "x"})
	})

	t.Run("the system prompt is copied across when present", func(t *testing.T) {
		meta := map[string]any{}
		stampSelfplayDevMeta(meta, nil, "", map[string]interface{}{"system_prompt": "be terse"})
		assert.Equal(t, "be terse", meta["_selfplay_prompt"])
	})

	t.Run("an empty system prompt is not stamped", func(t *testing.T) {
		meta := map[string]any{}
		stampSelfplayDevMeta(meta, nil, "", map[string]interface{}{"system_prompt": ""})
		assert.NotContains(t, meta, "_selfplay_prompt")
	})

	t.Run("a non-string system prompt is ignored", func(t *testing.T) {
		meta := map[string]any{}
		stampSelfplayDevMeta(meta, nil, "", map[string]interface{}{"system_prompt": 42})
		assert.NotContains(t, meta, "_selfplay_prompt")
	})

	t.Run("no registry or persona means no persona yaml", func(t *testing.T) {
		meta := map[string]any{}
		stampSelfplayDevMeta(meta, nil, "persona-1", nil)
		assert.NotContains(t, meta, "_persona_yaml")

		stampSelfplayDevMeta(meta, selfplay.NewRegistry(nil, nil, nil, nil), "", nil)
		assert.NotContains(t, meta, "_persona_yaml")
	})

	t.Run("an unknown persona is skipped rather than stamped empty", func(t *testing.T) {
		meta := map[string]any{}
		stampSelfplayDevMeta(meta, selfplay.NewRegistry(nil, nil, nil, nil), "no-such-persona", nil)
		assert.NotContains(t, meta, "_persona_yaml")
	})
}

// newTurnErrorArgs builds the minimum turnProcessingArgs handleTurnError reads.
func newTurnErrorArgs(totalTurns, logicalIdx, minTurns int, ignoreLast bool) *turnProcessingArgs {
	turns := make([]arenaconfig.TurnDefinition, totalTurns)
	return &turnProcessingArgs{
		req: &ConversationRequest{
			Scenario: &arenaconfig.Scenario{ID: "s1", Turns: turns},
		},
		cfg: &turnLoopConfig{
			partialSuccessMinTurns:   minTurns,
			ignoreLastTurnSessionEnd: ignoreLast,
		},
		state: &turnLoopState{logicalTurnIdx: logicalIdx},
	}
}

// TestHandleTurnError_DecidesWhatCountsAsFailure covers handleTurnError, which
// sat at 43.8%. It is the difference between a run reported as failed and one
// reported as complete: a provider hanging up after the last scripted turn is
// the normal end of a duplex conversation, while the same hang-up on turn one
// is a real failure. Both arrive as the same errSessionEnded.
func TestHandleTurnError_DecidesWhatCountsAsFailure(t *testing.T) {
	de := &DuplexConversationExecutor{}
	turn := &arenaconfig.TurnDefinition{Role: "customer"}

	t.Run("session end on the final turn is a clean finish", func(t *testing.T) {
		args := newTurnErrorArgs(2, 1, 99, true)
		de.handleTurnError(errSessionEnded, args, turn, 1, 0, 1)
		assert.NoError(t, args.state.turnErr, "the run must not be marked failed")
	})

	t.Run("but not on the very first turn, even with the flag set", func(t *testing.T) {
		args := newTurnErrorArgs(1, 0, 99, true)
		de.handleTurnError(errSessionEnded, args, turn, 0, 0, 1)
		assert.ErrorIs(t, args.state.turnErr, errSessionEnded,
			"logical turn 0 has produced nothing to accept")
	})

	t.Run("session end past the partial-success floor is partial success", func(t *testing.T) {
		args := newTurnErrorArgs(5, 3, 2, false)
		de.handleTurnError(errSessionEnded, args, turn, 1, 0, 1)
		assert.ErrorIs(t, args.state.turnErr, errPartialSuccess)
	})

	t.Run("session end below the floor is a plain failure", func(t *testing.T) {
		args := newTurnErrorArgs(5, 1, 3, false)
		de.handleTurnError(errSessionEnded, args, turn, 1, 0, 1)
		assert.ErrorIs(t, args.state.turnErr, errSessionEnded)
		assert.NotErrorIs(t, args.state.turnErr, errPartialSuccess)
	})

	t.Run("any other error fails the turn whatever the position", func(t *testing.T) {
		boom := errors.New("provider exploded")
		args := newTurnErrorArgs(2, 5, 0, true)
		de.handleTurnError(boom, args, turn, 1, 0, 1)
		assert.ErrorIs(t, args.state.turnErr, boom,
			"only errSessionEnded gets the lenient treatment")
	})

	t.Run("a mid-iteration session end is not the last turn", func(t *testing.T) {
		// iteration 0 of 3 on the last scenario turn: more iterations follow.
		args := newTurnErrorArgs(2, 4, 99, true)
		de.handleTurnError(errSessionEnded, args, turn, 1, 0, 3)
		assert.ErrorIs(t, args.state.turnErr, errSessionEnded)
	})
}

// TestGetTurnsToExecute_SelfPlayRoleRepeats is the other half of
// TestGetTurnsToExecute_OnlySelfPlayRepeats: with a registry that recognises
// the role, `turns: N` means the persona speaks N times.
func TestGetTurnsToExecute_SelfPlayRoleRepeats(t *testing.T) {
	reg := selfplay.NewRegistry(nil, nil, nil, []arenaconfig.SelfPlayRoleGroup{{ID: "customer"}})
	de := &DuplexConversationExecutor{selfPlayRegistry: reg}

	assert.Equal(t, 4, de.getTurnsToExecute(&arenaconfig.TurnDefinition{Role: "customer", Turns: 4}))
	assert.Equal(t, 1, de.getTurnsToExecute(&arenaconfig.TurnDefinition{Role: "customer"}),
		"turns: 0 still means one turn")
	assert.Equal(t, 1, de.getTurnsToExecute(&arenaconfig.TurnDefinition{Role: "agent", Turns: 4}),
		"a role the registry does not know is not a self-play role")
}

// TestWireSelfplayRubric_IgnoresANonContentGenerator covers the first guard of
// wireSelfplayRubric, which was 0%. The rubric wiring is best-effort decoration
// on top of a generator; a generator of another type must be left alone rather
// than type-asserted into a panic mid-run.
func TestWireSelfplayRubric_IgnoresANonContentGenerator(t *testing.T) {
	de := &DuplexConversationExecutor{}
	// A nil selfPlayRegistry would be dereferenced if the guard did not fire
	// first, so reaching the end of this call is itself the assertion.
	de.wireSelfplayRubric(&fakeSelfplayGenerator{}, nil)
}

// TestProcessSingleDuplexTurn_RejectsUnusableTurns covers the two error exits of
// processSingleDuplexTurn, which sat at 33.3%. Both return before touching the
// pipeline channels. They matter because a duplex turn that produces no audio
// and no text would otherwise leave the provider waiting on input that never
// arrives — the run hangs until the duplex timeout rather than failing with a
// message naming the turn.
func TestProcessSingleDuplexTurn_RejectsUnusableTurns(t *testing.T) {
	de := &DuplexConversationExecutor{}
	ctx := context.Background()

	t.Run("a user turn with neither audio nor text is rejected", func(t *testing.T) {
		err := de.processSingleDuplexTurn(ctx, &ConversationRequest{},
			&arenaconfig.TurnDefinition{Role: "user"}, 2, 0, "", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have audio parts or text content")
		assert.Contains(t, err.Error(), "2", "the error should name the turn index")
	})

	t.Run("a role that is neither user nor a self-play role is rejected", func(t *testing.T) {
		err := de.processSingleDuplexTurn(ctx, &ConversationRequest{},
			&arenaconfig.TurnDefinition{Role: "narrator", Content: "once upon a time"},
			0, 0, "", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported turn role")
		assert.Contains(t, err.Error(), "narrator")
	})
}

// TestTurnHasAudioPart_DetectsAudioAmongParts pins the predicate that routes a
// user turn to the file-streaming path rather than TTS. A false negative sends
// a recorded audio fixture through text-to-speech, which silently tests
// something other than the fixture.
func TestTurnHasAudioPart_DetectsAudioAmongParts(t *testing.T) {
	assert.False(t, turnHasAudioPart(&arenaconfig.TurnDefinition{}))
	assert.False(t, turnHasAudioPart(&arenaconfig.TurnDefinition{
		Parts: []arenaconfig.TurnContentPart{{Type: "text"}},
	}))
	assert.True(t, turnHasAudioPart(&arenaconfig.TurnDefinition{
		Parts: []arenaconfig.TurnContentPart{{Type: "text"}, {Type: turnPartTypeAudio}},
	}))
}
