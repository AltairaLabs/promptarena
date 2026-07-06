package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/PromptKit/runtime/logger"
	"github.com/AltairaLabs/PromptKit/runtime/pipeline"
	"github.com/AltairaLabs/PromptKit/runtime/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/tools"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	arenaaudio "github.com/AltairaLabs/promptarena/arena/audio"
	arenastages "github.com/AltairaLabs/promptarena/arena/stages"
	arenastore "github.com/AltairaLabs/promptarena/arena/statestore"
)

// defaultPipelineIdleTimeout is the default duplex pipeline IdleTimeout when a
// scenario does not declare its own duplex.timeout. Long enough to avoid
// premature cancellation of multi-turn conversations.
const defaultPipelineIdleTimeout = 30 * time.Second

// bargeInChannelBufferSize is the inter-stage channel buffer used for barge-in
// sessions. Small so only a few hundred ms of response audio can sit between the
// provider stage and the real-time output pacing stage; the session's unbounded
// pump queue (dropped on barge-in) absorbs the provider's faster-than-real-time
// audio, so this doesn't starve normal playback.
const bargeInChannelBufferSize = 2

// executeDuplexConversation handles the main duplex conversation logic.
// The pipeline is the single source of truth: PromptAssemblyStage loads the prompt,
// then DuplexProviderStage creates the session using system_prompt from metadata.
//
// This method implements retry logic for recoverable errors (session drops, network issues).
// On failure, it waits for retry_delay_ms and creates a fresh pipeline/session.
func (de *DuplexConversationExecutor) executeDuplexConversation(
	ctx context.Context,
	req *ConversationRequest,
	streamProvider providers.StreamInputSupport,
	emitter *events.Emitter,
) *ConversationResult {
	de.emitSessionStarted(emitter, req)

	// Get retry configuration from scenario
	var resilience *arenaconfig.DuplexResilienceConfig
	if req.Scenario != nil && req.Scenario.Duplex != nil {
		resilience = req.Scenario.Duplex.GetResilience()
	}
	maxRetries := resilience.GetMaxRetries(defaultMaxRetries)
	retryDelayMS := resilience.GetRetryDelayMs(defaultRetryDelayMS)

	var result *ConversationResult
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			logger.Info("Retrying duplex conversation",
				"attempt", attempt,
				"max_retries", maxRetries,
				"retry_delay_ms", retryDelayMS)
			time.Sleep(time.Duration(retryDelayMS) * time.Millisecond)

			// Clear state store for fresh retry
			if err := de.clearStateStoreForRetry(ctx, req); err != nil {
				logger.Warn("Failed to clear state store for retry", "error", err)
			}
		}

		// Build and execute the duplex pipeline
		// The session is created inside the pipeline by DuplexProviderStage,
		// using system_prompt from PromptAssemblyStage metadata.
		result = de.executeDuplexPipeline(ctx, req, streamProvider, emitter)

		// Check if we should retry
		if !result.Failed {
			// Success - no need to retry
			break
		}

		if !isRecoverableError(result.Err) {
			// Non-recoverable error - don't retry
			logger.Debug("Non-recoverable error, not retrying", "error", result.Error)
			break
		}

		if attempt < maxRetries {
			logger.Warn("Duplex conversation failed with recoverable error, will retry",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"error", result.Error)
		}
	}

	de.emitSessionCompleted(emitter, req)
	return result
}

// statusCoder is any error that carries an HTTP-style status code (runtime
// provider error types may implement this).
type statusCoder interface{ StatusCode() int }

// authErrorClassifier / retryableErrorClassifier are the typed classification
// hooks the runtime provider errors expose (e.g. gemini.APIError). Matching on
// these — not on substrings of the stringified message — is how we tell a 401
// from a transient socket drop.
type authErrorClassifier interface{ IsAuthError() bool }
type retryableErrorClassifier interface{ IsRetryable() bool }

