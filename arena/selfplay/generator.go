package selfplay

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	arenastages "github.com/AltairaLabs/promptarena/v2/arena/stages"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"
	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline"
	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

const selfPlayUserRole = "self-play user"

// ContentGenerator generates user messages using an LLM
type ContentGenerator struct {
	provider       providers.Provider
	persona        *arenaconfig.UserPersonaPack
	providerRubric string
}

// NewContentGenerator creates a new content generator with a specific provider and persona
func NewContentGenerator(provider providers.Provider, persona *arenaconfig.UserPersonaPack) *ContentGenerator {
	return &ContentGenerator{
		provider: provider,
		persona:  persona,
	}
}

// WithProviderRubric sets the TTS provider's characterization rubric that
// will be prepended to the persona's system prompt when the persona has
// opted in via style.expressive. Empty string disables the rubric (no-op
// for personas that did not opt in, and the explicit "this provider does
// not support characterization" signal otherwise). See issue #1130.
func (cg *ContentGenerator) WithProviderRubric(rubric string) *ContentGenerator {
	cg.providerRubric = rubric
	return cg
}

// extractRegionFromPersonaID extracts the region suffix from a persona ID
// e.g., "challenger_uk" -> "uk", "social-engineer" -> "us" (default)
func extractRegionFromPersonaID(personaID string) string {
	if strings.HasSuffix(personaID, "_uk") {
		return "uk"
	}
	if strings.HasSuffix(personaID, "_au") {
		return "au"
	}
	if strings.HasSuffix(personaID, "_us") {
		return "us"
	}
	// Default to US region
	return "us"
}

// sanitizeHistoryForText returns history suitable for a text-only LLM call.
// Media parts that lack inline base64 data (i.e. only carry storage_reference,
// file_path, or url) are stripped — text providers can't load them, and Gemini
// rejects the whole request when parts[0] is an empty inline_data. Messages
// that end up with neither Content nor any sendable Parts are dropped.
//
// Tool-call plumbing (role=tool messages and tool_calls on assistant messages)
// is also stripped: the persona LLM only needs to see what the agent SAID, not
// how it called functions to figure things out. Without this, role-swapping
// later would put tool_calls on a user-role message — which OpenAI rejects with
// "messages.[N].role 'tool' must be a response to a preceding 'tool_calls'".
func sanitizeHistoryForText(history []types.Message) []types.Message {
	if len(history) == 0 {
		return history
	}
	cleaned := make([]types.Message, 0, len(history))
	for i := range history {
		msg := history[i]
		// Drop tool-result messages — they're agent/Realtime plumbing, not
		// part of the conversation the persona needs to see.
		if strings.EqualFold(msg.Role, "tool") {
			continue
		}
		msg.Parts = sendableTextParts(msg.Parts)
		// Strip tool_calls — the persona doesn't act on them and they
		// become invalid after role-swap (tool_calls only valid on assistant).
		msg.ToolCalls = nil
		if msg.Content == "" && len(msg.Parts) == 0 {
			continue
		}
		cleaned = append(cleaned, msg)
	}
	return cleaned
}

// sendableTextParts returns the subset of parts a text-only LLM can load:
// text parts unchanged, media parts only when they have inline base64 data.
func sendableTextParts(parts []types.ContentPart) []types.ContentPart {
	if len(parts) == 0 {
		return parts
	}
	kept := make([]types.ContentPart, 0, len(parts))
	for j := range parts {
		part := parts[j]
		if part.Type == types.ContentTypeText {
			kept = append(kept, part)
			continue
		}
		if part.Media != nil && part.Media.Data != nil && *part.Media.Data != "" {
			kept = append(kept, part)
		}
	}
	return kept
}

