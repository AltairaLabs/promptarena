// Package toolresult rewrites tool results before the agent under test sees
// them, for a scenario's tool_result_transforms. It is a PromptKit tool hook
// that also implements hooks.ToolResultRewriter, so it works on live HTTP,
// MCP and mock tools alike.
package toolresult

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/v2/hooks"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"
)

// Run is what a rewrite knows about the run: where exec commands resolve,
// and what seeds the random converters.
type Run struct {
	ConfigDir string
	Scenario  string
	Trial     int
	Seed      int // defaults.seed
}

// Hook rewrites the results of the tools its rules name.
type Hook struct {
	rules []arenaconfig.ToolResultTransform
	run   Run
}

// NewHook returns a hook for rules, or nil when there are none.
func NewHook(rules []arenaconfig.ToolResultTransform, run Run) *Hook {
	if len(rules) == 0 {
		return nil
	}
	return &Hook{rules: rules, run: run}
}

// Name returns the hook name.
func (h *Hook) Name() string { return "tool-result-transforms" }

// BeforeExecution allows every call.
func (h *Hook) BeforeExecution(context.Context, hooks.ToolRequest) hooks.Decision { return hooks.Allow }

// AfterExecution allows every call; the rewrite happens in RewriteResult.
func (h *Hook) AfterExecution(context.Context, hooks.ToolRequest, hooks.ToolResponse) hooks.Decision {
	return hooks.Allow
}

// RewriteResult runs every rule for the tool, in order, over its result.
func (h *Hook) RewriteResult(ctx context.Context, req hooks.ToolRequest, content string) (string, error) {
	for i := range h.rules {
		r := &h.rules[i]
		if r.Tool != req.Name {
			continue
		}
		out, err := selfplay.ApplyTransforms(ctx, r.Transforms, h.run.ConfigDir, selfplay.TransformRequest{
			Message: content, Scenario: h.run.Scenario, Trial: h.run.Trial, Seed: h.run.Seed,
			Tool: req.Name, ToolArgs: req.Args, ToolCallID: req.CallID,
		})
		if err != nil {
			err = fmt.Errorf("tool_result_transforms[%d] on %s: %w", i, req.Name, err)
			recordFailure(ctx, err)
			return "", err
		}
		content = out
	}
	return content, nil
}

type failureLogKey struct{}

// failureLog collects the rewrites that failed in a run.
type failureLog struct {
	mu   sync.Mutex
	errs []error
}

// WithFailureLog returns a context that collects failed rewrites, and a
// function that returns them joined, or nil when none failed. A failed
// rewrite reaches the agent as a tool error, and the run goes on without the
// planted content, so the run must fail on it rather than pass a test that
// never ran.
func WithFailureLog(ctx context.Context) (context.Context, func() error) {
	log := &failureLog{}
	return context.WithValue(ctx, failureLogKey{}, log), func() error {
		log.mu.Lock()
		defer log.mu.Unlock()
		return errors.Join(log.errs...)
	}
}

func recordFailure(ctx context.Context, err error) {
	if log, ok := ctx.Value(failureLogKey{}).(*failureLog); ok {
		log.mu.Lock()
		log.errs = append(log.errs, err)
		log.mu.Unlock()
	}
}
