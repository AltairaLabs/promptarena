package generate

import (
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// The helpers in this file are the store-independent half of turning stored
// records into a SessionDetail. Every source, in-process or behind a deploy
// adapter, should use them rather than re-deriving the rules, so two sources
// fed the same records produce the same scenarios.

// ViolatesAny reports whether the session's evals violate at least one
// expectation. A session that never measured an expectation's eval, or
// measured it without a score, violates it: it cannot satisfy a range it has
// no value for. No expectations means nothing is violated.
func ViolatesAny(evals []EvalResult, expectations []Expectation) bool {
	for _, e := range expectations {
		var score *float64
		found := false
		for i := range evals {
			if evals[i].ID == e.EvalID {
				score, found = evals[i].Score, true
				break
			}
		}
		if !found || e.Violated(score) {
			return true
		}
	}
	return false
}

// HasFailedVerdict reports whether any eval recorded a negative verdict. A
// measurement with a score and no verdict never counts, however low it is;
// whether it is out of range is an Expectation's question.
func HasFailedVerdict(evals []EvalResult) bool {
	for i := range evals {
		if evals[i].Failed() {
			return true
		}
	}
	return false
}

// UserTurns returns, for each message position, the index of the user turn
// that message belongs to: a user message opens a turn, and the assistant
// reply and any tool traffic after it belong to that turn. Messages before the
// first user message (a system prompt) belong to no turn and get -1. This is
// the numbering EvalResult.Turn uses, which is not the message position.
func UserTurns(messages []types.Message) []int {
	out := make([]int, len(messages))
	turn := -1
	for i := range messages {
		if messages[i].Role == roleUser {
			turn++
		}
		out[i] = turn
	}
	return out
}

// UserTurnIndex maps a store's message ids to the user turn each message
// belongs to. ids[i] is the store's id for messages[i]; empty ids, and
// messages before the first user message, are left out.
func UserTurnIndex(messages []types.Message, ids []string) map[string]int {
	turns := UserTurns(messages)
	out := make(map[string]int, len(ids))
	for i, id := range ids {
		if id == "" || i >= len(turns) || turns[i] < 0 {
			continue
		}
		out[id] = turns[i]
	}
	return out
}

// EvalRow is one eval observation as a store typically records it: against
// the id of the message it evaluated rather than a user-turn index. An empty
// MessageID is a conversation-level result.
type EvalRow struct {
	ID        string
	Type      string
	Kind      string
	Score     *float64
	Passed    *bool
	Params    map[string]interface{}
	Message   string
	Details   map[string]interface{}
	MessageID string
}

// EvalResultsFromRows turns stored eval rows into EvalResults, resolving each
// row's MessageID to the user turn it answers through turnIndex (from
// UserTurnIndex). A row whose message id is empty or unknown is kept as a
// conversation-level result rather than dropped. Kind, Score and Passed are
// carried as recorded: a score with no verdict keeps Passed nil.
func EvalResultsFromRows(rows []EvalRow, turnIndex map[string]int) []EvalResult {
	if len(rows) == 0 {
		return nil
	}
	out := make([]EvalResult, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = EvalResult{
			ID: r.ID, Type: r.Type, Kind: r.Kind, Score: r.Score, Passed: r.Passed,
			Params: r.Params, Message: r.Message, Details: r.Details,
		}
		if t, ok := turnIndex[r.MessageID]; ok && r.MessageID != "" {
			out[i].Turn = &t
		}
	}
	return out
}