// isRecoverableError reports whether a failed duplex attempt should be retried.
// It classifies by error TYPE, never by substring: retrying is only worthwhile
// for errors the provider itself declares transient (rate limits, 5xx, socket
// drops). Auth failures (401/403) and client errors are terminal — retrying a
// bad API key just loops. Unknown error types are treated as non-recoverable:
// blind retrying is exactly what turned a 401 into an endless run.
func isRecoverableError(err error) bool {
	if err == nil {
		return false
	}
	// Context death is never retryable.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Auth errors are terminal — a 401 is a 401.
	var authErr authErrorClassifier
	if errors.As(err, &authErr) && authErr.IsAuthError() {
		return false
	}
	// Any 4xx (client error) is terminal; 429 is the one retryable 4xx and the
	// provider's own IsRetryable handles it below.
	var coded statusCoder
	if errors.As(err, &coded) {
		code := coded.StatusCode()
		if code >= 400 && code < 500 && code != http.StatusTooManyRequests {
			return false
		}
	}
	// Honour the provider's explicit retryability classification.
	var retryable retryableErrorClassifier
	if errors.As(err, &retryable) {
		return retryable.IsRetryable()
	}
	// Unknown/untyped error: do not retry.
	return false
}

// clearStateStoreForRetry clears the state store before a retry attempt.
// This ensures we don't accumulate duplicate messages across retries.
func (de *DuplexConversationExecutor) clearStateStoreForRetry(ctx context.Context, req *ConversationRequest) error {
	if req.StateStoreConfig == nil || req.StateStoreConfig.Store == nil {
		return nil
	}

	// ArenaStateStore has a Delete method, but the generic Store interface doesn't
	arenaStore, ok := req.StateStoreConfig.Store.(*arenastore.ArenaStateStore)
	if !ok {
		// For other store types, bulk-write an empty state to reset.
		// Requires BulkWriter; stores without bulk-write support are skipped.
		if bulkWriter, ok := req.StateStoreConfig.Store.(statestore.BulkWriter); ok {
			emptyState := &statestore.ConversationState{
				ID:       req.ConversationID,
				Messages: []types.Message{},
				Metadata: make(map[string]interface{}),
			}
			return bulkWriter.Save(ctx, emptyState)
		}
		return nil
	}

	// Delete existing state for this conversation
	return arenaStore.Delete(ctx, req.ConversationID)
}

// executeDuplexPipeline builds and runs the duplex streaming pipeline.
func (de *DuplexConversationExecutor) executeDuplexPipeline(
	ctx context.Context,
	req *ConversationRequest,
	streamProvider providers.StreamInputSupport,
	emitter *events.Emitter,
) *ConversationResult {
	// Create pipeline for duplex streaming
	//nolint:gocritic // Variable shadowing unavoidable in this context
	pipeline, err := de.buildDuplexPipeline(req, streamProvider)
	if err != nil {
		return &ConversationResult{
			Failed: true,
			Error:  fmt.Sprintf("failed to build duplex pipeline: %v", err),
		}
	}

	// Create input channel for audio chunks
	inputChan := make(chan stage.StreamElement)

	// Start pipeline execution
	outputChan, err := pipeline.Execute(ctx, inputChan)
	if err != nil {
		return &ConversationResult{
			Failed: true,
			Error:  fmt.Sprintf("failed to execute duplex pipeline: %v", err),
			Err:    err,
		}
	}

	// Get base directory for resolving file paths
	baseDir := ""
	if req.Config != nil {
		baseDir = req.Config.ConfigDir
	}

	// Process turns from scenario
	err = de.processDuplexTurns(ctx, req, baseDir, inputChan, outputChan, emitter)
	isExpectedErr := isExpectedDuplexError(err)
	if err != nil && !isExpectedErr {
		return &ConversationResult{
			Failed: true,
			Error:  fmt.Sprintf("duplex conversation failed: %v", err),
			Err:    err,
		}
	}

	// Build result from state store
	return de.buildResultFromStateStore(ctx, req)
}

