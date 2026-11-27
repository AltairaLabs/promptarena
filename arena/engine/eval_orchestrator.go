package engine

import (
	"context"

	"github.com/AltairaLabs/promptarena/v2/arena/agentkb"
	"github.com/AltairaLabs/promptarena/v2/arena/assertions"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/inference"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// WorkflowMetadataProvider is implemented by types that provide workflow state
// metadata for injection into the eval context during assertion evaluation.
type WorkflowMetadataProvider interface {
	WorkflowMetadata() map[string]any
}

// CompositionMetadataProvider is implemented by types that provide per-turn
// composition observability (step outputs, branch-taken, parallel status) for
// injection into the eval context, so composition_* assertions can target
// intermediate steps. Set per-run for composition scenarios (RFC 0010).
type CompositionMetadataProvider interface {
	CompositionMetadata() map[string]any
}

// EvalOrchestrator orchestrates eval and assertion execution during Arena runs.
type EvalOrchestrator struct {
	runner               *evals.EvalRunner
	defs                 []evals.EvalDef
	taskType             string
	metadata             map[string]any           // injected into every EvalContext (e.g. judge_targets)
	eventBus             events.Bus               // optional event bus for provider call telemetry in evals
	workflowMetaProvider WorkflowMetadataProvider // optional workflow state provider
	// compositionMetaProvider, when set (per-run), injects composition step
	// observability into every EvalContext for composition_* assertions.
	compositionMetaProvider CompositionMetadataProvider
	// inferenceRegistry, when non-nil, is attached to the context passed to
	// every eval/assertion handler invocation via inference.WithRegistry.
	// A classifier-backed check (topic_policy, audio_emotion, the text
	// classifiers, ...) that names no provider resolves the registry's default
	// from it. Nil is fine: such a check then reports itself skipped.
	inferenceRegistry *inference.Registry
	// providerBinding, when non-nil, is attached alongside the inference
	// registry so a check that names a LOGICAL provider from its pack's
	// requires block resolves the same way here as it does inside the
	// pipeline. Nil is fine — the runtime reports "no binding configured",
	// which is a different fix from "you named something unbound".
	providerBinding evals.ProviderBinding
}

// NewEvalOrchestrator creates a hook for executing pack evals during Arena runs.
// If skipEvals is true, the runner is nil and all methods are no-ops.
// The evalTypeFilter, when non-empty, restricts execution to matching eval types.
func NewEvalOrchestrator(
	registry *evals.EvalTypeRegistry,
	defs []evals.EvalDef,
	skipEvals bool,
	evalTypeFilter []string,
	taskType string,
) *EvalOrchestrator {
	// Filter defs by eval type if filter is set
	filteredDefs := filterEvalDefs(defs, evalTypeFilter)

	var runner *evals.EvalRunner
	if !skipEvals {
		runner = evals.NewEvalRunner(registry)
	}

	return &EvalOrchestrator{
		runner:   runner,
		defs:     filteredDefs,
		taskType: taskType,
	}
}

// Clone creates a shallow copy suitable for per-run use. The runner and defs
// are shared (immutable after construction), but metadata and workflow provider
// are independent. This avoids data races when concurrent runs set different
// workflow metadata providers.
func (h *EvalOrchestrator) Clone() *EvalOrchestrator {
	if h == nil {
		return nil
	}
	clone := *h
	clone.workflowMetaProvider = nil
	clone.compositionMetaProvider = nil
	// Clone the runner so the emitter is independent per run.
	// The emitter is wired per-call in buildEvalContext with proper session IDs.
	if h.runner != nil {
		clone.runner = h.runner.Clone()
	}
	// Copy metadata map so per-run mutations don't affect the original
	if h.metadata != nil {
		clone.metadata = make(map[string]any, len(h.metadata))
		for k, v := range h.metadata {
			clone.metadata[k] = v
		}
	}
	return &clone
}

// Quiet returns a copy of the orchestrator that emits no events: no
// eval.completed/failed events and no judge provider-call telemetry. It keeps
// the metadata and workflow/composition providers, so assertions see the same
// context. Used for intermediate checks whose results are not themselves
// reported as evals, such as self-play objective checks between exchanges.
func (h *EvalOrchestrator) Quiet() *EvalOrchestrator {
	if h == nil {
		return nil
	}
	quiet := *h
	quiet.eventBus = nil
	if h.runner != nil {
		quiet.runner = h.runner.Clone()
		quiet.runner.SetEmitter(nil)
	}
	return &quiet
}

// SetMetadata sets metadata that will be injected into every EvalContext.
// Used to pass judge_targets, prompt_registry, and other config to eval handlers.
func (h *EvalOrchestrator) SetMetadata(metadata map[string]any) {
	if h == nil {
		return
	}
	h.metadata = metadata
}

// SetInferenceRegistry sets the inference.Registry attached to the context
// passed to every eval/assertion handler. Safe to call with nil.
func (h *EvalOrchestrator) SetInferenceRegistry(r *inference.Registry) {
	if h == nil {
		return
	}
	h.inferenceRegistry = r
}

// SetProviderBinding sets the binding that resolves the logical provider names
// a pack's checks declare. Safe to call with nil.
func (h *EvalOrchestrator) SetProviderBinding(b evals.ProviderBinding) {
	if h == nil {
		return
	}
	h.providerBinding = b
}

// withClassify returns ctx wrapped with the orchestrator's inference.Registry
// and provider binding, or the original ctx when neither is configured. Called
// from every Run* entry point so a handler's inference.FromContext and
// evals.BindingFromContext lookups succeed without each caller plumbing them
// through manually.
func (h *EvalOrchestrator) withClassify(ctx context.Context) context.Context {
	if h == nil {
		return ctx
	}
	if h.inferenceRegistry != nil {
		ctx = inference.WithRegistry(ctx, h.inferenceRegistry)
	}
	return evals.WithProviderBinding(ctx, h.providerBinding)
}

// SetEventBus configures the event bus for provider call telemetry in eval handlers.
// When set, an emitter is injected into each EvalContext's metadata so that
// LLM judge provider calls emit ProviderCallStarted/Completed/Failed events.
func (h *EvalOrchestrator) SetEventBus(bus events.Bus) {
	if h == nil {
		return
	}
	h.eventBus = bus
}

// HasEvals returns true if there are eval defs to execute.
func (h *EvalOrchestrator) HasEvals() bool {
	if h == nil {
		return false
	}
	return len(h.defs) > 0
}

// RunTurnEvals runs turn-triggered evals after a turn completes.
// Returns converted ConversationValidationResult entries.
func (h *EvalOrchestrator) RunTurnEvals(
	ctx context.Context,
	messages []types.Message,
	turnIndex int,
	sessionID string,
) []assertions.ConversationValidationResult {
	if h == nil || !h.HasEvals() || h.runner == nil {
		return nil
	}

	evalCtx := withArenaEvalMetadata(ctx, h.buildEvalContext(messages, turnIndex, sessionID))
	results := h.runner.RunTurnEvals(h.withClassify(ctx), h.defs, evalCtx)
	return assertions.ConvertEvalResults(results)
}

// RunSessionEvals runs session-complete evals after conversation
// finishes and returns the runtime's raw EvalResult values. Pack-level
// evals are observations (no pass/fail), so arena passes them through
// to the report unchanged — same shape that would surface in
// production. Use RunSessionAssertions for arena-only test assertions
// that do go through the gating-shaped converter.
func (h *EvalOrchestrator) RunSessionEvals(
	ctx context.Context,
	messages []types.Message,
	sessionID string,
) []evals.EvalResult {
	if h == nil || !h.HasEvals() || h.runner == nil {
		return nil
	}

	turnIndex := len(messages) - 1
	if turnIndex < 0 {
		turnIndex = 0
	}
	evalCtx := withArenaEvalMetadata(ctx, h.buildEvalContext(messages, turnIndex, sessionID))
	return h.runner.RunSessionEvals(h.withClassify(ctx), h.defs, evalCtx)
}

// RunPackEvals runs every pack eval a finished conversation should have
// produced, in trigger order: the turn triggers (every_turn, sample_turns)
// once for each completed assistant turn, then the session triggers
// (on_session_complete, sample_sessions), then on_conversation_complete.
//
// Turn evals are replayed over the transcript rather than fired from inside
// the turn pipeline: each one sees the messages up to and including the
// assistant reply that closed its turn, which is what it would have seen at
// that moment, and its TurnIndex is that reply's message index. A turn ends at
// an assistant message that is the last message or is followed by a user
// message, so the intermediate assistant messages of a tool loop are not
// scored as turns of their own.
//
// Like RunSessionEvals, the results are the runtime's raw observations.
func (h *EvalOrchestrator) RunPackEvals(
	ctx context.Context,
	messages []types.Message,
	sessionID string,
) []evals.EvalResult {
	if h == nil || !h.HasEvals() || h.runner == nil {
		return nil
	}

	classifyCtx := h.withClassify(ctx)
	var results []evals.EvalResult
	for _, end := range turnEndIndexes(messages) {
		turn := messages[:end+1]
		evalCtx := withArenaEvalMetadata(ctx, h.buildEvalContext(turn, end, sessionID))
		results = append(results, h.runner.RunTurnEvals(classifyCtx, h.defs, evalCtx)...)
	}

	results = append(results, h.RunSessionEvals(ctx, messages, sessionID)...)

	turnIndex := max(len(messages)-1, 0)
	evalCtx := withArenaEvalMetadata(ctx, h.buildEvalContext(messages, turnIndex, sessionID))
	return append(results, h.runner.RunConversationEvals(classifyCtx, h.defs, evalCtx)...)
}

// turnEndIndexes returns the index of each assistant message that closes a
// turn: one that is the last message, or is followed by a user message.
func turnEndIndexes(messages []types.Message) []int {
	var ends []int
	for i := range messages {
		if messages[i].Role != roleAssistant {
			continue
		}
		if i == len(messages)-1 || messages[i+1].Role == roleUser {
			ends = append(ends, i)
		}
	}
	return ends
}

// RunConversationEvals runs conversation-complete evals after all turns finish.
// Returns converted ConversationValidationResult entries.
func (h *EvalOrchestrator) RunConversationEvals(
	ctx context.Context,
	messages []types.Message,
	sessionID string,
) []assertions.ConversationValidationResult {
	if h == nil || !h.HasEvals() || h.runner == nil {
		return nil
	}

	turnIndex := len(messages) - 1
	if turnIndex < 0 {
		turnIndex = 0
	}
	evalCtx := withArenaEvalMetadata(ctx, h.buildEvalContext(messages, turnIndex, sessionID))
	results := h.runner.RunConversationEvals(h.withClassify(ctx), h.defs, evalCtx)
	return assertions.ConvertEvalResults(results)
}

// RunAssertionsAsEvals converts assertion configs to EvalDefs and runs them
// through the runner. Returns raw EvalResults (not converted to assertion format).
// The trigger parameter overrides the default trigger on each converted def.
//
// Each assertion is converted to an EvalDef with type "assertion", which the
// runner dispatches to AssertionEvalHandler. The wrapper resolves the inner
// eval handler from the registry, executes it, and applies min_score/max_score
// thresholds to determine pass/fail.
func (h *EvalOrchestrator) RunAssertionsAsEvals(
	ctx context.Context,
	assertionConfigs []assertions.AssertionConfig,
	messages []types.Message,
	turnIndex int,
	sessionID string,
	trigger evals.EvalTrigger,
) []evals.EvalResult {
	if h == nil || h.runner == nil || len(assertionConfigs) == 0 {
		return nil
	}

	defs := make([]evals.EvalDef, len(assertionConfigs))
	for i, cfg := range assertionConfigs {
		defs[i] = assertions.ToEvalDef(cfg, i)
		defs[i].Trigger = string(trigger)
	}

	evalCtx := withArenaEvalMetadata(ctx, h.buildEvalContext(messages, turnIndex, sessionID))
	classifyCtx := h.withClassify(ctx)

	if trigger == evals.TriggerOnConversationComplete {
		return h.runner.RunConversationEvals(classifyCtx, defs, evalCtx)
	}
	return h.runTurnAssertions(classifyCtx, assertionConfigs, defs, evalCtx, messages)
}

// runTurnAssertions runs a turn's assertions. A turn-level assertion judges
// this turn, so it sees only the tool calls made since the turn's user
// message, its `when` gate included: the history holds every earlier turn's
// calls too. A session-level one placed on a turn (tools_called_session, say)
// still sees the whole session, as does a type evalmeta does not describe.
// Results come back in the assertions' order.
func (h *EvalOrchestrator) runTurnAssertions(
	ctx context.Context,
	cfgs []assertions.AssertionConfig,
	defs []evals.EvalDef,
	evalCtx *evals.EvalContext,
	messages []types.Message,
) []evals.EvalResult {
	turnCtx := withCurrentTurnToolCalls(evalCtx, messages)
	var turnDefs, sessionDefs []evals.EvalDef
	for i := range defs {
		if agentkb.EvalLevel(assertedEvalType(&cfgs[i])) == agentkb.EvalLevelTurn {
			turnDefs = append(turnDefs, defs[i])
		} else {
			sessionDefs = append(sessionDefs, defs[i])
		}
	}
	if len(sessionDefs) == 0 {
		return h.runner.RunTurnEvals(ctx, defs, turnCtx)
	}
	if len(turnDefs) == 0 {
		return h.runner.RunTurnEvals(ctx, defs, evalCtx)
	}
	byID := make(map[string]evals.EvalResult, len(defs))
	ran := append(h.runner.RunTurnEvals(ctx, turnDefs, turnCtx), h.runner.RunTurnEvals(ctx, sessionDefs, evalCtx)...)
	for i := range ran {
		byID[ran[i].EvalID] = ran[i]
	}
	results := make([]evals.EvalResult, 0, len(defs))
	for i := range defs {
		if r, ok := byID[defs[i].ID]; ok {
			results = append(results, r)
		}
	}
	return results
}

// assertedEvalType is the eval an assertion runs: its type, or for an
// explicit `type: assertion` wrapper, the inner eval_type.
func assertedEvalType(cfg *assertions.AssertionConfig) string {
	if cfg.Type == evals.WrapperTypeAssertion {
		if inner, ok := cfg.Params["eval_type"].(string); ok {
			return inner
		}
	}
	return cfg.Type
}

// withCurrentTurnToolCalls returns a copy of evalCtx whose ToolCalls are only
// those made at or after the last user message of messages: the current
// turn's. evalCtx is returned as-is when it holds none from earlier turns.
func withCurrentTurnToolCalls(evalCtx *evals.EvalContext, messages []types.Message) *evals.EvalContext {
	start := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == roleUser {
			start = i
			break
		}
	}
	if start == 0 || len(evalCtx.ToolCalls) == 0 {
		return evalCtx
	}
	var current []evals.ToolCallRecord
	for i := range evalCtx.ToolCalls {
		if evalCtx.ToolCalls[i].TurnIndex >= start {
			current = append(current, evalCtx.ToolCalls[i])
		}
	}
	if len(current) == len(evalCtx.ToolCalls) {
		return evalCtx
	}
	scoped := *evalCtx
	scoped.ToolCalls = current
	return &scoped
}

