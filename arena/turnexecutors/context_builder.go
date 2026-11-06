package turnexecutors

import (
	"context"
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

// ConversationContextBuilder owns one conversation's context-builder stage,
// so that state the stage keeps between calls survives from turn to turn.
//
// Arena builds a fresh pipeline every turn. A context-builder stage built with
// it starts empty every turn, and the relevance embedding cache PromptKit keeps
// on the stage (context_policy.relevance.cache_embeddings) never gets a second
// call to serve. The SDK avoids this by building its pipeline once per
// conversation; Arena keeps its per-turn pipeline and reuses only this stage
// (and with it the embedding provider) for the life of the conversation.
//
// One value per conversation. It must never be shared between conversations or
// runs: the cache holds that conversation's message vectors.
type ConversationContextBuilder struct {
	once  sync.Once
	stage *stage.ContextBuilderStage

	// systemPrompt is the stage's own TurnState. The stage reads only
	// SystemPrompt from it, and the per-turn TurnState is rebuilt every turn,
	// so each turn copies its rendered prompt in here before the stage reads
	// it. See conversationContextStage.Process.
	systemPrompt *stage.TurnState

	// mu serializes turns through the stage, so the copy into systemPrompt
	// never races a read by another turn's pipeline.
	mu sync.Mutex
}

// NewConversationContextBuilder returns an empty builder; the stage is built on
// the first turn that has a context policy.
func NewConversationContextBuilder() *ConversationContextBuilder {
	return &ConversationContextBuilder{systemPrompt: stage.NewTurnState()}
}

// stageFor returns this turn's context-builder stage, or nil when the scenario
// has no context policy. The underlying stage is built from the first turn's
// scenario and reused after that; the policy is scenario config, which does not
// change during a conversation.
func (c *ConversationContextBuilder) stageFor(scenario *arenaconfig.Scenario, turnState *stage.TurnState) stage.Stage {
	c.once.Do(func() {
		if policy := buildContextPolicy(scenario); policy != nil {
			c.stage = stage.NewContextBuilderStageWithTurnState(policy, c.systemPrompt)
		}
	})
	if c.stage == nil {
		return nil
	}
	return &conversationContextStage{BaseStage: c.stage.BaseStage, owner: c, turnState: turnState}
}

// contextBuilderStageFor returns the context-builder stage for a turn: the
// conversation's long-lived one when the request carries a
// ConversationContextBuilder, otherwise a new one for this turn only.
func contextBuilderStageFor(req *TurnRequest, turnState *stage.TurnState) stage.Stage {
	if req.ContextBuilder != nil {
		return req.ContextBuilder.stageFor(req.Scenario, turnState)
	}
	if policy := buildContextPolicy(req.Scenario); policy != nil {
		return stage.NewContextBuilderStageWithTurnState(policy, turnState)
	}
	return nil
}

// conversationContextStage runs one turn through the conversation's
// context-builder stage, handing it this turn's system prompt.
type conversationContextStage struct {
	stage.BaseStage
	owner     *ConversationContextBuilder
	turnState *stage.TurnState
}

// Process forwards the turn's elements to the conversation's stage and, once
// upstream has finished, copies this turn's system prompt to where that stage
// reads it.
//
// The copy waits for input to close because upstream stages write
// SystemPrompt until then (instruction stages append after draining their
// input). The context builder accumulates, so it reads the prompt only after
// its own input closes, which happens after the copy.
func (s *conversationContextStage) Process(
	ctx context.Context,
	input <-chan stage.StreamElement,
	output chan<- stage.StreamElement,
) error {
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()

	inner := make(chan stage.StreamElement)
	done := make(chan error, 1)
	go func() { done <- s.owner.stage.Process(ctx, inner, output) }()

	forwardErr := forwardUntilClosed(ctx, input, inner)
	if s.turnState != nil {
		s.owner.systemPrompt.SystemPrompt = s.turnState.SystemPrompt
	}
	close(inner)

	if err := <-done; err != nil {
		return err
	}
	return forwardErr
}

// forwardUntilClosed copies input to out until input closes or ctx ends. On
// cancellation it keeps draining input, so upstream is never left blocked.
func forwardUntilClosed(ctx context.Context, input <-chan stage.StreamElement, out chan<- stage.StreamElement) error {
	var err error
	for elem := range input {
		if err != nil {
			continue
		}
		select {
		case out <- elem:
		case <-ctx.Done():
			err = ctx.Err()
		}
	}
	return err
}
