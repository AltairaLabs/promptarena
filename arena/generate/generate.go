package generate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
)

const (
	dirPerms  = 0o750
	filePerms = 0o600
)

// Request is everything Generate needs. Source is required; the rest is
// optional.
type Request struct {
	// Source supplies sessions. Any SessionSourceAdapter: local recordings, a
	// deploy adapter's sessions capability, or an in-process store.
	Source SessionSourceAdapter
	// List narrows which sessions are fetched.
	List ListOptions
	// Convert controls the scenario shape. When Convert.Expectations is empty,
	// List.Expectations is used, so one input selects and bounds.
	Convert ConvertOptions
	// Dedup drops sessions with the same failure fingerprint, keeping the first.
	Dedup bool
	// Pack, when set, supplies params and declared thresholds for recorded
	// evals whose source did not record them (matched by eval ID).
	Pack *packspec.Pack
}

// Skipped names a session that produced no scenario and why.
type Skipped struct {
	SessionID string
	Reason    string
}

// Result is what Generate produced. Nothing has been written to disk.
type Result struct {
	Scenarios    []*arenaconfig.ScenarioConfig
	Skipped      []Skipped
	Warnings     []string
	Decisions    []Decision
	Deduplicated int
}

// Generate lists sessions from the source, fetches each, optionally
// deduplicates, and converts each into a scenario. A session that fails to
// load or convert is recorded in Skipped rather than failing the run; a
// failure to list is fatal. It never prints and never writes files.
func Generate(ctx context.Context, req Request) (*Result, error) {
	if req.Source == nil {
		return nil, fmt.Errorf("generate: Source is required")
	}
	if len(req.Convert.Expectations) == 0 {
		req.Convert.Expectations = req.List.Expectations
	}

	summaries, err := req.Source.List(ctx, req.List)
	if err != nil {
		return nil, fmt.Errorf("listing sessions from %s: %w", req.Source.Name(), err)
	}

	res := &Result{}
	var sessions []*SessionDetail
	for i := range summaries {
		detail, getErr := req.Source.Get(ctx, summaries[i].ID)
		if getErr != nil {
			res.Skipped = append(res.Skipped, Skipped{SessionID: summaries[i].ID, Reason: getErr.Error()})
			continue
		}
		enrichFromPack(detail, req.Pack)
		sessions = append(sessions, detail)
	}

	if req.Dedup {
		before := len(sessions)
		sessions = DeduplicateSessions(sessions)
		res.Deduplicated = before - len(sessions)
	}

	for _, s := range sessions {
		conv, convErr := Convert(s, req.Convert)
		if convErr != nil {
			res.Skipped = append(res.Skipped, Skipped{SessionID: s.ID, Reason: convErr.Error()})
			continue
		}
		res.Scenarios = append(res.Scenarios, conv.Scenario)
		res.Warnings = append(res.Warnings, conv.Warnings...)
		res.Decisions = append(res.Decisions, conv.Decisions...)
	}
	return res, nil
}

// enrichFromPack fills Params and Threshold on evals the source recorded
// without them, from the pack's eval of the same ID. Recorded values win.
func enrichFromPack(detail *SessionDetail, pack *packspec.Pack) {
	if pack == nil || detail == nil {
		return
	}
	byID := make(map[string]*packspec.Eval, len(pack.Evals))
	for _, e := range pack.Evals {
		if e != nil {
			byID[e.ID] = e
		}
	}
	for i := range detail.Evals {
		def, ok := byID[detail.Evals[i].ID]
		if !ok {
			continue
		}
		if detail.Evals[i].Params == nil && len(def.Params) > 0 {
			detail.Evals[i].Params = def.Params
		}
		if detail.Evals[i].Threshold == nil && def.Threshold != nil && def.Threshold.Value != nil {
			detail.Evals[i].Threshold = &EvalThreshold{Operator: def.Threshold.Operator, Value: *def.Threshold.Value}
		}
	}
}

// WriteScenarios writes each scenario to <dir>/<name>.scenario.yaml, creating
// dir, and returns the paths written in order.
func WriteScenarios(dir string, scenarios []*arenaconfig.ScenarioConfig) ([]string, error) {
	if err := os.MkdirAll(dir, dirPerms); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}
	paths := make([]string, 0, len(scenarios))
	for _, sc := range scenarios {
		data, err := yaml.Marshal(sc)
		if err != nil {
			return paths, fmt.Errorf("marshal scenario %s: %w", sc.Metadata.Name, err)
		}
		path := filepath.Join(dir, sc.Metadata.Name+".scenario.yaml")
		if err := os.WriteFile(path, data, filePerms); err != nil {
			return paths, fmt.Errorf("writing %s: %w", path, err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}
