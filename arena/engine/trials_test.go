package engine

import (
	"context"
	"math"
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"

	runtimestore "github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func TestGroupTrialRuns(t *testing.T) {
	t.Run("no trials", func(t *testing.T) {
		combos := []RunCombination{
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 1},
		}
		runIDs := []string{"run-1"}
		groups := groupTrialRuns(runIDs, combos)
		if len(groups) != 1 {
			t.Fatalf("expected 1 group, got %d", len(groups))
		}
		if len(groups[0].runIDs) != 1 {
			t.Fatalf("expected 1 run in group, got %d", len(groups[0].runIDs))
		}
	})

	t.Run("multiple trials grouped", func(t *testing.T) {
		combos := []RunCombination{
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 0},
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 1},
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 2},
		}
		runIDs := []string{"run-1", "run-2", "run-3"}
		groups := groupTrialRuns(runIDs, combos)
		if len(groups) != 1 {
			t.Fatalf("expected 1 group, got %d", len(groups))
		}
		if len(groups[0].runIDs) != 3 {
			t.Fatalf("expected 3 runs in group, got %d", len(groups[0].runIDs))
		}
	})

	t.Run("mixed trials and non-trials", func(t *testing.T) {
		combos := []RunCombination{
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 2, TrialIndex: 0},
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 2, TrialIndex: 1},
			{ScenarioID: "s2", ProviderID: "p1", Region: "r1", TotalTrials: 1},
		}
		runIDs := []string{"run-1", "run-2", "run-3"}
		groups := groupTrialRuns(runIDs, combos)
		if len(groups) != 2 {
			t.Fatalf("expected 2 groups, got %d", len(groups))
		}
	})

	t.Run("skips empty run IDs", func(t *testing.T) {
		combos := []RunCombination{
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 0},
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 1},
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 2},
		}
		runIDs := []string{"run-1", "", "run-3"}
		groups := groupTrialRuns(runIDs, combos)
		if len(groups) != 1 {
			t.Fatalf("expected 1 group, got %d", len(groups))
		}
		if len(groups[0].runIDs) != 2 {
			t.Fatalf("expected 2 runs (skipping empty), got %d", len(groups[0].runIDs))
		}
	})
}

func TestComputeTrialResults(t *testing.T) {
	t.Run("all pass", func(t *testing.T) {
		trials := []trialRunResult{
			{runID: "r1", allPassed: true, assertionResults: outcomes(map[string]bool{"contains": true})},
			{runID: "r2", allPassed: true, assertionResults: outcomes(map[string]bool{"contains": true})},
			{runID: "r3", allPassed: true, assertionResults: outcomes(map[string]bool{"contains": true})},
		}
		result := computeTrialResults(trials)
		if result.TrialCount != 3 {
			t.Fatalf("expected 3 trials, got %d", result.TrialCount)
		}
		if result.PassRate != 1.0 {
			t.Fatalf("expected pass rate 1.0, got %f", result.PassRate)
		}
		if result.FlakinessScore != 0 {
			t.Fatalf("expected flakiness 0, got %f", result.FlakinessScore)
		}
		stats := result.PerAssertionStats["contains"]
		if stats.PassRate != 1.0 {
			t.Fatalf("expected assertion pass rate 1.0, got %f", stats.PassRate)
		}
	})

	t.Run("mixed results", func(t *testing.T) {
		trials := []trialRunResult{
			{runID: "r1", allPassed: true, assertionResults: outcomes(map[string]bool{"contains": true, "regex": true})},
			{runID: "r2", allPassed: false, assertionResults: outcomes(map[string]bool{"contains": true, "regex": false})},
			{runID: "r3", allPassed: false, assertionResults: outcomes(map[string]bool{"contains": false, "regex": false})},
		}
		result := computeTrialResults(trials)
		if result.TrialCount != 3 {
			t.Fatalf("expected 3 trials, got %d", result.TrialCount)
		}
		// 1/3 pass overall
		if math.Abs(result.PassRate-1.0/3.0) > 1e-9 {
			t.Fatalf("expected pass rate ~0.333, got %f", result.PassRate)
		}
		// contains: 2/3 pass
		containsStats := result.PerAssertionStats["contains"]
		if math.Abs(containsStats.PassRate-2.0/3.0) > 1e-9 {
			t.Fatalf("expected contains pass rate ~0.667, got %f", containsStats.PassRate)
		}
		if containsStats.PassCount != 2 || containsStats.FailCount != 1 {
			t.Fatalf("expected 2 pass, 1 fail, got %d/%d", containsStats.PassCount, containsStats.FailCount)
		}
		// regex: 1/3 pass
		regexStats := result.PerAssertionStats["regex"]
		if math.Abs(regexStats.PassRate-1.0/3.0) > 1e-9 {
			t.Fatalf("expected regex pass rate ~0.333, got %f", regexStats.PassRate)
		}
	})

	t.Run("all fail", func(t *testing.T) {
		trials := []trialRunResult{
			{runID: "r1", allPassed: false, assertionResults: outcomes(map[string]bool{"contains": false})},
			{runID: "r2", allPassed: false, assertionResults: outcomes(map[string]bool{"contains": false})},
		}
		result := computeTrialResults(trials)
		if result.PassRate != 0 {
			t.Fatalf("expected pass rate 0, got %f", result.PassRate)
		}
		if result.FlakinessScore != 0 {
			t.Fatalf("expected flakiness 0 (deterministic failure), got %f", result.FlakinessScore)
		}
	})

	t.Run("50/50 is maximally flaky", func(t *testing.T) {
		trials := []trialRunResult{
			{runID: "r1", allPassed: true, assertionResults: outcomes(map[string]bool{})},
			{runID: "r2", allPassed: false, assertionResults: outcomes(map[string]bool{})},
		}
		result := computeTrialResults(trials)
		if result.FlakinessScore != 1.0 {
			t.Fatalf("expected flakiness 1.0, got %f", result.FlakinessScore)
		}
	})
}