// RunAssertionsAsConversationResults converts assertion configs to EvalDefs,
// runs them through the runner, and wraps results in ConversationValidationResult.
// The results use the original assertion type names (not pack_eval: prefixed).
func (h *EvalOrchestrator) RunAssertionsAsConversationResults(
	ctx context.Context,
	assertionConfigs []assertions.AssertionConfig,
	messages []types.Message,
	turnIndex int,
	sessionID string,
	trigger evals.EvalTrigger,
) []assertions.ConversationValidationResult {
	if h == nil {
		return nil
	}
	results := h.RunAssertionsAsEvals(ctx, assertionConfigs, messages, turnIndex, sessionID, trigger)
	converted := assertions.ConvertEvalResults(results)
	// Restore original assertion type names — ConvertEvalResults adds pack_eval:
	// prefix which is only appropriate for pack-defined evals, not scenario assertions.
	// Also surface the assertion's configured message (e.g. "Gate 1: kit is
	// schema-valid") as the headline, so reports label each assertion meaningfully
	// instead of showing the handler's generic explanation. The handler explanation
	// is preserved under Details["explanation"].
	for i := range converted {
		if i >= len(assertionConfigs) {
			continue
		}
		converted[i].Type = assertionConfigs[i].Type
		if assertionConfigs[i].Message == "" {
			continue
		}
		if converted[i].Message != "" {
			if converted[i].Details == nil {
				converted[i].Details = map[string]interface{}{}
			}
			converted[i].Details["explanation"] = converted[i].Message
		}
		converted[i].Message = assertionConfigs[i].Message
	}
	return converted
}

