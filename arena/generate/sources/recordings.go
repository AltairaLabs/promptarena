// Package sources provides built-in SessionSourceAdapter implementations.
package sources

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/AltairaLabs/promptarena/arena/adapters"
	"github.com/AltairaLabs/promptarena/arena/generate"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// RecordingsAdapter serves sessions from files on disk through the recording
// adapter registry: arena run output (`out/*.json`, what `promptarena run`
// writes), PromptKit session recordings (`*.recording.json` or the event
// store's `*.jsonl`), and transcripts.
type RecordingsAdapter struct {
	source   string
	registry *adapters.Registry

	mu    sync.Mutex
	paths map[string]string // session ID -> file path, filled by List
}

// NewRecordingsAdapter creates a RecordingsAdapter that reads from the given source glob.
func NewRecordingsAdapter(source string) *RecordingsAdapter {
	return &RecordingsAdapter{
		source:   source,
		registry: adapters.NewRegistry(),
		paths:    make(map[string]string),
	}
}

// Name returns "recordings".
func (a *RecordingsAdapter) Name() string {
	return "recordings"
}

// List enumerates recording files and returns a summary for each.
// FilterPassed is honored against the recording's own assertion results;
// files that fail to load are skipped rather than failing the listing.
func (a *RecordingsAdapter) List(
	_ context.Context,
	opts generate.ListOptions,
) ([]generate.SessionSummary, error) {
	refs, err := a.registry.Enumerate(a.source, "")
	if err != nil {
		return nil, fmt.Errorf("enumerating recordings: %w", err)
	}

	var summaries []generate.SessionSummary
	for _, ref := range refs {
		msgs, meta, loadErr := a.registry.Load(ref)
		if loadErr != nil {
			continue // skip unloadable recordings
		}

		summary := buildSummary(ref.ID, msgs, meta, opts.Expectations)
		a.remember(summary.ID, ref.ID)
		if opts.FilterPassed != nil && summary.HasFailures == *opts.FilterPassed {
			continue
		}
		if len(opts.Expectations) > 0 && !violatesAny(evalsFrom(msgs, meta), opts.Expectations) {
			continue
		}
		summaries = append(summaries, summary)

		if opts.Limit > 0 && len(summaries) >= opts.Limit {
			break
		}
	}

	return summaries, nil
}

// Get loads a single recording and returns its full session detail. The
// argument is a session ID as returned by List (a run ID for arena output, the
// recording's session_id otherwise); a file path is accepted too, so a caller
// that already knows the file need not List first.
func (a *RecordingsAdapter) Get(ctx context.Context, sessionID string) (*generate.SessionDetail, error) {
	path, err := a.resolve(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	ref := adapters.RecordingReference{
		ID:     path,
		Source: a.source,
	}

	msgs, meta, err := a.registry.Load(ref)
	if err != nil {
		return nil, fmt.Errorf("loading recording %q: %w", sessionID, err)
	}

	return &generate.SessionDetail{
		SessionSummary: buildSummary(path, msgs, meta, nil),
		Messages:       msgs,
		Pack:           packRef(meta),
		Variables:      variables(meta),
		Workflow:       workflowTrace(meta),
		Evals:          evalsFrom(msgs, meta),
	}, nil
}

// remember records which file a session ID came from, so Get can take the ID
// List handed out rather than making the caller keep the path.
func (a *RecordingsAdapter) remember(sessionID, path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.paths[sessionID] = path
}

// resolve turns a session ID into a file path: from the index List built, else
// by treating the argument as a path, else by enumerating the source once to
// build the index. Unknown IDs are an error rather than a silent empty result.
func (a *RecordingsAdapter) resolve(ctx context.Context, sessionID string) (string, error) {
	a.mu.Lock()
	path, ok := a.paths[sessionID]
	a.mu.Unlock()
	if ok {
		return path, nil
	}
	if _, err := os.Stat(sessionID); err == nil {
		return sessionID, nil
	}
	if _, err := a.List(ctx, generate.ListOptions{}); err != nil {
		return "", fmt.Errorf("resolving session %q: %w", sessionID, err)
	}
	a.mu.Lock()
	path, ok = a.paths[sessionID]
	a.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("session %q not found in %s", sessionID, a.source)
	}
	return path, nil
}

