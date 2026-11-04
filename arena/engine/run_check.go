package engine

import (
	"context"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"

	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
)

// runCheckKeyError is the result detail RunCheck sets when it can't run.
const runCheckKeyError = "error"

// RunCheck runs one check, written as in a scenario's
// conversation_assertions, over a conversation outside a run. It uses the
// engine's judges, classifiers and provider bindings, as a run would, and
// emits no eval events. Judge calibration uses it to score a check against
// labeled examples.
func (e *Engine) RunCheck(
	ctx context.Context, check asrt.AssertionConfig, messages []types.Message, sessionID string,
) asrt.ConversationValidationResult {
	e.quietOnce.Do(func() { e.quietOrch = e.evalOrchestrator.Quiet() })
	results := e.quietOrch.RunAssertionsAsConversationResults(ctx, []asrt.AssertionConfig{check}, messages,
		len(messages)-1, sessionID, evals.TriggerOnConversationComplete)
	if len(results) == 0 {
		return asrt.ConversationValidationResult{
			Type: check.Type, Details: map[string]interface{}{runCheckKeyError: "no eval runner configured"},
		}
	}
	return results[0]
}