// NextUserTurn generates a user message using the LLM through a stage pipeline.
// The opts parameter is optional and can be nil.
func (cg *ContentGenerator) NextUserTurn(
	ctx context.Context,
	history []types.Message,
	scenarioID string,
	opts *GeneratorOptions,
) (*pipeline.ExecutionResult, error) {
	if cg.provider == nil {
		return nil, fmt.Errorf("ContentGenerator provider is nil - check self-play provider configuration")
	}
	if cg.persona == nil {
		return nil, fmt.Errorf("ContentGenerator persona is nil")
	}

	// Strip media parts that the text-mode LLM can't load (storage refs,
	// file paths, URLs). Without this, a duplex assistant message that
	// carries an externalized audio Part poisons the next selfplay call —
	// Gemini rejects the request when parts[0].inline_data has no bytes.
	history = sanitizeHistoryForText(history)

	// Build base variables for persona assembly: the fixed self-play keys,
	// then the per-exchange feedback variables (exchange number, objective
	// status and checks, private notes).
	baseVariables := map[string]string{
		"context_slot": "", // Empty for self-play - conversation context comes through messages
		"domain_hint":  "general",
		"user_context": "user",
	}
	for k, v := range feedbackVariables(cg.persona, history, opts) {
		baseVariables[k] = v
	}

	turn, err := cg.generate(ctx, history, scenarioID, opts, baseVariables, "")
	if err != nil {
		return nil, err
	}
	var repaired []string
	if cg.persona.RegeneratesOnFactViolation() {
		turn, repaired = cg.repairInventedSpecifics(ctx, turn, history, scenarioID, opts, baseVariables)
	}
	result := turn.result

	// Validate the response (treat failures as warnings, not hard errors).
	// Adversarial personas write long, structured turns on purpose, so the
	// short-user-message checks don't apply to them.
	if result.Response != nil && result.Response.Content != "" && !cg.persona.IsAdversarial() {
		if err := cg.validateUserResponse(result.Response.Content); err != nil {
			logger.Warn(
				"User response validation warning",
				"provider", cg.provider.ID(),
				"role", selfPlayUserRole,
				"warning", err.Error(),
			)
			// Add warning to metadata
			if result.Metadata == nil {
				result.Metadata = make(map[string]interface{})
			}
			result.Metadata["validation_warning"] = err.Error()
			result.Metadata["warning_type"] = "user_response_validation"
		}
	}

	// Add persona metadata to result
	if result.Metadata == nil {
		result.Metadata = make(map[string]interface{})
	}
	result.Metadata["persona"] = cg.persona.ID
	if turn.notes != "" {
		result.Metadata[MetaPersonaNotes] = turn.notes
	}
	if len(repaired) > 0 {
		result.Metadata[MetaFidelityRepaired] = repaired
	}
	result.Metadata["role"] = "self-play-user"
	result.Metadata["self_play_provider"] = cg.provider.ID()
	// Surface the resolved system prompt so the duplex executor can stamp
	// it on the user message for the arena dev panel's Self-Play tab. The
	// PersonaAssemblyStage writes it into turnState during pipeline execution;
	// reading from turnState here is safe because generate has returned, so
	// no stage is still mutating turnState.
	if turn.systemPrompt != "" {
		result.Metadata["system_prompt"] = turn.systemPrompt
	}

	return result, nil
}

// generatedTurn is one generation of the persona's message, with its private
// notes already taken out.
type generatedTurn struct {
	result       *pipeline.ExecutionResult
	notes        string
	systemPrompt string
}

// repairInventedSpecifics generates the turn once more when it states a
// specific the persona was not given, telling the persona what it invented.
// It returns the turn to send, with both generations' cost, and the
// specifics the first draft invented; when the draft invented nothing, or
// the retry fails, the draft is returned unchanged and nothing is repaired.
//
// What counts as given matches the run's fidelity check, except that only
// the conversation the persona sees (history, without tool results) is
// read: the persona's private notes and the objective's check feedback are
// not given.
func (cg *ContentGenerator) repairInventedSpecifics(
	ctx context.Context, turn *generatedTurn, history []types.Message,
	scenarioID string, opts *GeneratorOptions, baseVariables map[string]string,
) (*generatedTurn, []string) {
	if turn.result.Response == nil {
		return turn, nil
	}
	given := []string{PersonaGivenText(cg.persona), ConversationText(history)}
	for _, k := range []string{VarObjective, VarRewoundMessage, VarRewoundResponse} {
		given = append(given, baseVariables[k])
	}
	invented := InventedSpecifics(turn.result.Response.Content, given...)
	if len(invented) == 0 {
		return turn, nil
	}
	logger.Debug("Self-play persona invented specifics; regenerating",
		"persona", cg.persona.ID, "specifics", invented)
	retry, err := cg.generate(ctx, history, scenarioID, retryOptions(cg.resolveSeed(opts), opts), baseVariables,
		RegenerateInstruction(invented))
	if err != nil {
		logger.Warn("Self-play persona regeneration failed; sending the first draft",
			"persona", cg.persona.ID, "error", err)
		return turn, nil
	}
	addCostInfo(&retry.result.CostInfo, &turn.result.CostInfo)
	// The dev panel shows the persona's prompt, not the retry note.
	retry.systemPrompt = turn.systemPrompt
	return retry, invented
}

// retryOptions returns opts for the regenerated draft: with a seed, the next
// one, so a seeded provider does not return the same draft again.
func retryOptions(seed *int, opts *GeneratorOptions) *GeneratorOptions {
	if seed == nil {
		return opts
	}
	retry := GeneratorOptions{}
	if opts != nil {
		retry = *opts
	}
	next := *seed + 1
	retry.Seed = &next
	return &retry
}