// isExpectedDuplexError checks if an error is expected (not a failure).
// Uses errors.Is so wrapped errors (e.g. "turn failed: %w" around
// context.DeadlineExceeded) are recognized — without that, a deadline
// reached deep in the turn loop comes back wrapped and the duplex path
// throws away an otherwise-complete result, skipping the assertion
// evaluation that's the whole point of running the scenario.
func isExpectedDuplexError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, errPartialSuccess)
}

// buildDuplexPipeline creates the streaming pipeline for duplex mode.
// The pipeline follows the same pattern as non-duplex: PromptAssemblyStage runs first
// to add system_prompt to metadata, then DuplexProviderStage creates the session
// using that system_prompt.
func (de *DuplexConversationExecutor) buildDuplexPipeline(
	req *ConversationRequest,
	streamProvider providers.StreamInputSupport,
) (*stage.StreamPipeline, error) {
	// Create pipeline with no ExecutionTimeout - duplex conversations use the parent context's
	// timeout (configured via scenario.duplex.timeout, default 10 minutes) for overall timing.
	// The default 30-second ExecutionTimeout would prematurely cancel multi-turn conversations.
	// Pipeline IdleTimeout defaults to 30s — far too short for duplex
	// scenarios where a single LLM-as-user / TTS / Gemini-response cycle
	// can quietly take longer than 30s. Drive it from the scenario's
	// `duplex.timeout` instead so a scenario declaring `timeout: 10m`
	// gets a 10-minute idle ceiling, not 30s.
	builder := stage.NewPipelineBuilderWithConfig(de.buildDuplexPipelineConfig(req))
	var stages []stage.Stage

	// Build merged variables for prompt assembly (consistent with non-duplex pipeline)
	mergedVars := de.buildMergedVariables(req)

	// Determine target sample rate from provider capabilities
	// Each provider has different audio requirements (e.g., Gemini: 16kHz, OpenAI: 24kHz)
	targetSampleRate := resolveTargetSampleRate(streamProvider)

	stages, err := de.appendDuplexInputStages(stages, req, targetSampleRate)
	if err != nil {
		return nil, err
	}

	// 1. Prompt assembly stage (runs BEFORE provider, like non-duplex)
	// This enriches elements with:
	// - system_prompt for DuplexProviderStage to use at session creation
	// - base_variables for template processing
	taskType := ""
	if req.Scenario != nil {
		taskType = req.Scenario.TaskType
	}
	turnState := stage.NewTurnState()
	stages = append(stages,
		stage.NewVariableProviderStageWithVarsAndTurnState(mergedVars, nil, turnState),
		stage.NewPromptAssemblyStageWithTurnState(de.promptRegistry, taskType, mergedVars, turnState),
		// NOTE: ScenarioContextExtractionStage is NOT included in the duplex pipeline.
		// It accumulates ALL elements before forwarding, which blocks the real-time
		// element flow needed for duplex streaming. Context extraction is handled
		// via mergedVars passed to PromptAssemblyStage.
		stage.NewTemplateStageWithTurnState(nil, turnState),
	)

	// 2. Duplex provider stage - creates session using system_prompt from metadata
	// The session is created lazily when the first element arrives, reading
	// system_prompt from the element's metadata (set by PromptAssemblyStage).
	baseConfig := de.buildBaseSessionConfig(req, targetSampleRate)

	// Create emitter for audio event recording if event bus is available
	// Use RunID as SessionID to ensure events are stored (EventBus only stores events with non-empty SessionID)
	var emitter *events.Emitter
	if req.EventBus != nil {
		emitter = events.NewEmitter(req.EventBus, req.RunID, req.RunID, req.ConversationID)
	}

	duplexStage := stage.NewDuplexProviderStageWithTurnState(
		streamProvider, baseConfig, emitter, turnState,
	)
	// Wire the session observer (interactive ASM barge-in: session.BargeIn →
	// playback flush). Nil for non-interactive runs.
	if req.OnDuplexSession != nil {
		duplexStage.SetSessionObserver(req.OnDuplexSession)
	}
	stages = append(stages, duplexStage)

	stages = de.appendDuplexOutputStages(stages, req, turnState, emitter)

	return builder.Chain(stages...).Build()
}

