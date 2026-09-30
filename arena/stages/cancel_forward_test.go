package stages

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
)

func TestCancelForwarder_LiveContextForwards(t *testing.T) {
	out := make(chan stage.StreamElement, 1)
	var fwd cancelForwarder
	text := "hi"
	fwd.forward(context.Background(), out, stage.StreamElement{Text: &text})
	require.Len(t, out, 1)
	assert.Equal(t, "hi", *(<-out).Text)
}

func TestCancelForwarder_CanceledStillDeliversToAReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := make(chan stage.StreamElement)
	got := make(chan stage.StreamElement, 1)
	go func() { got <- <-out }()

	var fwd cancelForwarder
	text := "partial"
	fwd.forward(ctx, out, stage.StreamElement{Text: &text})

	assert.Equal(t, "partial", *(<-got).Text)
	assert.False(t, fwd.gone)
}

func TestCancelForwarder_StopsOnceConsumerIsGone(t *testing.T) {
	prev := canceledForwardTimeout
	canceledForwardTimeout = 5 * time.Millisecond
	t.Cleanup(func() { canceledForwardTimeout = prev })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := make(chan stage.StreamElement) // never read

	var fwd cancelForwarder
	fwd.forward(ctx, out, stage.StreamElement{})
	require.True(t, fwd.gone, "an unread send after cancellation must mark the consumer gone")

	canceledForwardTimeout = time.Hour
	done := make(chan struct{})
	go func() {
		fwd.forward(ctx, out, stage.StreamElement{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("forward kept waiting after the consumer was known to be gone")
	}
}
