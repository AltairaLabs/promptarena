package deploy

import (
	"context"
	"encoding/json"
	"time"
)

// SessionsCapability is the ProviderInfo capability string an adapter
// advertises when it implements SessionSourceProvider: it can list and fetch
// recorded sessions from the platform it deploys to, so `promptarena generate
// --source <adapter>` can turn production sessions into regression scenarios.
const SessionsCapability = "sessions"

// SessionSourceProvider is the OPTIONAL deploy capability for reading recorded
// sessions. Adapters that implement it advertise SessionsCapability in
// ProviderInfo.Capabilities, and the adaptersdk serves list_sessions and
// get_session only for providers that satisfy this interface.
//
// Callers decide whether the capability is available by checking Capabilities
// first; an adapter that does not serve the methods answers method-not-found,
// which surfaces as ErrMethodNotSupported.
//
// The adapter decides whether a filter is applied server-side; the caller
// re-checks client-side, so the result is the same either way. Redaction is
// the platform's responsibility before it serves session data — an adapter
// does none of its own.
type SessionSourceProvider interface {
	ListSessions(ctx context.Context, req *ListSessionsRequest) (*ListSessionsResponse, error)
	GetSession(ctx context.Context, req *GetSessionRequest) (*GetSessionResponse, error)
}

// SessionExpectation is a caller-stated range for a measured eval's score. A
// session whose score for EvalID falls outside [Min, Max], or that never
// measured it, matches.
type SessionExpectation struct {
	EvalID string   `json:"eval_id"`
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
}

// ListSessionsRequest asks for session summaries.
type ListSessionsRequest struct {
	// DeployConfig is the merged deploy config as JSON: endpoint, workspace,
	// api_token and whatever else the adapter's profile carries. The same
	// document Plan/Apply receive.
	DeployConfig string `json:"deploy_config"`
	// Environment is the deploy environment the config was merged for.
	Environment string `json:"environment"`
	// FilterPassed narrows by verdict: nil = all, true = passed only, false =
	// failed only (a failed recorded verdict or a violated expectation).
	FilterPassed *bool `json:"filter_passed,omitempty"`
	// FilterEvalType narrows to sessions that recorded an eval of this type.
	FilterEvalType string `json:"filter_eval_type,omitempty"`
	// Expectations narrow to sessions whose measurement fell outside a range.
	Expectations []SessionExpectation `json:"expectations,omitempty"`
	// Limit caps the page size. 0 means the adapter's default.
	Limit int `json:"limit,omitempty"`
	// Cursor continues a previous page; empty starts from the beginning.
	Cursor string `json:"cursor,omitempty"`
}

// ListSessionsResponse is one page of summaries.
type ListSessionsResponse struct {
	Sessions []SessionSummary `json:"sessions"`
	// NextCursor is non-empty when more pages follow.
	NextCursor string `json:"next_cursor,omitempty"`
}

// SessionSummary is a lightweight description of one session.
type SessionSummary struct {
	ID          string         `json:"id"`
	ScenarioID  string         `json:"scenario_id,omitempty"`
	ProviderID  string         `json:"provider_id,omitempty"`
	Timestamp   time.Time      `json:"timestamp,omitzero"`
	TurnCount   int            `json:"turn_count,omitempty"`
	HasFailures bool           `json:"has_failures"`
	Tags        []string       `json:"tags,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// GetSessionRequest asks for one session in full.
type GetSessionRequest struct {
	DeployConfig string `json:"deploy_config"`
	Environment  string `json:"environment"`
	SessionID    string `json:"session_id"`
}

// GetSessionResponse carries the session.
type GetSessionResponse struct {
	Session SessionDetail `json:"session"`
}

// SessionDetail is everything a session recorded that a regression scenario
// can be built from. Every field beyond the summary and Messages is optional.
type SessionDetail struct {
	SessionSummary
	// Messages is a JSON array of PromptKit runtime types.Message: the
	// conversation with tool calls and results inline and multimodal parts
	// inline or by URI. Raw so this package stays off the runtime types; the
	// caller decodes it.
	Messages json.RawMessage `json:"messages"`
	// Pack identifies the pack the session ran against, as far as known.
	Pack *SessionPack `json:"pack,omitempty"`
	// Variables are the template variables the run was invoked with.
	Variables map[string]string `json:"variables,omitempty"`
	// Workflow is the recorded workflow trace, when the session ran one.
	Workflow *SessionWorkflow `json:"workflow,omitempty"`
	// Evals are every recorded eval observation: measurements (kind "eval")
	// and judged assertions/guardrails, at both levels.
	Evals []SessionEval `json:"evals,omitempty"`
}

// SessionPack identifies a pack. Any field may be empty.
type SessionPack struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	Digest  string `json:"digest,omitempty"`
}

// SessionWorkflow is a recorded workflow trace.
type SessionWorkflow struct {
	EntryState      string              `json:"entry_state,omitempty"`
	EntryPromptTask string              `json:"entry_prompt_task,omitempty"`
	Transitions     []SessionTransition `json:"transitions,omitempty"`
}

// SessionTransition is one recorded state change. MessageIndex is the index
// into Messages of the last message before it; -1 when unknown.
type SessionTransition struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Event        string `json:"event,omitempty"`
	PromptTask   string `json:"prompt_task,omitempty"`
	MessageIndex int    `json:"message_index"`
}

// SessionEval is one recorded eval observation. Kind is "eval" (a score, no
// verdict), "assertion" or "guardrail" (a verdict in Passed). Turn is nil for
// a conversation-level result, else the index of the user turn it answers.
type SessionEval struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Kind      string            `json:"kind,omitempty"`
	Score     *float64          `json:"score,omitempty"`
	Passed    *bool             `json:"passed,omitempty"`
	Params    map[string]any    `json:"params,omitempty"`
	Threshold *SessionThreshold `json:"threshold,omitempty"`
	Message   string            `json:"message,omitempty"`
	Details   map[string]any    `json:"details,omitempty"`
	Turn      *int              `json:"turn,omitempty"`
}

// SessionThreshold is the pack author's declared expectation for a score
// (packspec.EvalThreshold): Operator "gte", "lte", "gt", "lt" or "eq".
type SessionThreshold struct {
	Operator string  `json:"operator"`
	Value    float64 `json:"value"`
}