// buildDuplexPipelineConfig builds the StreamPipeline configuration for a duplex
// run. ExecutionTimeout is disabled (duplex uses the parent context's
// scenario-driven timeout) and IdleTimeout is driven from the scenario's
// duplex.timeout so long single LLM/TTS cycles don't trip the 30s default. With
// barge-in it also shrinks the inter-stage channel buffers so interrupted
// response audio can't keep playing for seconds after the user speaks.
func (de *DuplexConversationExecutor) buildDuplexPipelineConfig(req *ConversationRequest) *stage.PipelineConfig {
	idleTimeout := defaultPipelineIdleTimeout
	if req.Scenario != nil && req.Scenario.Duplex != nil {
		idleTimeout = req.Scenario.Duplex.GetTimeoutDuration(idleTimeout)
	}
	pipelineConfig := stage.DefaultPipelineConfig().
		WithExecutionTimeout(0).
		WithIdleTimeout(idleTimeout)
	if req.VoiceBargeIn {
		pipelineConfig = pipelineConfig.WithChannelBufferSize(bargeInChannelBufferSize)
	}
	return pipelineConfig
}

// resolveTargetSampleRate returns the provider's preferred input sample rate,
// falling back to the default when the provider declares no preference. Each
// provider has different audio requirements (e.g. Gemini 16kHz, OpenAI 24kHz).
func resolveTargetSampleRate(streamProvider providers.StreamInputSupport) int {
	targetSampleRate := defaultSampleRate // fallback to 16kHz
	caps := streamProvider.GetStreamingCapabilities()
	if caps.Audio != nil && caps.Audio.PreferredSampleRate > 0 {
		targetSampleRate = caps.Audio.PreferredSampleRate
		logger.Debug("buildDuplexPipeline: using provider preferred sample rate",
			"sample_rate", targetSampleRate)
	}
	return targetSampleRate
}

// appendDuplexInputStages appends the input-side stages (audio pacing, input
// monitor tap, resample, client-side VAD) in order. Pacing and the monitor tap
// are opt-in; the resample stage always runs. Returns an error only if the VAD
// stage fails to construct.
func (de *DuplexConversationExecutor) appendDuplexInputStages(
	stages []stage.Stage,
	req *ConversationRequest,
	targetSampleRate int,
) ([]stage.Stage, error) {
	// 0a. Audio pacing stage — emits each audio element at the cadence implied
	// by its byte count and sample rate so downstream consumers (input tap →
	// local-sink playback, and the provider's VAD) see a microphone-like rhythm.
	// Skipped when nothing downstream cares about cadence (selfplay + no router).
	// Placed first, before the input MonitorTap, so both paths observe the flow.
	if needsAudioPacing(req) {
		stages = append(stages, stage.NewAudioPacingStage())
	}

	// 0b. Input audio monitor tap (opt-in) — placed BEFORE the resample stage so
	// the router hears audio at its source sample rate (resample-first causes
	// chunk-boundary phase clicks). The tap still resamples to the router's
	// canonical rate internally so consumers continue to see one rate.
	if req.AudioRouter != nil {
		stages = append(stages, arenaaudio.NewMonitorTap(req.AudioRouter, arenaaudio.MonitorTapConfig{
			Position: stage.RecordingPositionInput,
		}))
	}

	// 1. Audio resample stage - normalizes all input audio to target sample rate
	// for the provider. Downstream stages receive consistent sample rates.
	stages = append(stages, stage.NewAudioResampleStage(stage.AudioResampleConfig{
		TargetSampleRate:      targetSampleRate,
		PassthroughIfSameRate: true,
	}))

	// Add VAD stage if using client-side turn detection
	if de.shouldUseClientVAD(req) {
		vadStage, err := stage.NewAudioTurnStage(de.buildVADConfig(req))
		if err != nil {
			return nil, fmt.Errorf("failed to create VAD stage: %w", err)
		}
		stages = append(stages, vadStage)
	}
	return stages, nil
}

