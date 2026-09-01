package assertions

import (
	"fmt"
	"strings"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
)

// PackEvalTypePrefix is prepended to eval types when converting to assertion results.
// Report renderers use this prefix to distinguish pack eval results from scenario assertions.
const PackEvalTypePrefix = "pack_eval:"

// ExtractScore extracts a float64 score from a Details map.
// Returns (score, true) if present and numeric, (0, false) otherwise.
func ExtractScore(details map[string]interface{}) (float64, bool) {
	if details == nil {
		return 0, false
	}
	v, ok := details["score"]
	if !ok {
		return 0, false
	}
	switch s := v.(type) {
	case float64:
		return s, true
	case float32:
		return float64(s), true
	case int:
		return float64(s), true
	case int64:
		return float64(s), true
	default:
		return 0, false
	}
}

// ExtractDurationMs extracts eval duration in milliseconds from a Details map.
// Returns (duration, true) if present and numeric, (0, false) otherwise.
func ExtractDurationMs(details map[string]interface{}) (int64, bool) {
	if details == nil {
		return 0, false
	}
	v, ok := details["duration_ms"]
	if !ok {
		return 0, false
	}
	switch d := v.(type) {
	case int64:
		return d, true
	case float64:
		return int64(d), true
	case int:
		return int64(d), true
	default:
		return 0, false
	}
}

// ExtractEvalID extracts the eval ID string from a Details map.
// Returns (id, true) if present and a string, ("", false) otherwise.
func ExtractEvalID(details map[string]interface{}) (string, bool) {
	if details == nil {
		return "", false
	}
	v, ok := details["eval_id"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// IsPackEval returns true if the ConversationValidationResult originated from a pack eval.
func IsPackEval(cvr ConversationValidationResult) bool {
	return strings.HasPrefix(cvr.Type, PackEvalTypePrefix)
}

// IsSkipped checks whether a Details map indicates the eval was skipped.
// Returns (reason, true) if skipped, ("", false) otherwise.
func IsSkipped(details map[string]interface{}) (string, bool) {
	if details == nil {
		return "", false
	}
	v, ok := details["skipped"]
	if !ok {
		return "", false
	}
	skipped, isBool := v.(bool)
	if !isBool || !skipped {
		return "", false
	}
	reason, _ := details["skip_reason"].(string)
	return reason, true
}

// ConvertEvalResults transforms a slice of EvalResult into ConversationValidationResult entries.
// Each result is tagged with the PackEvalTypePrefix so renderers can group them separately.
// This function is used by both the PackEvalAdapter (engine) and the statestore when
// building AssertionsSummary from eval results.
func ConvertEvalResults(results []evals.EvalResult) []ConversationValidationResult {
	if len(results) == 0 {
		return nil
	}

	converted := make([]ConversationValidationResult, 0, len(results))
	for i := range results {
		converted = append(converted, convertOneEvalResult(&results[i]))
	}
	return converted
}

// convertOneEvalResult converts a single EvalResult to ConversationValidationResult.
func convertOneEvalResult(r *evals.EvalResult) ConversationValidationResult {
	passed := EvalResultPassed(r)

	msg := r.Explanation
	if !passed && r.Error != "" {
		msg = r.Error
	}

	details := make(map[string]interface{})
	details["eval_id"] = r.EvalID
	details["duration_ms"] = r.DurationMs
	if r.Score != nil {
		details["score"] = *r.Score
	}
	if r.MetricValue != nil {
		details["metric_value"] = *r.MetricValue
	}
	if r.Value != nil {
		details["value"] = r.Value
	}
	// Merge handler-supplied Details (e.g. tool_exec's parsed tool
	// response) into the output. Handler-supplied keys win over the
	// fixed fields above only when the handler explicitly chose the
	// same key — typically Details carries handler-specific metric
	// payloads that are otherwise unrepresentable on the typed fields.
	for k, v := range r.Details {
		if _, taken := details[k]; !taken {
			details[k] = v
		}
	}
	if r.Error != "" {
		details["error"] = r.Error
	}
	if r.Skipped {
		details["skipped"] = true
		details["skip_reason"] = r.SkipReason
	}

	return ConversationValidationResult{
		Type:    fmt.Sprintf("%s%s", PackEvalTypePrefix, r.Type),
		Passed:  passed,
		Message: msg,
		Details: details,
	}
}

// EvalResultPassed reports a result's pass/fail, preferring the one the result
// STATES over anything derived from it.
//
// PromptKit distinguishes three roles. An assertion and a guardrail coerce a
// measurement to a boolean and state it in Passed. An eval MEASURES: it returns
// a value and a score and states no pass or fail at all.
//
// This used to read r.Value.(bool), because the promptkit assertion wrapper
// overwrote Value with its boolean. It no longer does — Value now carries the
// inner eval's own output, which is what a report most wants to show — so that
// read fell through to the score branch for every assertion, and an llm_judge
// scoring 0.9 under min_score 0.8 was reported as FAILED. That `>= 1.0` is an
// assertion's DEFAULT threshold; applying it to a result carrying its own
// threshold is the same bug promptkit removed from its event path in #1861.
//
// The score fallback stays for results that state nothing — a plain eval, or a
// result deserialized from a run predating Passed. Arena's report needs a
// boolean in that column either way, and how it should present a measurement
// that nobody asserted on is a separate question from this one.
//
// A skipped result is answered before any of that. PromptKit skips an eval
// whose `when` precondition is unmet and leaves Passed, Score and Value zero,
// which fell through to the score branch and reported FAILED — so a `when`
// gate failed the very run it was written to exempt, and gating was unusable
// (PromptKit #1931). "Never ran" is not "failed", and it outranks anything the
// result states about pass/fail, so the check comes first. Callers that need to
// tell a skip from a pass read Details["skipped"] via IsSkipped; this mirrors
// what the turn-assertion path already does for its own skips, marking them
// passed and skipped together.
func EvalResultPassed(r *evals.EvalResult) bool {
	if r.Skipped {
		return true
	}
	if r.Passed != nil {
		return *r.Passed
	}
	// An assertion's boolean lived in Value before Passed existed.
	if v, ok := r.Value.(bool); ok {
		return v
	}
	return r.Error == "" && r.Score != nil && *r.Score >= 1.0
}
