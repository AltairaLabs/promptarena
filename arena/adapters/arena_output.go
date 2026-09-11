package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/types"
)

// ArenaOutputAdapter loads the per-run JSON that `promptarena run` writes into
// its output directory, one file per scenario/provider combination. That file
// is engine.RunResult marshalled: the run's identity, its full message list as
// PromptKit messages (tool calls and results inline), and both levels of
// assertion result — conversation-level at the top, per-turn on each assistant
// message under meta.assertions.
type ArenaOutputAdapter struct{}

// NewArenaOutputAdapter creates a new arena output adapter.
func NewArenaOutputAdapter() *ArenaOutputAdapter {
	return &ArenaOutputAdapter{}
}

// CanHandle returns true for .json files that are not PromptKit session
// recordings, or for an "arena_output" type hint. Run output has no special
// extension — it is whatever `promptarena run` wrote — so the check is by
// content at Load time; here only the session-recording suffix is excluded.
func (a *ArenaOutputAdapter) CanHandle(source, typeHint string) bool {
	if typeHint != "" {
		// An explicit hint names the format; never steal a hinted source by
		// extension from the adapter the hint was meant for.
		return matchesTypeHint(typeHint, "arena", "arena_output", "scenario_output")
	}
	return hasExtension(source, ".json") && !hasExtension(source, ".recording.json")
}

// Enumerate expands a source into individual recording references.
// For file-based sources, this expands glob patterns to matching files.
func (a *ArenaOutputAdapter) Enumerate(source string) ([]RecordingReference, error) {
	return EnumerateFiles(source, "arena_output")
}

// runOutputFile is the subset of engine.RunResult's JSON this adapter reads.
// Declared locally because engine imports this package.
type runOutputFile struct {
	RunID       string                 `json:"RunID"`
	PromptPack  string                 `json:"PromptPack"`
	Region      string                 `json:"Region"`
	ScenarioID  string                 `json:"ScenarioID"`
	ProviderID  string                 `json:"ProviderID"`
	Params      map[string]interface{} `json:"Params"`
	Messages    []types.Message        `json:"Messages"`
	StartTime   time.Time              `json:"StartTime"`
	EndTime     time.Time              `json:"EndTime"`
	Duration    time.Duration          `json:"Duration"`
	Error       string                 `json:"Error"`
	SessionTags []string               `json:"SessionTags"`

	ConversationAssertions assertionsSummary `json:"conversation_assertions"`
}

// assertionsSummary mirrors engine.AssertionsSummary, which is also the shape
// of an assistant message's meta.assertions. Each result additionally carries
// the assertion's original config, which the engine's typed result drops.
type assertionsSummary struct {
	Results []recordedAssertionResult `json:"results"`
}

type recordedAssertionResult struct {
	Type    string                 `json:"type"`
	Passed  bool                   `json:"passed"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details"`
	Config  struct {
		Params map[string]interface{} `json:"params"`
	} `json:"config"`
}

// Load reads a run output file and returns its messages and metadata.
func (a *ArenaOutputAdapter) Load(ref RecordingReference) ([]types.Message, *RecordingMetadata, error) {
	data, err := os.ReadFile(ref.ID) //nolint:gosec // path comes from the user's config or CLI
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read arena output: %w", err)
	}

	var run runOutputFile
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, nil, fmt.Errorf("failed to parse arena output JSON %s: %w", ref.ID, err)
	}
	if run.RunID == "" {
		return nil, nil, fmt.Errorf(
			"%s is not arena run output (no RunID); expected a file written by `promptarena run`", ref.ID)
	}

	return run.Messages, metadataFromRun(&run), nil
}

// metadataFromRun lifts the run's identity, timing and assertion results.
func metadataFromRun(run *runOutputFile) *RecordingMetadata {
	meta := &RecordingMetadata{
		SessionID: run.RunID,
		Tags:      run.SessionTags,
		Duration:  run.Duration,
		Extras:    make(map[string]interface{}),
	}
	if meta.Duration == 0 && !run.EndTime.IsZero() && !run.StartTime.IsZero() {
		meta.Duration = run.EndTime.Sub(run.StartTime)
	}
	if run.ProviderID != "" {
		meta.ProviderInfo = map[string]interface{}{"provider_id": run.ProviderID}
	}
	putExtra(meta.Extras, "scenario_id", run.ScenarioID)
	putExtra(meta.Extras, "prompt_pack", run.PromptPack)
	putExtra(meta.Extras, "region", run.Region)
	putExtra(meta.Extras, "error", run.Error)
	if len(run.Params) > 0 {
		meta.Extras["params"] = run.Params
	}

	meta.Timestamps = make([]time.Time, len(run.Messages))
	for i := range run.Messages {
		ts := run.Messages[i].Timestamp
		if ts.IsZero() {
			ts = run.StartTime
		}
		meta.Timestamps[i] = ts
	}

	meta.ConversationAssertions = recordedAssertions(run.ConversationAssertions.Results)
	for i := range run.Messages {
		turn := turnAssertions(&run.Messages[i])
		if len(turn) == 0 {
			continue
		}
		if meta.TurnAssertions == nil {
			meta.TurnAssertions = make(map[int][]RecordedAssertion)
		}
		meta.TurnAssertions[i] = turn
	}
	return meta
}

// turnAssertions decodes meta.assertions off an assistant message. The engine
// stores it as a generic map, so it goes through JSON to reach the typed shape.
func turnAssertions(msg *types.Message) []RecordedAssertion {
	raw, ok := msg.Meta["assertions"]
	if !ok {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var summary assertionsSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil
	}
	return recordedAssertions(summary.Results)
}

func recordedAssertions(results []recordedAssertionResult) []RecordedAssertion {
	if len(results) == 0 {
		return nil
	}
	out := make([]RecordedAssertion, len(results))
	for i, r := range results {
		out[i] = RecordedAssertion{
			Type:    r.Type,
			Passed:  r.Passed,
			Message: r.Message,
			Params:  r.Config.Params,
			Details: r.Details,
		}
	}
	return out
}

func putExtra(extras map[string]interface{}, key, value string) {
	if value != "" {
		extras[key] = value
	}
}
