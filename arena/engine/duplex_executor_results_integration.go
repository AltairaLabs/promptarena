package engine

import (
	"context"
	"errors"
	"fmt"

	arenaassertions "github.com/AltairaLabs/promptarena/v2/arena/assertions"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// buildResultFromStateStore loads final state and builds result.
func (de *DuplexConversationExecutor) buildResultFromStateStore(
	ctx context.Context,
	req *ConversationRequest,
) *ConversationResult {
	if req.StateStoreConfig == nil || req.StateStoreConfig.Store == nil {
		return &ConversationResult{
			Messages: []types.Message{},
			Cost:     types.CostInfo{},
		}
	}

	store, ok := req.StateStoreConfig.Store.(statestore.Store)
	if !ok {
		return &ConversationResult{
			Messages: []types.Message{},
			Cost:     types.CostInfo{},
			Failed:   true,
			Error:    "invalid StateStore implementation",
		}
	}

	state, err := store.Load(ctx, req.ConversationID)
	if err != nil && !errors.Is(err, statestore.ErrNotFound) {
		return &ConversationResult{
			Messages: []types.Message{},
			Cost:     types.CostInfo{},
			Failed:   true,
			Error:    fmt.Sprintf("failed to load state: %v", err),
		}
	}

	var messages []types.Message
	if state != nil {
		messages = state.Messages
	}

	turnErr := stampTurnAssertions(ctx, req, store, state)

	totalCost := de.calculateTotalCost(messages)
	mediaOutputs := CollectMediaOutputs(messages)
	toolStats := de.calculateToolStats(messages)

	// Copy any when: eval mounts in now that the agent is done, so the evals
	// below find files the agent never saw.
	if err := deliverEvalFiles(ctx); err != nil {
		return &ConversationResult{Messages: messages, Failed: true, Error: err.Error()}
	}

	// Evaluate conversation-level assertions
	convAssertionResults := de.evaluateConversationAssertions(ctx, req, messages)

	// Run every pack eval the finished conversation should have produced: turn
	// triggers replayed per assistant turn, then session, then conversation
	// triggers, as the text path does. Non-gating observations surface on
	// EvalResults; assertions stay in ConversationAssertionResults.
	var packEvalResults []evals.EvalResult
	if de.evalOrchestrator != nil && de.evalOrchestrator.HasEvals() {
		packEvalResults = de.evalOrchestrator.RunPackEvals(ctx, messages, req.ConversationID)
	}

	return &ConversationResult{
		Messages:                     messages,
		Cost:                         totalCost,
		MediaOutputs:                 mediaOutputs,
		ToolStats:                    toolStats,
		SelfPlay:                     de.containsSelfPlay(req.Scenario),
		PersonaID:                    de.findFirstSelfPlayPersona(req.Scenario),
		ConversationAssertionResults: convAssertionResults,
		EvalResults:                  packEvalResults,
		Failed:                       turnErr != "",
		Error:                        turnErr,
	}
}

// stampTurnAssertions writes each duplex turn's assertion outcome onto its
// reply in the final transcript and returns the run error, "" when every turn
// assertion passed.
//
// The save stage rewrites the transcript on every save, so the outcomes are
// stamped here, after the session has finished saving, not in the store
// mid-session. The run result is read back from the store, so the stamped
// transcript is written there too. A failing turn assertion fails the run, as
// it does on the text path, where it surfaces as the turn's error.
func stampTurnAssertions(
	ctx context.Context, req *ConversationRequest, store statestore.Store, state *statestore.ConversationState,
) string {
	if state == nil || !req.duplexTurnAssertions.apply(state.Messages) {
		return ""
	}
	if writer, ok := store.(statestore.BulkWriter); ok {
		if err := writer.Save(ctx, state); err != nil {
			return fmt.Sprintf("failed to save turn assertion results: %v", err)
		}
	}
	total, failed := arenaassertions.CountTurnAssertions(state.Messages)
	if failed == 0 {
		return ""
	}
	return fmt.Sprintf("turn assertions failed: %d of %d", failed, total)
}

// calculateToolStats aggregates tool usage statistics from messages.
func (de *DuplexConversationExecutor) calculateToolStats(messages []types.Message) *types.ToolStats {
	toolStats := &types.ToolStats{
		TotalCalls: 0,
		ByTool:     make(map[string]int),
	}

	for i := range messages {
		for j := range messages[i].ToolCalls {
			toolStats.TotalCalls++
			toolStats.ByTool[messages[i].ToolCalls[j].Name]++
		}
	}

	if toolStats.TotalCalls == 0 {
		return nil
	}

	return toolStats
}

// getConversationHistory retrieves the conversation history from state store.
func (de *DuplexConversationExecutor) getConversationHistory(
	ctx context.Context, req *ConversationRequest,
) []types.Message {
	if req.StateStoreConfig == nil || req.StateStoreConfig.Store == nil {
		return nil
	}

	store, ok := req.StateStoreConfig.Store.(statestore.Store)
	if !ok {
		return nil
	}

	state, err := store.Load(ctx, req.ConversationID)
	if err != nil {
		return nil
	}

	if state == nil {
		return nil
	}

	return state.Messages
}
