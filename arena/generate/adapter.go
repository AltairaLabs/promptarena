// Package generate provides pluggable session source adapters for scenario generation.
// It enables turning production session data (with failing assertions) into
// reproducible test scenarios for PromptArena.
package generate

import (
	"context"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// SessionSourceAdapter provides session data for scenario generation.
// Implementations query external systems (e.g., session APIs, local recordings)
// and return structured session data that can be converted into Arena scenarios.
type SessionSourceAdapter interface {
	// Name returns the adapter's unique identifier (e.g., "recordings", "omnia").
	Name() string

	// List returns session summaries matching the given options.
	List(ctx context.Context, opts ListOptions) ([]SessionSummary, error)

	// Get returns the full session detail including messages and eval results.
	Get(ctx context.Context, sessionID string) (*SessionDetail, error)
}

// ListOptions controls which sessions are returned by List.
type ListOptions struct {
	// FilterPassed filters by pass/fail status: nil=all, *true=passed only, *false=failed only.
	FilterPassed *bool
	// FilterEvalType filters by assertion type (e.g., "content_matches").
	FilterEvalType string
	// Limit caps the number of results. 0 means unlimited.
	Limit int
	// Expectations narrow to sessions where a measurement fell outside the
	// given range. Adapters that can filter server-side should; the converter
	// re-checks client-side so the outcome is the same either way.
	Expectations []Expectation
}

// SessionSummary is a lightweight representation of a session for listing.
type SessionSummary struct {
	ID          string
	Source      string
	ScenarioID  string
	ProviderID  string
	Timestamp   time.Time
	TurnCount   int
	HasFailures bool
	Tags        []string
	Metadata    map[string]interface{}
}

// SessionDetail contains the full session data needed for scenario generation.
// Every field beyond Messages is optional: adapters fill what their source
// records and leave the rest nil.
type SessionDetail struct {
	SessionSummary
	// Messages is the complete conversation history, tool calls and results
	// inline, multimodal parts inline or by URI.
	Messages []types.Message
	// Pack identifies the pack the session ran against, as far as the source
	// knows it. Nil when unknown.
	Pack *PackRef
	// Variables are the template variables the run was invoked with. Nil when
	// the source does not record them.
	Variables map[string]string
	// Workflow is the workflow trace: entry state and ordered transitions. Nil
	// when the session ran no workflow or the source does not record it.
	Workflow *WorkflowTrace
	// Evals are every recorded eval observation at both levels: measurements
	// (kind "eval"), judged assertions and guardrails. Nil when the source
	// ran none.
	Evals []EvalResult
}

// EvalResult is one recorded eval observation.
//
// Kind says what role the eval played (events.EvalKind: "eval", "assertion",
// "guardrail"). A measurement has a Score and a nil Passed; only a judged
// assertion or a guardrail carries a verdict. Params are the eval's own
// parameters when the source knows them (arena run output records them;
// Omnia does not, and the pack supplies them). Threshold is the pack author's
// declared expectation when the pack is available. Turn is nil for a
// conversation-level result, else the index of the USER turn it answers.
type EvalResult struct {
	ID        string
	Type      string
	Kind      string
	Score     *float64
	Passed    *bool
	Params    map[string]interface{}
	Threshold *EvalThreshold
	Message   string
	Details   map[string]interface{}
	Turn      *int
}

// Failed reports whether the eval reached a verdict and it was negative. A
// measurement without a verdict is never "failed"; whether it is out of range
// is an Expectation's question.
func (r EvalResult) Failed() bool {
	return r.Passed != nil && !*r.Passed
}

// EvalThreshold mirrors packspec.EvalThreshold: the pack author's declared
// pass/fail bound on a score. Operator follows the spec's schema guide
// ("gte", "lte", "gt", "lt", "eq"); symbolic forms are accepted too.
type EvalThreshold struct {
	Operator string
	Value    float64
}

// PackRef identifies the pack a session ran against. Any field may be empty.
type PackRef struct {
	Name    string
	Version string
	Digest  string
}

// WorkflowTrace is what a session recorded of its workflow.
type WorkflowTrace struct {
	// EntryState is the state the session started in; EntryPromptTask is that
	// state's prompt_task, which is how an arena scenario selects a start
	// state (task_type). Either may be empty when the source does not know.
	EntryState      string
	EntryPromptTask string
	// Transitions in the order they happened.
	Transitions []WorkflowTransition
}

// WorkflowTransition is one recorded state change.
type WorkflowTransition struct {
	From       string
	To         string
	Event      string
	PromptTask string
	// MessageIndex is the index into SessionDetail.Messages of the last message
	// recorded before the transition, so a transition can be placed in the
	// conversation. -1 when unknown.
	MessageIndex int
}