// SetWorkflowMetadataProvider sets the workflow state provider for eval context injection.
// Called per-run for workflow scenarios so assertions can access the current workflow state.
func (h *EvalOrchestrator) SetWorkflowMetadataProvider(provider WorkflowMetadataProvider) {
	if h == nil {
		return
	}
	h.workflowMetaProvider = provider
}

// SetCompositionMetadataProvider sets the composition observability provider for
// eval context injection. Called per-run for composition scenarios so
// composition_* assertions can access per-step outputs/branch/parallel status.
func (h *EvalOrchestrator) SetCompositionMetadataProvider(provider CompositionMetadataProvider) {
	if h == nil {
		return
	}
	h.compositionMetaProvider = provider
}

// buildEvalContext constructs an EvalContext from Arena messages.
// Delegates to the shared evals.BuildEvalContext helper.
// If an event bus is configured, an emitter is injected into metadata
// so LLM judge handlers can emit provider call telemetry.
// If a workflow metadata provider is set, workflow state is injected
// so workflow assertions (state_is, transitioned_to, workflow_complete) work.
func (h *EvalOrchestrator) buildEvalContext(
	messages []types.Message,
	turnIndex int,
	sessionID string,
) *evals.EvalContext {
	latencyMs, haveLatency := latestAssistantLatencyMs(messages)
	metadata := h.metadata
	if h.eventBus != nil || h.workflowMetaProvider != nil || h.compositionMetaProvider != nil || haveLatency {
		metadata = h.mergeEvalMetadata(sessionID, latencyMs, haveLatency)
	}
	return evals.BuildEvalContext(messages, turnIndex, sessionID, h.taskType, metadata)
}

