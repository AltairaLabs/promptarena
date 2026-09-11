// Package sources provides built-in SessionSourceAdapter implementations.
package sources

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/AltairaLabs/promptarena/arena/adapters"
	"github.com/AltairaLabs/promptarena/arena/assertions"
	"github.com/AltairaLabs/promptarena/arena/generate"

	"github.com/AltairaLabs/PromptKit/runtime/types"
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

		summary := buildSummary(ref.ID, msgs, meta)
		a.remember(summary.ID, ref.ID)
		if opts.FilterPassed != nil && summary.HasFailures == *opts.FilterPassed {
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
		SessionSummary:  buildSummary(path, msgs, meta),
		Messages:        msgs,
		EvalResults:     conversationResults(meta),
		TurnEvalResults: turnResults(msgs, meta),
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
// stands in for the session ID when the recording names none.
func buildSummary(path string, msgs []types.Message, meta *adapters.RecordingMetadata) generate.SessionSummary {
	summary := generate.SessionSummary{
		ID:        path,
		Source:    path,
		TurnCount: len(msgs),
	}
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
	summary.HasFailures = hasFailures(meta)
	return summary
}

func hasFailures(meta *adapters.RecordingMetadata) bool {
	for _, r := range meta.ConversationAssertions {
		if !r.Passed {
			return true
		}
	}
	for _, results := range meta.TurnAssertions {
		for _, r := range results {
			if !r.Passed {
				return true
			}
		}
	}
	return false
}

func conversationResults(meta *adapters.RecordingMetadata) []assertions.ConversationValidationResult {
	if meta == nil || len(meta.ConversationAssertions) == 0 {
		return nil
	}
	out := make([]assertions.ConversationValidationResult, len(meta.ConversationAssertions))
	for i, r := range meta.ConversationAssertions {
		out[i] = assertions.ConversationValidationResult{
			Type:    r.Type,
			Passed:  r.Passed,
			Message: r.Message,
			Details: r.Details,
		}
	}
	return out
}

// turnResults re-keys per-message assertion results by the user turn they
// answer. Recordings key them by message index (the assistant message they
// were evaluated against); the converter indexes scenario turns, which are the
// user messages, so an assistant message maps to the most recent user message
// before it.
func turnResults(msgs []types.Message, meta *adapters.RecordingMetadata) map[int][]generate.TurnEvalResult {
	if meta == nil || len(meta.TurnAssertions) == 0 {
		return nil
	}
	userTurnBefore := make([]int, len(msgs))
	turn := -1
	for i := range msgs {
		if msgs[i].Role == "user" {
			turn++
		}
		userTurnBefore[i] = turn
	}

	out := make(map[int][]generate.TurnEvalResult)
	for msgIdx, results := range meta.TurnAssertions {
		if msgIdx < 0 || msgIdx >= len(msgs) || userTurnBefore[msgIdx] < 0 {
			continue
		}
		key := userTurnBefore[msgIdx]
		for _, r := range results {
			out[key] = append(out[key], generate.TurnEvalResult{
				Type:    r.Type,
				Passed:  r.Passed,
				Message: r.Message,
				Params:  r.Params,
			})
		}
	}
	return out
}
