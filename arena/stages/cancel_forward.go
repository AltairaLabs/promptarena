package stages

import (
	"context"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
)

// canceledForwardTimeout bounds each send once the turn's context is done. A
// variable so tests need not wait it out.
var canceledForwardTimeout = 500 * time.Millisecond

// cancelForwarder forwards a pass-through stage's elements so a canceled turn
// still delivers what it produced — the partial reply and the error the
// provider stage emits after the cancellation. While ctx is live a send is an
// ordinary cancellable one. After it is done, each element is offered for a
// short time: a consumer collecting the result still reads, and one that has
// gone away must not block the pipeline, so the first send that times out
// stops all further forwarding.
//
// It mirrors the runtime's stage-internal forwarder of the same name.
type cancelForwarder struct {
	gone bool
}

func (f *cancelForwarder) forward(ctx context.Context, output chan<- stage.StreamElement, elem stage.StreamElement) {
	if f.gone {
		return
	}
	if ctx.Err() == nil {
		select {
		case output <- elem:
			return
		case <-ctx.Done():
		}
	}
	timer := time.NewTimer(canceledForwardTimeout)
	defer timer.Stop()
	select {
	case output <- elem:
	case <-timer.C:
		f.gone = true
	}
}