// mergeEvalMetadata builds a per-invocation copy of the orchestrator metadata,
// overlaying the emitter (wired into the runner), workflow state, composition
// state, and per-turn provider latency. The base h.metadata map is never
// mutated so concurrent invocations stay isolated.
func (h *EvalOrchestrator) mergeEvalMetadata(sessionID string, latencyMs int64, haveLatency bool) map[string]any {
	merged := make(map[string]any, len(h.metadata)+4) //nolint:mnd // extra capacity for emitter + workflow + latency keys
	for k, v := range h.metadata {
		merged[k] = v
	}
	if h.eventBus != nil {
		emitter := events.NewEmitter(h.eventBus, sessionID, sessionID, sessionID)
		merged["emitter"] = emitter
		// Wire the emitter into the runner for eval.completed/failed events.
		// Safe: each concurrent run gets its own orchestrator clone (and runner).
		if h.runner != nil {
			h.runner.SetEmitter(emitter)
		}
	}
	if h.workflowMetaProvider != nil {
		for k, v := range h.workflowMetaProvider.WorkflowMetadata() {
			merged[k] = v
		}
	}
	if h.compositionMetaProvider != nil {
		for k, v := range h.compositionMetaProvider.CompositionMetadata() {
			merged[k] = v
		}
	}
	if haveLatency {
		// Bridge per-turn provider latency from the assistant message
		// into metadata so the latency_budget assertion can read it.
		merged["latency_ms"] = float64(latencyMs)
	}
	return merged
}

// latestAssistantLatencyMs returns the LatencyMs of the most recent
// assistant message, and a flag indicating whether one was found.
// The boolean lets callers distinguish a measured 0 ms (sub-millisecond
// in-process mock) from "no latency available". Used to bridge per-turn
// provider latency into the eval metadata for the latency_budget
// assertion.
func latestAssistantLatencyMs(messages []types.Message) (int64, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == roleAssistant {
			return messages[i].LatencyMs, true
		}
	}
	return 0, false
}

// filterEvalDefs filters eval defs to only include types in the filter list.
// If the filter is empty, all defs are returned.
func filterEvalDefs(defs []evals.EvalDef, filter []string) []evals.EvalDef {
	if len(filter) == 0 {
		return defs
	}

	allowed := make(map[string]bool, len(filter))
	for _, t := range filter {
		allowed[t] = true
	}

	var filtered []evals.EvalDef
	for i := range defs {
		if allowed[defs[i].Type] {
			filtered = append(filtered, defs[i])
		}
	}
	return filtered
}
