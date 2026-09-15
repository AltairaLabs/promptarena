package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunIDFromContext_RoundTrips(t *testing.T) {
	ctx := withRunID(context.Background(), "run-abc")
	assert.Equal(t, "run-abc", runIDFromContext(ctx))
}

func TestRunIDFromContext_AbsentIsEmpty(t *testing.T) {
	assert.Equal(t, "", runIDFromContext(context.Background()))
}

func TestRunIDFromContext_InnermostWins(t *testing.T) {
	ctx := withRunID(withRunID(context.Background(), "outer"), "inner")
	assert.Equal(t, "inner", runIDFromContext(ctx))
}
