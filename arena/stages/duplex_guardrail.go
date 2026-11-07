package stages

import (
	"context"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/hooks"
	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// DuplexGuardrailStage runs a pack's output guardrails on the assistant turns
// of a realtime duplex session.
//
// The text path runs guardrails as provider hooks inside ProviderStage. The
// duplex provider stage has no hook support, so without this stage a scenario
// with a duplex: block ran with no validator enforcement at all, and
// guardrail_triggered could never observe a firing. This stage sits right
// after DuplexProviderStage and gives each completed assistant message the same
// AfterCall treatment ProviderStage does: on a firing it stamps the message's
// Validations (which guardrail_triggered reads), emits the validation event,
// and, for an enforcing guardrail, rewrites the message content.
//
// Enforcement in a realtime session is necessarily after the fact: the
// provider streamed the turn's audio and transcript deltas while it spoke, so a
// listener has already heard them by the time the turn completes. What this
// stage corrects is the turn as recorded: the transcript held in state, the
// recording, the reports and every later assertion. Input guardrails do not
// run here, because the user's audio has already reached the provider.
type DuplexGuardrailStage struct {
	stage.BaseStage
	hooks      *hooks.Registry
	emitter    *events.Emitter
	providerID string
	model      string
}

// NewDuplexGuardrailStage creates the stage. reg holds the compiled pack
// validators; emitter may be nil (no validation events).
func NewDuplexGuardrailStage(
	reg *hooks.Registry, emitter *events.Emitter, providerID, model string,
) *DuplexGuardrailStage {
	reg.SetEmitter(emitter)
	return &DuplexGuardrailStage{
		BaseStage:  stage.NewBaseStage("duplex_guardrails", stage.StageTypeTransform),
		hooks:      reg,
		emitter:    emitter,
		providerID: providerID,
		model:      model,
	}
}

// Process forwards every element, guarding each assistant message on the way.
// A hook that denies without enforcing (a hand-written hook, never a pack
// validator) fails the turn the way it fails a text turn: its error rides on
// the element that carried the message.
func (s *DuplexGuardrailStage) Process(
	ctx context.Context,
	input <-chan stage.StreamElement,
	output chan<- stage.StreamElement,
) error {
	defer close(output)

	var fwd cancelForwarder
	var history []types.Message
	turn := 0
	for elem := range input {
		if msg := elem.Message; msg != nil {
			if msg.Role == roleAssistant {
				if err := s.guard(ctx, msg, history, turn); err != nil && elem.Error == nil {
					elem.Error = err
				}
				turn++
			}
			history = append(history, *msg)
		}
		fwd.forward(ctx, output, elem)
	}
	return ctx.Err()
}

// guard runs the AfterCall chain on one assistant message, mirroring
// ProviderStage.runAfterCallHooks.
func (s *DuplexGuardrailStage) guard(
	ctx context.Context, msg *types.Message, history []types.Message, turn int,
) error {
	req := &hooks.ProviderRequest{
		ProviderID: s.providerID,
		Model:      s.model,
		Messages:   history,
		TurnIndex:  turn,
	}
	resp := &hooks.ProviderResponse{
		ProviderID: s.providerID,
		Model:      s.model,
		Message:    *msg,
		LatencyMs:  msg.LatencyMs,
	}

	start := time.Now()
	d := s.hooks.RunAfterProviderCall(ctx, req, resp)
	if d.Allow {
		return nil
	}

	s.recordFiring(msg, d, turn, time.Since(start))
	if !d.Enforced {
		return &hooks.HookDeniedError{
			HookName: "provider_hook",
			HookType: "provider_after",
			Reason:   d.Reason,
			Metadata: d.Metadata,
		}
	}
	applyEnforcedTurn(msg, &resp.Message, d)
	return nil
}

// recordFiring stamps the firing on msg.Validations and emits the validation
// event, as ProviderStage.recordGuardrailFiring does.
func (s *DuplexGuardrailStage) recordFiring(
	msg *types.Message, d hooks.Decision, turn int, duration time.Duration,
) {
	vType, _ := d.Metadata["validator_type"].(string)
	if vType != "" {
		msg.Validations = append(msg.Validations, types.ValidationResult{
			ValidatorType: vType,
			Passed:        false,
			Details:       firingDetails(d),
			Timestamp:     time.Now(),
		})
	}

	if s.emitter == nil {
		return
	}
	data := &events.ValidationEventData{
		ValidatorName: vType,
		ValidatorType: vType,
		Direction:     hooks.DirectionOutput,
		Duration:      duration,
		Enforced:      d.Enforced,
		Score:         decisionScore(d.Metadata),
		TurnIndex:     turn,
		Violations:    []string{d.Reason},
	}
	data.Validator, _ = d.Metadata[hooks.MetadataKeyValidatorDeclaration].(*packspec.Validator)
	s.emitter.GuardrailResult(data)
}

// firingDetails copies a decision's metadata for the recorded validation: the
// score dereferenced to a plain float64, the direction tagged, and the pack
// declaration left out (it rides on the decision only for the event).
func firingDetails(d hooks.Decision) map[string]any {
	details := make(map[string]any, len(d.Metadata)+1)
	for k, v := range d.Metadata {
		if k == hooks.MetadataKeyValidatorDeclaration {
			continue
		}
		details[k] = v
	}
	if _, isPtr := details["score"].(*float64); isPtr {
		details["score"] = decisionScore(d.Metadata)
	}
	details["direction"] = hooks.DirectionOutput
	return details
}

// decisionScore reads a guardrail's score, which the adapter stores as a
// *float64 and a hand-written hook may store as a float64.
func decisionScore(metadata map[string]any) float64 {
	switch v := metadata["score"].(type) {
	case *float64:
		if v != nil {
			return *v
		}
	case float64:
		return v
	}
	return 0
}

// applyEnforcedTurn folds an enforcing decision into the assistant message.
// The hook rewrote enforced.Content (truncated or replaced); the duplex message
// also carries its transcript as a text part, which is rewritten to match so no
// reader of Parts sees the original text. Audio parts are kept: they are what
// the session actually played. Tool calls are kept too, unlike the text path:
// the session already executed them, and dropping them would hide that.
func applyEnforcedTurn(msg, enforced *types.Message, d hooks.Decision) {
	msg.Content = enforced.Content
	for _, v := range enforced.Validations {
		if !containsValidation(msg.Validations, v) {
			msg.Validations = append(msg.Validations, v)
		}
	}
	msg.Parts = replaceTextParts(msg.Parts, msg.Content)
	msg.FinishReason = types.FinishReasonSafety
	for k, v := range d.Metadata {
		if k == hooks.MetadataKeyValidatorDeclaration {
			continue
		}
		if msg.Meta == nil {
			msg.Meta = make(map[string]interface{}, len(d.Metadata))
		}
		msg.Meta[k] = v
	}
}

// containsValidation reports whether vs already holds v (same validator,
// same timestamp). The response copy shares the message's validations, so
// without this every existing entry would be appended twice.
func containsValidation(vs []types.ValidationResult, v types.ValidationResult) bool {
	for i := range vs {
		if vs[i].ValidatorType == v.ValidatorType && vs[i].Timestamp.Equal(v.Timestamp) {
			return true
		}
	}
	return false
}

// replaceTextParts swaps the text parts for a single part holding content, at
// the position of the first one, and leaves every other part in place.
func replaceTextParts(parts []types.ContentPart, content string) []types.ContentPart {
	out := make([]types.ContentPart, 0, len(parts))
	replaced := false
	for _, p := range parts {
		if p.Type != types.ContentTypeText {
			out = append(out, p)
			continue
		}
		if !replaced {
			text := content
			out = append(out, types.ContentPart{Type: types.ContentTypeText, Text: &text})
			replaced = true
		}
	}
	return out
}