// appendDuplexOutputStages appends the output-side stages (output audio pacing,
// output monitor tap, media externalizer, arena state-store save) in order.
// Each is gated on its enabling condition; the save stage wires the emitter when
// one is available so message elements are broadcast to the live TUI.
func (de *DuplexConversationExecutor) appendDuplexOutputStages(
	stages []stage.Stage,
	req *ConversationRequest,
	turnState *stage.TurnState,
	emitter *events.Emitter,
) []stage.Stage {
	// 2a-pre. Output audio pacing. Realtime providers stream assistant audio
	// faster than playback rate; without output pacing the LocalSink queue fills
	// instantly (turn-end fires before audio is heard) and the next selfplay turn
	// overlaps the still-playing response. Same gate as input pacing; distinct
	// name so the builder doesn't reject the pair as duplicates.
	if needsAudioPacing(req) {
		stages = append(stages, stage.NewNamedAudioPacingStage("audio-pacing-output"))
	}

	// 2a. Output audio monitor tap (opt-in). Placed after the provider so
	// generated agent audio flows through the tap to the router at realtime rate.
	if req.AudioRouter != nil {
		stages = append(stages, arenaaudio.NewMonitorTap(req.AudioRouter, arenaaudio.MonitorTapConfig{
			Position: stage.RecordingPositionOutput,
		}))
	}

	// 3. Media externalizer stage to save audio files
	if de.mediaStorage != nil {
		stages = append(stages, stage.NewMediaExternalizerStage(&stage.MediaExternalizerConfig{
			Enabled:         true,
			StorageService:  de.mediaStorage,
			SizeThresholdKB: 0, // Externalize all media (audio can be large)
			DefaultPolicy:   "retain",
			RunID:           req.RunID,
			ConversationID:  req.ConversationID,
		}))
	}

	// 5. Arena state store save stage to capture conversation messages. Handles
	// system_prompt in metadata and prepends it as a system message. When an
	// event bus is available we wire an emitter so each Message element is also
	// broadcast on the bus — that feeds the live TUI conversation panel.
	if req.StateStoreConfig != nil && req.StateStoreConfig.Store != nil {
		saveStage := arenastages.NewArenaStateStoreSaveStageWithTurnState(
			de.buildPipelineStateStoreConfig(req), turnState)
		if emitter != nil {
			saveStage = saveStage.WithEmitter(emitter)
		}
		stages = append(stages, saveStage)
	}
	return stages
}

// buildBaseSessionConfig creates the base streaming configuration without system instruction.
// The system instruction will be added by DuplexProviderStage from element metadata.
// sampleRate is determined from provider capabilities in buildDuplexPipeline.
func (de *DuplexConversationExecutor) buildBaseSessionConfig(
	req *ConversationRequest,
	sampleRate int,
) *providers.StreamingInputConfig {
	cfg := &providers.StreamingInputConfig{
		Config: types.StreamingMediaConfig{
			Type:       types.ContentTypeAudio,
			ChunkSize:  defaultAudioChunkSize,
			SampleRate: sampleRate,
			Encoding:   "pcm_linear16",
			Channels:   1,
			BitDepth:   geminiAudioBitDepth,
		},
		Metadata: make(map[string]interface{}),
	}

	// Explicitly enable input-audio transcription so the OpenAI Realtime provider
	// emits conversation.item.input_audio_transcription.completed events.
	// The provider defaults to enabled when the key is absent, but setting it
	// explicitly prevents accidental suppression by later metadata writes.
	cfg.Metadata["input_transcription"] = true

	de.applyResponseModalities(cfg, req)
	de.applySelfPlayVADConfig(cfg, req)
	de.applyScenarioVADConfig(cfg, req)
	de.applyToolsConfig(cfg)
	de.applyMockScenarioContext(cfg, req)

	return cfg
}

// applyMockScenarioContext threads the scenario ID through the streaming
// session metadata so a mock provider with a configured response repository
// can return per-turn scenario-mapped responses instead of a static
// auto-respond text. No-op for real providers — they ignore the key.
func (de *DuplexConversationExecutor) applyMockScenarioContext(
	cfg *providers.StreamingInputConfig,
	req *ConversationRequest,
) {
	if req.Scenario == nil || req.Scenario.ID == "" {
		return
	}
	cfg.Metadata["mock_scenario_id"] = req.Scenario.ID
}