func TestGenerateScenarioCombinations_Trials(t *testing.T) {
	// Test that trial expansion works in generateScenarioCombinations
	// by verifying the RunCombination fields
	combos := []RunCombination{
		{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 0},
		{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 1},
		{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 2},
	}

	for i, c := range combos {
		if c.TrialIndex != i {
			t.Errorf("combo %d: expected TrialIndex %d, got %d", i, i, c.TrialIndex)
		}
		if c.TotalTrials != 3 {
			t.Errorf("combo %d: expected TotalTrials 3, got %d", i, c.TotalTrials)
		}
	}
}

func TestAggregateTrialResults(t *testing.T) {
	t.Run("single run no aggregation", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		combos := []RunCombination{
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 1},
		}
		runIDs := []string{"run-1"}
		summaryIDs := AggregateTrialResults(store, runIDs, combos)
		if len(summaryIDs) != 1 {
			t.Fatalf("expected 1 summary ID, got %d", len(summaryIDs))
		}
		if summaryIDs[0] != "run-1" {
			t.Fatalf("expected run-1, got %s", summaryIDs[0])
		}
	})

	t.Run("multi-trial aggregation end-to-end", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		ctx := context.Background()

		// Set up 3 trial runs with mixed results
		setupTrialRun(t, store, ctx, "run-a", true, []assertions.ConversationValidationResult{
			{Type: "contains", Passed: true, Message: "ok"},
		})
		setupTrialRun(t, store, ctx, "run-b", false, []assertions.ConversationValidationResult{
			{Type: "contains", Passed: false, Message: "fail"},
		})
		setupTrialRun(t, store, ctx, "run-c", true, []assertions.ConversationValidationResult{
			{Type: "contains", Passed: true, Message: "ok"},
		})

		combos := []RunCombination{
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 0},
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 1},
			{ScenarioID: "s1", ProviderID: "p1", Region: "r1", TotalTrials: 3, TrialIndex: 2},
		}
		runIDs := []string{"run-a", "run-b", "run-c"}

		summaryIDs := AggregateTrialResults(store, runIDs, combos)
		if len(summaryIDs) != 1 {
			t.Fatalf("expected 1 summary ID, got %d", len(summaryIDs))
		}
		if summaryIDs[0] != "run-a" {
			t.Fatalf("expected run-a, got %s", summaryIDs[0])
		}

		// Verify the trial results were attached to the first run
		rr, err := store.GetResult(ctx, "run-a")
		if err != nil {
			t.Fatalf("GetResult: %v", err)
		}
		if rr.TrialResults == nil {
			t.Fatal("expected TrialResults to be attached")
		}
	})
}

