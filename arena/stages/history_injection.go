// Package stages provides arena-specific pipeline stages for test execution.
package stages

import (
	"context"
	"strings"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// HistoryInjectionStage prepends conversation history to the pipeline stream.
// This is useful for self-play scenarios where history needs to be explicitly provided
// before the LLM generates the next user message.
//
// The stage emits all history messages first, then forwards any incoming elements.
//
// When swapRoles is true, user↔assistant roles are swapped so that the self-play
// LLM sees its own prior outputs as "assistant" and the target's responses as "user".
// This prevents the self-play LLM from confusing itself with the target assistant.
type HistoryInjectionStage struct {
	stage.BaseStage
	history   []types.Message
	swapRoles bool
}

// NewHistoryInjectionStage creates a new history injection stage.
func NewHistoryInjectionStage(history []types.Message) *HistoryInjectionStage {
	return &HistoryInjectionStage{
		BaseStage: stage.NewBaseStage("history_injection", stage.StageTypeTransform),
		history:   history,
	}
}

// NewHistoryInjectionStageSwapped creates a history injection stage that swaps
// user↔assistant roles. Use this for self-play generation so the LLM sees the
// conversation from its own perspective (its prior outputs as "assistant").
func NewHistoryInjectionStageSwapped(history []types.Message) *HistoryInjectionStage {
	return &HistoryInjectionStage{
		BaseStage: stage.NewBaseStage("history_injection", stage.StageTypeTransform),
		history:   history,
		swapRoles: true,
	}
}

// Process emits history messages first, then forwards all incoming elements.
//
//nolint:lll // Channel signature cannot be shortened
func (s *HistoryInjectionStage) Process(ctx context.Context, input <-chan stage.StreamElement, output chan<- stage.StreamElement) error {
	defer close(output)

	// First, emit all history messages as StreamElements
	for i := range s.history {
		msg := s.history[i] // avoid copying in loop
		if s.swapRoles {
			msg = swapMessageRole(&s.history[i])
		}
		elem := stage.StreamElement{
			Message: &msg,
		}

		select {
		case output <- elem:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Then forward all incoming elements
	for elem := range input {
		select {
		case output <- elem:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// blockedReplyBeforeMetaKey mirrors PromptKit's internal
// history.BlockedReplyBeforeMetaKey: on a user message, it marks the reply
// before it as part of a blocked input turn.
const blockedReplyBeforeMetaKey = "_blocked_input_reply_before"

// swapMessageRole returns a shallow copy of the message with user↔assistant swapped.
// System messages are left unchanged.
//
// The blocked-input marks are dropped from the copy. They tell the provider
// stage to keep a guardrail-blocked turn out of the agent's model context, but
// the self-play user is the person who was shown that turn: its canned reply
// is the last thing they read. Swapped and still marked, the reply would be a
// marked "user" message, which the provider stage leaves out, and the persona
// would answer its own message.
func swapMessageRole(msg *types.Message) types.Message {
	swapped := *msg
	switch strings.ToLower(msg.Role) {
	case roleUser:
		swapped.Role = "assistant"
	case "assistant":
		swapped.Role = roleUser
	}
	swapped.Meta = withoutBlockedInputMarks(msg.Meta)
	return swapped
}

// withoutBlockedInputMarks returns meta without the blocked-input marks, as a
// copy when it has to remove one, so the caller's history is not written.
func withoutBlockedInputMarks(meta map[string]interface{}) map[string]interface{} {
	_, blocked := meta[types.MetaBlockedInput]
	_, before := meta[blockedReplyBeforeMetaKey]
	if !blocked && !before {
		return meta
	}
	out := make(map[string]interface{}, len(meta))
	for k, v := range meta {
		if k != types.MetaBlockedInput && k != blockedReplyBeforeMetaKey {
			out[k] = v
		}
	}
	return out
}