// buildSummary lifts the summary fields out of the loaded recording. The path
// stands in for the session ID when the recording names none. HasFailures is
// true for a failed verdict or a violated expectation.
func buildSummary(
	path string, msgs []types.Message, meta *adapters.RecordingMetadata, expectations []generate.Expectation,
) generate.SessionSummary {
	summary := generate.SessionSummary{ID: path, Source: path, TurnCount: len(msgs)}
	if meta == nil {
		return summary
	}
	summary.Tags = meta.Tags
	summary.Metadata = meta.Extras
	if meta.SessionID != "" {
		summary.ID = meta.SessionID
	}
	if len(meta.Timestamps) > 0 && !meta.Timestamps[0].IsZero() {
		summary.Timestamp = meta.Timestamps[0]
	}
	if id, ok := meta.Extras["scenario_id"].(string); ok {
		summary.ScenarioID = id
	}
	if id, ok := meta.ProviderInfo["provider_id"].(string); ok {
		summary.ProviderID = id
	}
	evals := evalsFrom(msgs, meta)
	summary.HasFailures = hasFailedVerdict(evals) || violatesAny(evals, expectations)
	return summary
}

func hasFailedVerdict(evals []generate.EvalResult) bool {
	for i := range evals {
		if evals[i].Failed() {
			return true
		}
	}
	return false
}

// violatesAny reports whether any expectation is violated by the session's
// measurement of that eval. A session that never measured the eval violates
// it: it cannot satisfy a range it has no value for.
func violatesAny(evals []generate.EvalResult, expectations []generate.Expectation) bool {
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

func packRef(meta *adapters.RecordingMetadata) *generate.PackRef {
	if meta == nil {
		return nil
	}
	name, _ := meta.Extras["prompt_pack"].(string)
	if name == "" {
		return nil
	}
	return &generate.PackRef{Name: name}
}

// variables are the string-valued entries of the run's Params; numbers there
// are provider parameters, not template variables.
func variables(meta *adapters.RecordingMetadata) map[string]string {
	if meta == nil {
		return nil
	}
	params, _ := meta.Extras["params"].(map[string]interface{})
	var out map[string]string
	for k, v := range params {
		if str, ok := v.(string); ok {
			if out == nil {
				out = make(map[string]string)
			}
			out[k] = str
		}
	}
	return out
}

func workflowTrace(meta *adapters.RecordingMetadata) *generate.WorkflowTrace {
	if meta == nil || len(meta.WorkflowTransitions) == 0 {
		return nil
	}
	trace := &generate.WorkflowTrace{EntryState: meta.WorkflowTransitions[0].From}
	for _, t := range meta.WorkflowTransitions {
		trace.Transitions = append(trace.Transitions, generate.WorkflowTransition{
			From: t.From, To: t.To, Event: t.Event, PromptTask: t.PromptTask, MessageIndex: t.MessageIndex,
		})
	}
	return trace
}

// evalsFrom assembles every recorded eval: conversation-level judged results,
// pack measurements, and per-turn judged results re-keyed by the user turn
// they answer. Recordings key per-turn results by message index (the
// assistant message evaluated); the converter indexes scenario turns, which
// are user messages, so an assistant message maps to the most recent user
// message before it.
func evalsFrom(msgs []types.Message, meta *adapters.RecordingMetadata) []generate.EvalResult {
	if meta == nil {
		return nil
	}
	var out []generate.EvalResult
	for _, r := range meta.ConversationAssertions {
		out = append(out, toEval(r, nil))
	}
	for _, r := range meta.EvalResults {
		out = append(out, toEval(r, nil))
	}
	if len(meta.TurnAssertions) == 0 {
		return out
	}
	userTurnBefore := make([]int, len(msgs))
	turn := -1
	for i := range msgs {
		if msgs[i].Role == "user" {
			turn++
		}
		userTurnBefore[i] = turn
	}
	// Map iteration is unordered; sort so Evals, and therefore the generated
	// assertion order, is stable run to run.
	msgIdxs := make([]int, 0, len(meta.TurnAssertions))
	for msgIdx := range meta.TurnAssertions {
		msgIdxs = append(msgIdxs, msgIdx)
	}
	sort.Ints(msgIdxs)
	for _, msgIdx := range msgIdxs {
		if msgIdx < 0 || msgIdx >= len(msgs) || userTurnBefore[msgIdx] < 0 {
			continue
		}
		t := userTurnBefore[msgIdx]
		for _, r := range meta.TurnAssertions[msgIdx] {
			out = append(out, toEval(r, &t))
		}
	}
	return out
}

func toEval(r adapters.RecordedEval, turn *int) generate.EvalResult {
	var t *int
	if turn != nil {
		v := *turn
		t = &v
	}
	return generate.EvalResult{
		ID: r.ID, Type: r.Type, Kind: r.Kind, Score: r.Score, Passed: r.Passed,
		Params: r.Params, Message: r.Message, Details: r.Details, Turn: t,
	}
}