func TestLoadTrialResults(t *testing.T) {
	t.Run("loads results from store", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		ctx := context.Background()

		setupTrialRun(t, store, ctx, "r1", true, []assertions.ConversationValidationResult{
			{Type: "contains", Passed: true, Message: "ok"},
			{Type: "regex", Passed: true, Message: "ok"},
		})
		setupTrialRun(t, store, ctx, "r2", false, []assertions.ConversationValidationResult{
			{Type: "contains", Passed: true, Message: "ok"},
			{Type: "regex", Passed: false, Message: "fail"},
		})

		results := loadTrialResults(store, []string{"r1", "r2"}, nil)
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		if !results[0].allPassed {
			t.Fatal("expected r1 to pass")
		}
		if results[1].allPassed {
			t.Fatal("expected r2 to fail")
		}
		if !results[0].assertionResults["conversation[0]:contains"].passed {
			t.Fatal("expected r1 contains to pass")
		}
		if results[1].assertionResults["conversation[1]:regex"].passed {
			t.Fatal("expected r2 regex to fail")
		}
	})

	t.Run("missing run returns conversation error", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		results := loadTrialResults(store, []string{"nonexistent"}, nil)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].conversationError {
			t.Fatal("expected conversationError for missing run")
		}
	})

	t.Run("run with error marked as conversation error", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		ctx := context.Background()

		setupTrialRunWithError(t, store, ctx, "r-err", "provider timeout")

		results := loadTrialResults(store, []string{"r-err"}, nil)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].conversationError {
			t.Fatal("expected conversationError for errored run")
		}
		if results[0].allPassed {
			t.Fatal("expected allPassed to be false for errored run")
		}
	})
}

func TestAttachTrialResults(t *testing.T) {
	t.Run("attaches to existing run", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		ctx := context.Background()

		setupTrialRun(t, store, ctx, "run-1", true, nil)

		tr := &TrialResults{
			TrialCount:     3,
			PassRate:       2.0 / 3.0,
			FlakinessScore: 0.5,
		}
		attachTrialResults(store, "run-1", tr)

		rr, err := store.GetResult(ctx, "run-1")
		if err != nil {
			t.Fatalf("GetResult: %v", err)
		}
		if rr.TrialResults == nil {
			t.Fatal("expected TrialResults to be attached")
		}
	})

	t.Run("no-op for missing run", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		tr := &TrialResults{TrialCount: 3, PassRate: 1.0}
		// Should not panic
		attachTrialResults(store, "nonexistent", tr)
	})
}