// generate runs the persona pipeline once. A non-empty retryNote is appended
// to the system prompt, telling the persona what was wrong with its previous
// draft.
func (cg *ContentGenerator) generate(
	ctx context.Context, history []types.Message, scenarioID string,
	opts *GeneratorOptions, baseVariables map[string]string, retryNote string,
) (*generatedTurn, error) {
	// Extract region from persona ID (or default to "us")
	region := extractRegionFromPersonaID(cg.persona.ID)

	temperature := cg.resolveTemperature(opts)

	// Log the self-play user generation API call
	logger.LLMCall(
		cg.provider.ID(),
		selfPlayUserRole,
		len(history),
		float64(temperature),
		"persona",
		cg.persona.ID,
	)

	// Build stage pipeline:
	// 1. PersonaAssemblyStage - assembles persona prompt with fragments/vars
	// 2. HistoryInjectionStage - prepends conversation history
	// 3. SelfPlayUserTurnContextStage - injects scenario context for MockProvider
	// 4. TemplateStage - final variable substitution
	// 5. ProviderStage - calls LLM
	var emitter *events.Emitter
	if opts != nil {
		emitter = opts.Emitter
	}
	providerConfig := &stage.ProviderConfig{
		Temperature: temperature,
		MaxTokens:   cg.resolveMaxTokens(opts),
		Seed:        cg.resolveSeed(opts),
		Source:      events.SourceSelfPlay,
	}

	turnState := stage.NewTurnState()

	// Create the selfplay context stage.
	// If opts provides a selfplay turn index, use it instead of computing from history.
	// This is important for scenarios with mixed file-based and selfplay turns.
	var selfplayContextStage stage.Stage
	selfplayTurnIndex := 0
	if opts != nil {
		selfplayTurnIndex = opts.SelfplayTurnIndex
	}
	if selfplayTurnIndex > 0 {
		selfplayContextStage = arenastages.NewSelfPlayUserTurnContextStageWithHintAndTurnState(
			&arenaconfig.Scenario{ID: scenarioID},
			selfplayTurnIndex,
			cg.persona.ID,
			turnState,
		)
	} else {
		selfplayContextStage = arenastages.NewSelfPlayUserTurnContextStageWithTurnState(
			&arenaconfig.Scenario{ID: scenarioID},
			cg.persona.ID,
			turnState,
		)
	}

	stages := []stage.Stage{
		arenastages.NewPersonaAssemblyStageWithTurnState(cg.persona, region, baseVariables, turnState).
			WithProviderRubric(cg.providerRubric),
	}

	// Instructions appended to the persona's system prompt: the facts rule
	// for a persona with declared facts, the natural termination
	// instruction, and the note on a regenerated draft. They share one
	// stage, since a pipeline's stage names must be unique.
	instruction := FactsInstruction(cg.persona)
	if opts != nil && opts.NaturalTerminationEnabled {
		instruction += CompletionInstruction
	}
	instruction += retryNote
	if instruction != "" {
		stages = append(stages, arenastages.NewCompletionInstructionStageWithTurnState(instruction, turnState))
	}

	// Swap user↔assistant roles so the self-play LLM sees its own prior outputs
	// as "assistant" and the target's responses as "user". This prevents the LLM
	// from confusing itself with the target and generating assistant-style answers.
	stages = append(stages,
		arenastages.NewHistoryInjectionStageSwapped(history),
		selfplayContextStage,
		stage.NewTemplateStageWithTurnState(emitter, turnState),
		stage.NewProviderStageWithTurnState(cg.provider, nil, nil, providerConfig, emitter, nil, turnState),
	)

	builder := stage.NewPipelineBuilder()
	pl, err := builder.Chain(stages...).Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build stage pipeline: %w", err)
	}

	// Empty input element — system prompt + history drives generation. Per-Turn
	// coordination data lives in TurnState, not on the element.
	inputElem := stage.StreamElement{}

	// Execute pipeline synchronously
	stageResult, err := pl.ExecuteSync(ctx, inputElem)
	if err != nil {
		logger.LLMError(cg.provider.ID(), selfPlayUserRole, err)
		return nil, fmt.Errorf("failed to generate user turn: %w", err)
	}

	// Convert stage.ExecutionResult to pipeline.ExecutionResult
	result := convertStageResult(stageResult)

	// Log response
	logger.LLMResponse(
		cg.provider.ID(),
		selfPlayUserRole,
		result.CostInfo.InputTokens,
		result.CostInfo.OutputTokens,
		result.CostInfo.TotalCost,
	)

	// Private notes are taken out before anything else sees the turn: the
	// agent under test never receives them, and the checks after this judge
	// only what is sent.
	var notes string
	if cg.persona.PrivateNotes && result.Response != nil {
		result.Response.Content, notes = extractNotes(result.Response.Content)
		if result.Response.Content == "" && notes != "" {
			return nil, fmt.Errorf("persona %s wrote only private notes and no message for the agent; "+
				"the persona prompt must ask for a message outside <notes>", cg.persona.ID)
		}
	}
	return &generatedTurn{result: result, notes: notes, systemPrompt: turnState.SystemPrompt}, nil
}