// applyResponseModalities adds response modalities from provider config.
func (de *DuplexConversationExecutor) applyResponseModalities(
	cfg *providers.StreamingInputConfig,
	req *ConversationRequest,
) {
	if req.Config == nil || req.Provider == nil {
		return
	}
	providerID := req.Provider.ID()
	providerCfg, ok := req.Config.LoadedProviders[providerID]
	if !ok || providerCfg.AdditionalConfig == nil {
		return
	}
	if modalities, exists := providerCfg.AdditionalConfig["response_modalities"]; exists {
		cfg.Metadata["response_modalities"] = modalities
	}
}

// needsAudioPacing reports whether the data path actually needs cadence
// enforcement on this run. The pacing stage is the sole cadence
// authority (see runtime/pipeline/stage/stages_pacing.go and
// tools/arena/audio/types.go on the observer model), so the only thing
// to decide here is "is anyone on the data path going to interpret
// arrival rate?" Two cases say yes:
//
//   - Provider VAD: any non-selfplay duplex run. The provider's session
//     timestamps audio arrival to time turn-end silence, and that has
//     to be wall-clock-correct regardless of who is listening locally.
//   - Live observer attached (req.AudioRouter != nil): a LocalSink or
//     SSE relay is fanning audio out for human listening. Pacing
//     produces a smooth playback drain off the router. Observers
//     themselves cannot exert backpressure — pacing is what gives them
//     non-bursty input.
//
// The remaining case (selfplay, VAD off, no router attached — headless
// CI selfplay) is the only combination where pacing wastes wall-clock
// time on no consumer, so we skip it.
func needsAudioPacing(req *ConversationRequest) bool {
	if req.AudioRouter != nil {
		return true
	}
	if req.Config == nil || req.Config.SelfPlay == nil || !req.Config.SelfPlay.IsEnabled() {
		return true
	}
	return false
}

// applySelfPlayVADConfig disables provider-side VAD only when THIS
// scenario actually uses selfplay (i.e. has at least one persona-driven
// turn). Previously this checked whether selfplay was *configured* at
// the arena level, which incorrectly disabled VAD for scripted-text
// scenarios sharing the same arena config — see
// `duplex-scripted-text-openai`, which declares
// `turn_detection.mode: asm` but was getting `automaticActivityDetection.disabled=true`
// pushed to Gemini, defeating ASM and causing long inter-turn delays
// that looked like a playback bug.
func (de *DuplexConversationExecutor) applySelfPlayVADConfig(
	cfg *providers.StreamingInputConfig,
	req *ConversationRequest,
) {
	if req.Config == nil || req.Config.SelfPlay == nil || !req.Config.SelfPlay.IsEnabled() {
		return
	}
	if !scenarioUsesSelfPlay(req.Scenario) {
		return
	}
	cfg.Metadata["vad_disabled"] = true
	logger.Debug("buildBaseSessionConfig: VAD disabled for selfplay scenario",
		"scenario_id", req.Scenario.ID)
}

// scenarioUsesSelfPlay reports whether the given scenario has at least
// one turn driven by a persona. Scripted-text scenarios (turns with
// only `content`) return false even when the arena's self_play block
// is populated — those turns don't drive the LLM-as-user path and
// must not trigger VAD-disabled overrides intended for selfplay.
func scenarioUsesSelfPlay(scenario *arenaconfig.Scenario) bool {
	if scenario == nil {
		return false
	}
	for i := range scenario.Turns {
		if scenario.Turns[i].Persona != "" {
			return true
		}
	}
	return false
}