// setupTrialRun creates a run in the statestore with the given assertion results.
func setupTrialRun(t *testing.T, store *statestore.ArenaStateStore, ctx context.Context, runID string, passed bool, cvResults []assertions.ConversationValidationResult) {
	t.Helper()
	if err := store.Save(ctx, &runtimestore.ConversationState{
		ID: runID,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	meta := &statestore.RunMetadata{
		RunID:                        runID,
		ConversationAssertionResults: cvResults,
	}
	if err := store.SaveMetadata(ctx, runID, meta); err != nil {
		t.Fatalf("SaveMetadata: %v", err)
	}
}

// setupTrialRunWithError creates a run with an error in the statestore.
func setupTrialRunWithError(t *testing.T, store *statestore.ArenaStateStore, ctx context.Context, runID, errMsg string) {
	t.Helper()
	if err := store.Save(ctx, &runtimestore.ConversationState{
		ID: runID,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	meta := &statestore.RunMetadata{
		RunID: runID,
		Error: errMsg,
	}
	if err := store.SaveMetadata(ctx, runID, meta); err != nil {
		t.Fatalf("SaveMetadata: %v", err)
	}
}

// outcomes builds trial outcomes that use the default pass_threshold.
func outcomes(passed map[string]bool) map[string]trialOutcome {
	out := make(map[string]trialOutcome, len(passed))
	for k, p := range passed {
		out[k] = trialOutcome{assertionType: k, passed: p, threshold: defaultPassThreshold}
	}
	return out
}

func ptr(f float64) *float64 { return &f }

// turnMessages returns a user/assistant exchange whose assistant message
// carries turn assertion results in the shape the assertion stage writes.
func turnMessages(results ...map[string]interface{}) []types.Message {
	list := make([]interface{}, len(results))
	failed := 0
	for i, r := range results {
		list[i] = r
		if !r["passed"].(bool) {
			failed++
		}
	}
	return []types.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello", Meta: map[string]interface{}{
			"assertions": map[string]interface{}{
				"results": list, "passed": failed == 0, "failed": failed, "total": len(list),
			},
		}},
	}
}

func turnResult(assertionType string, passed bool, threshold *float64) map[string]interface{} {
	cfg := map[string]interface{}{"type": assertionType}
	if threshold != nil {
		cfg["pass_threshold"] = *threshold
	}
	return map[string]interface{}{
		"type": assertionType, "passed": passed, "message": "msg", "config": cfg,
		"details": map[string]interface{}{},
	}
}

func saveTrialRun(
	t *testing.T, store *statestore.ArenaStateStore, runID string, msgs []types.Message,
	conv []assertions.ConversationValidationResult, runErr string,
) {
	t.Helper()
	ctx := context.Background()
	if err := store.Save(ctx, &runtimestore.ConversationState{ID: runID, Messages: msgs}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	meta := &statestore.RunMetadata{RunID: runID, ConversationAssertionResults: conv, Error: runErr}
	if err := store.SaveMetadata(ctx, runID, meta); err != nil {
		t.Fatalf("SaveMetadata: %v", err)
	}
}

func trialCombos(n int) []RunCombination {
	combos := make([]RunCombination, n)
	for i := range combos {
		combos[i] = RunCombination{ScenarioID: "s1", ProviderID: "p1", TotalTrials: n, TrialIndex: i}
	}
	return combos
}

func TestAggregateTrialResults_PassThreshold(t *testing.T) {
	conv := func(passed bool) []assertions.ConversationValidationResult {
		return []assertions.ConversationValidationResult{{Type: "contains", Passed: passed, Message: "says hi"}}
	}

	t.Run("conversation assertion meeting its threshold passes the group", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		saveTrialRun(t, store, "a", nil, conv(false), "")
		saveTrialRun(t, store, "b", nil, conv(true), "")
		saveTrialRun(t, store, "c", nil, conv(true), "")
		thresholds := func(string) []assertions.AssertionConfig {
			return []assertions.AssertionConfig{{Type: "contains", PassThreshold: ptr(0.66)}}
		}

		aggregateTrialResults(store, []string{"a", "b", "c"}, trialCombos(3), thresholds, nil)

		rr, _ := store.GetResult(context.Background(), "a")
		if !rr.ConversationAssertions.Passed {
			t.Fatalf("2 of 3 meets 0.66, want the group to pass: %+v", rr.ConversationAssertions)
		}
		got := rr.ConversationAssertions.Results[0]
		if got.Message != "says hi (passed 2 of 3 trials, pass_threshold 0.66)" {
			t.Fatalf("message = %q", got.Message)
		}
		if got.Details["trials"].(map[string]interface{})["pass_count"] != 2 {
			t.Fatalf("details = %v", got.Details)
		}
		tr := rr.TrialResults.(*TrialResults)
		if !tr.Passed || !tr.PerAssertionStats["conversation[0]:contains"].Passed {
			t.Fatalf("trial results = %+v", tr)
		}
	})

	t.Run("default threshold fails the group when a later trial fails", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		saveTrialRun(t, store, "a", nil, conv(true), "")
		saveTrialRun(t, store, "b", nil, conv(false), "")

		AggregateTrialResults(store, []string{"a", "b"}, trialCombos(2))

		rr, _ := store.GetResult(context.Background(), "a")
		if rr.ConversationAssertions.Passed {
			t.Fatal("the first trial passed but the second failed; default threshold 1.0 must fail the group")
		}
		if rr.TrialResults.(*TrialResults).Passed {
			t.Fatal("TrialResults.Passed should be false")
		}
	})

	t.Run("turn assertions are judged against the threshold in their config", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		saveTrialRun(t, store, "a", turnMessages(
			turnResult("min_length", false, ptr(0.5)), turnResult("content_includes", true, nil)), nil, "")
		saveTrialRun(t, store, "b", turnMessages(
			turnResult("min_length", true, ptr(0.5)), turnResult("content_includes", false, nil)), nil, "")

		aggregateTrialResults(store, []string{"a", "b"}, trialCombos(2), nil, nil)

		state, _ := store.GetArenaState(context.Background(), "a")
		summary := state.Messages[1].Meta["assertions"].(map[string]interface{})
		results := summary["results"].([]interface{})
		minLength := results[0].(map[string]interface{})
		contains := results[1].(map[string]interface{})
		if minLength["passed"] != true {
			t.Fatalf("min_length passed 1 of 2 against 0.5, want pass: %v", minLength)
		}
		if contains["passed"] != false {
			t.Fatalf("content_includes passed 1 of 2 against 1.0, want fail: %v", contains)
		}
		if summary["passed"] != false || summary["failed"] != 1 {
			t.Fatalf("summary not recounted: %v", summary)
		}
	})

	t.Run("skipped turn assertions are left out", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		skipped := turnResult("min_length", true, nil)
		skipped["skipped"] = true
		saveTrialRun(t, store, "a", turnMessages(skipped), nil, "")
		saveTrialRun(t, store, "b", turnMessages(turnResult("min_length", true, nil)), nil, "")

		aggregateTrialResults(store, []string{"a", "b"}, trialCombos(2), nil, nil)

		rr, _ := store.GetResult(context.Background(), "a")
		stats := rr.TrialResults.(*TrialResults).PerAssertionStats["turn[0]:min_length#0"]
		if stats.PassCount+stats.FailCount != 1 || !stats.Passed {
			t.Fatalf("stats = %+v, want 1 counted trial that passed", stats)
		}
	})

	t.Run("an errored trial counts against assertions it has no result for", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		saveTrialRun(t, store, "a", nil, conv(true), "")
		saveTrialRun(t, store, "b", nil, nil, "provider timeout")
		thresholds := func(string) []assertions.AssertionConfig {
			return []assertions.AssertionConfig{{Type: "contains", PassThreshold: ptr(0.6)}}
		}

		aggregateTrialResults(store, []string{"a", "b"}, trialCombos(2), thresholds, nil)

		rr, _ := store.GetResult(context.Background(), "a")
		tr := rr.TrialResults.(*TrialResults)
		if tr.Passed || tr.ErroredTrials != 1 {
			t.Fatalf("1 of 2 is below 0.6, want fail with 1 errored trial: %+v", tr)
		}
	})

	t.Run("a turn assertion below its threshold errors the representative run", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		saveTrialRun(t, store, "a", turnMessages(turnResult("min_length", true, nil)), nil, "")
		saveTrialRun(t, store, "b", turnMessages(turnResult("min_length", false, nil)), nil,
			"validation failed: min_length")

		aggregateTrialResults(store, []string{"a", "b"}, trialCombos(2), nil, nil)

		rr, _ := store.GetResult(context.Background(), "a")
		want := "turn assertions below pass_threshold across 2 trials: " +
			"turn 1 min_length (passed 1 of 2 trials, pass_threshold 1.00)"
		if rr.Error != want {
			t.Fatalf("error = %q, want %q", rr.Error, want)
		}
	})

	t.Run("a trial that errored doesn't represent a group that met its thresholds", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		saveTrialRun(t, store, "a", turnMessages(turnResult("min_length", false, ptr(0.5))), nil,
			"validation failed: min_length")
		saveTrialRun(t, store, "b", turnMessages(turnResult("min_length", true, ptr(0.5))), nil, "")

		ids := aggregateTrialResults(store, []string{"a", "b"}, trialCombos(2), nil, nil)

		if len(ids) != 1 || ids[0] != "b" {
			t.Fatalf("summary IDs = %v, want [b]", ids)
		}
		rr, _ := store.GetResult(context.Background(), "b")
		if rr.Error != "" || !rr.TrialResults.(*TrialResults).Passed {
			t.Fatalf("want a clean passing representative, got error %q", rr.Error)
		}
	})

	t.Run("a conversation assertion missing from the representative is added", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		saveTrialRun(t, store, "a", nil, nil, "")
		saveTrialRun(t, store, "b", nil, conv(false), "")

		aggregateTrialResults(store, []string{"a", "b"}, trialCombos(2), nil, nil)

		rr, _ := store.GetResult(context.Background(), "a")
		if rr.ConversationAssertions.Passed || len(rr.ConversationAssertions.Results) != 1 {
			t.Fatalf("want one added failing result: %+v", rr.ConversationAssertions)
		}
		if got := rr.ConversationAssertions.Results[0].Type; got != "contains" {
			t.Fatalf("added result type = %q", got)
		}
	})

	t.Run("every trial errored: the first represents the group", func(t *testing.T) {
		got := representativeTrial([]trialRunResult{
			{runID: "a", conversationError: true}, {runID: "b", conversationError: true},
		})
		if got != "a" {
			t.Fatalf("representative = %q", got)
		}
	})

	t.Run("turn results group by config even when explanations differ", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		a := turnResult("contains", false, ptr(0.5))
		a["message"] = "missing X"
		b := turnResult("contains", true, ptr(0.5))
		b["message"] = "found X"
		saveTrialRun(t, store, "a", turnMessages(a), nil, "validation failed")
		saveTrialRun(t, store, "b", turnMessages(b), nil, "")

		aggregateTrialResults(store, []string{"a", "b"}, trialCombos(2), nil, nil)

		rr, _ := store.GetResult(context.Background(), "b")
		tr := rr.TrialResults.(*TrialResults)
		if len(tr.PerAssertionStats) != 1 || !tr.Passed {
			t.Fatalf("want one assertion at 1 of 2 meeting 0.5: %+v", tr.PerAssertionStats)
		}
	})

	t.Run("a trial stopped by a failing turn assertion doesn't fail the turns it never reached", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		later := turnMessages(turnResult("max_length", true, nil))
		saveTrialRun(t, store, "a", turnMessages(turnResult("min_length", false, ptr(0.5))), nil,
			"validation failed: min_length")
		saveTrialRun(t, store, "b",
			append(turnMessages(turnResult("min_length", true, ptr(0.5))), later...), nil, "")

		aggregateTrialResults(store, []string{"a", "b"}, trialCombos(2), nil, nil)

		rr, _ := store.GetResult(context.Background(), "b")
		tr := rr.TrialResults.(*TrialResults)
		if !tr.Passed || rr.Error != "" {
			t.Fatalf("turn 2 ran once and passed; want the group to pass: %+v, error %q", tr.PerAssertionStats, rr.Error)
		}
	})

	t.Run("errored trials with no assertions to explain them set the run error", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		saveTrialRun(t, store, "a", nil, nil, "provider 500")
		saveTrialRun(t, store, "b", nil, nil, "")

		aggregateTrialResults(store, []string{"a", "b"}, trialCombos(2), nil, nil)

		rr, _ := store.GetResult(context.Background(), "b")
		if rr.Error != "1 of 2 trials errored: provider 500" {
			t.Fatalf("error = %q", rr.Error)
		}
	})

	t.Run("duplex-shaped turn results are judged too", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		duplex := func(passed bool) []types.Message {
			return []types.Message{{Role: "assistant", Content: "hi", Meta: map[string]interface{}{
				"assertions": map[string]interface{}{
					"results":    []map[string]interface{}{{"type": "contains", "passed": passed, "message": "m"}},
					"all_passed": passed,
				},
			}}}
		}
		saveTrialRun(t, store, "a", duplex(true), nil, "")
		saveTrialRun(t, store, "b", duplex(false), nil, "")

		aggregateTrialResults(store, []string{"a", "b"}, trialCombos(2), nil, nil)

		state, _ := store.GetArenaState(context.Background(), "a")
		if state.Messages[0].Meta["assertions"].(map[string]interface{})["all_passed"] != false {
			t.Fatal("all_passed should be recounted after the failing trial")
		}
		if state.RunMetadata.Error == "" {
			t.Fatal("a turn assertion below threshold should set the run error")
		}
	})

	t.Run("a conversation threshold only applies to the result of its own type", func(t *testing.T) {
		configs := []assertions.AssertionConfig{{Type: "contains", PassThreshold: ptr(0.5)}}
		if got := conversationThreshold(configs, 0, "objective"); got != defaultPassThreshold {
			t.Fatalf("mismatched type got %v", got)
		}
		if got := conversationThreshold(configs, 0, "contains"); got != 0.5 {
			t.Fatalf("matching type got %v", got)
		}
	})

	t.Run("no assertions: the group passes only when every trial ran", func(t *testing.T) {
		ok := computeTrialResults([]trialRunResult{{allPassed: true}, {allPassed: true}})
		bad := computeTrialResults([]trialRunResult{{allPassed: true}, {conversationError: true}})
		if !ok.Passed || bad.Passed {
			t.Fatalf("ok=%v bad=%v", ok.Passed, bad.Passed)
		}
	})
}

func TestEngineThresholdsFor(t *testing.T) {
	e := &Engine{
		config: &arenaconfig.Config{Globals: &arenaconfig.Globals{
			ConversationAssertions: []arenaconfig.AssertionConfig{{Type: "cost_budget"}},
		}},
		scenarios: map[string]*arenaconfig.Scenario{
			"s1": {ID: "s1", ConversationAssertions: []arenaconfig.AssertionConfig{
				{Type: "contains", PassThreshold: ptr(0.8)},
			}},
		},
	}
	got := e.thresholdsFor("s1")
	if len(got) != 2 || got[0].Type != "cost_budget" || got[1].PassThreshold == nil || *got[1].PassThreshold != 0.8 {
		t.Fatalf("configs = %+v, want cost_budget then contains at 0.8 (globals first)", got)
	}
	if e.thresholdsFor("missing") != nil {
		t.Fatal("unknown scenario should have no thresholds")
	}
}
