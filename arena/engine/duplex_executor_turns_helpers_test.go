package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/pipeline"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/selfplay"
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