// applyScenarioVADConfig adds VAD configuration from scenario.
func (de *DuplexConversationExecutor) applyScenarioVADConfig(
	cfg *providers.StreamingInputConfig,
	req *ConversationRequest,
) {
	if req.Scenario == nil || req.Scenario.Duplex == nil {
		return
	}
	if req.Scenario.Duplex.TurnDetection == nil || req.Scenario.Duplex.TurnDetection.VAD == nil {
		return
	}

	vad := req.Scenario.Duplex.TurnDetection.VAD
	vadConfig := make(map[string]interface{})

	if vad.SilenceThresholdMs > 0 {
		vadConfig["silence_threshold_ms"] = vad.SilenceThresholdMs
	}
	if vad.MinSpeechMs > 0 {
		vadConfig["min_speech_ms"] = vad.MinSpeechMs
	}
	if vad.MaxTurnDurationS > 0 {
		vadConfig["max_turn_duration_s"] = vad.MaxTurnDurationS
	}

	if len(vadConfig) > 0 {
		cfg.Metadata["vad_config"] = vadConfig
		logger.Debug("buildBaseSessionConfig: VAD config from scenario",
			"silence_threshold_ms", vad.SilenceThresholdMs,
			"min_speech_ms", vad.MinSpeechMs)
	}
}

// applyToolsConfig adds tools from the tool registry to the session config.
func (de *DuplexConversationExecutor) applyToolsConfig(cfg *providers.StreamingInputConfig) {
	if de.toolRegistry == nil {
		return
	}

	toolDescs := de.toolRegistry.GetTools()
	if len(toolDescs) == 0 {
		return
	}

	cfg.Tools = make([]providers.StreamingToolDefinition, 0, len(toolDescs))
	for _, td := range toolDescs {
		cfg.Tools = append(cfg.Tools, de.convertToolDefinition(td))
	}
	logger.Debug("buildBaseSessionConfig: tools configured for streaming session",
		"tool_count", len(cfg.Tools))
}

// convertToolDefinition converts a tool descriptor to a streaming tool definition.
func (de *DuplexConversationExecutor) convertToolDefinition(
	td *tools.ToolDescriptor,
) providers.StreamingToolDefinition {
	var params map[string]interface{}
	if td.InputSchema != nil {
		if err := json.Unmarshal(td.InputSchema, &params); err != nil {
			logger.Debug("buildBaseSessionConfig: failed to parse tool schema",
				"tool", td.Name, "error", err)
			params = nil
		}
	}
	return providers.StreamingToolDefinition{
		Name:        td.Name,
		Description: td.Description,
		Parameters:  params,
	}
}

// buildMergedVariables builds the merged variables map for prompt assembly.
// This is consistent with how non-duplex pipelines build variables.
func (de *DuplexConversationExecutor) buildMergedVariables(req *ConversationRequest) map[string]string {
	mergedVars := make(map[string]string)

	// Add region if available
	if req.Region != "" {
		mergedVars["region"] = req.Region
	}

	// Add any metadata from the request as variables
	for k, v := range req.Metadata {
		mergedVars[k] = v
	}

	return mergedVars
}

// buildPipelineStateStoreConfig converts engine StateStoreConfig to pipeline StateStoreConfig.
// It also injects the system prompt from the prompt registry into metadata so that
// ArenaStateStoreSaveStage can capture it in the state store output.
func (de *DuplexConversationExecutor) buildPipelineStateStoreConfig(
	req *ConversationRequest,
) *pipeline.StateStoreConfig {
	if req.StateStoreConfig == nil {
		return nil
	}

	// Start with existing metadata or create new map
	metadata := make(map[string]interface{})
	for k, v := range req.StateStoreConfig.Metadata {
		metadata[k] = v
	}

	// Inject system prompt from prompt registry if available
	// This ensures the system prompt is captured in the state store output
	if de.promptRegistry != nil && req.Scenario != nil && req.Scenario.TaskType != "" {
		if assembled := de.promptRegistry.Load(req.Scenario.TaskType); assembled != nil {
			if assembled.SystemPrompt != "" {
				metadata["system_prompt"] = assembled.SystemPrompt
			}
		}
	}

	return &pipeline.StateStoreConfig{
		Store:          req.StateStoreConfig.Store,
		ConversationID: req.ConversationID,
		UserID:         req.StateStoreConfig.UserID,
		Metadata:       metadata,
	}
}