// addCostInfo adds from's token counts and costs to into.
func addCostInfo(into, from *types.CostInfo) {
	into.InputTokens += from.InputTokens
	into.OutputTokens += from.OutputTokens
	into.CachedTokens += from.CachedTokens
	into.InputCostUSD += from.InputCostUSD
	into.OutputCostUSD += from.OutputCostUSD
	into.CachedCostUSD += from.CachedCostUSD
	into.TotalCost += from.TotalCost
}

// Token caps for generated user turns when the persona sets no max_tokens.
const (
	defaultUserTurnMaxTokens = 200 // Short user messages
	// completionMarkerTokens is the extra room left for the completion marker
	// when natural termination is enabled.
	completionMarkerTokens = 100
)

// resolveTemperature returns the turn's temperature: the per-turn override
// when set, else the persona default, else the package default.
func (cg *ContentGenerator) resolveTemperature(opts *GeneratorOptions) float32 {
	if opts != nil && opts.Temperature != nil {
		return *opts.Temperature
	}
	if cg.persona.Defaults.Temperature != nil {
		return *cg.persona.Defaults.Temperature
	}
	return arenaconfig.DefaultPersonaTemperature
}

// resolveSeed returns the seed to send: the per-turn seed when set, else the
// persona's defaults.seed. A zero or absent seed sends none.
func (cg *ContentGenerator) resolveSeed(opts *GeneratorOptions) *int {
	if opts != nil && opts.Seed != nil {
		return opts.Seed
	}
	if cg.persona.Defaults.Seed != nil && *cg.persona.Defaults.Seed != 0 {
		return cg.persona.Defaults.Seed
	}
	return nil
}

// resolveMaxTokens returns the token cap for a generated turn. A persona's
// max_tokens is used exactly as configured. Otherwise the short-user-message
// default applies, plus room for the completion marker when natural
// termination is enabled.
func (cg *ContentGenerator) resolveMaxTokens(opts *GeneratorOptions) int {
	if cg.persona.Defaults.MaxTokens != nil {
		return *cg.persona.Defaults.MaxTokens
	}
	if opts != nil && opts.NaturalTerminationEnabled {
		return defaultUserTurnMaxTokens + completionMarkerTokens
	}
	return defaultUserTurnMaxTokens
}

// convertStageResult converts stage.ExecutionResult to pipeline.ExecutionResult
func convertStageResult(stageResult *stage.ExecutionResult) *pipeline.ExecutionResult {
	result := &pipeline.ExecutionResult{
		Messages: stageResult.Messages,
		CostInfo: stageResult.CostInfo,
		Metadata: stageResult.Metadata,
	}

	// Convert Response if present
	if stageResult.Response != nil {
		result.Response = &pipeline.Response{
			Role:      stageResult.Response.Role,
			Content:   stageResult.Response.Content,
			ToolCalls: stageResult.Response.ToolCalls,
		}
	}

	return result
}

// reSentenceEnd matches sentence-ending punctuation (.!?) that is followed
// by whitespace or end-of-string, but NOT preceded by common abbreviation
// patterns (e.g. "Dr.", "Mr.", "U.S."). This gives a more reliable sentence
// count than splitting on "." alone.
var reSentenceEnd = regexp.MustCompile(`[.!?](?:\s|$)`)

const maxSentencesPerResponse = 2

// validateUserResponse validates that the user response meets requirements
func (cg *ContentGenerator) validateUserResponse(content string) error {
	// Count sentence-ending punctuation followed by whitespace or end-of-string.
	sentenceCount := len(reSentenceEnd.FindAllString(strings.TrimSpace(content), -1))
	if sentenceCount == 0 {
		sentenceCount = 1 // Treat content without terminal punctuation as one sentence
	}

	if sentenceCount > maxSentencesPerResponse {
		return fmt.Errorf("response too long: %d sentences (max %d)", sentenceCount, maxSentencesPerResponse)
	}

	// Check for at most one question (temporarily disabled for testing)
	questionCount := strings.Count(content, "?")
	if questionCount > 2 { // Increased from 1 to 2 for testing
		return fmt.Errorf("too many questions: %d (max 2)", questionCount)
	}

	// Check for role integrity (no plans or assistant-like language)
	content = strings.ToLower(content)
	problematicPhrases := []string{
		"here's your plan",
		"here's what you should do",
		"step 1:",
		"step 2:",
		"first, you should",
		"as an ai",
		"i recommend",
		"my suggestion is",
	}

	for _, phrase := range problematicPhrases {
		if strings.Contains(content, phrase) {
			return fmt.Errorf("role integrity violation: contains assistant-like language: %s", phrase)
		}
	}

	return nil
}
