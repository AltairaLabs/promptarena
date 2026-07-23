package results

import (
	"fmt"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/AltairaLabs/promptarena/arena/engine"
)

// SummaryBuilder helps build ResultSummary from RunResult slices
type SummaryBuilder struct {
	configFile string
	timestamp  time.Time
}

// NewSummaryBuilder creates a new summary builder
func NewSummaryBuilder(configFile string) *SummaryBuilder {
	return &SummaryBuilder{
		configFile: configFile,
		timestamp:  time.Now(),
	}
}

// BuildSummary creates a ResultSummary from the provided results
func (b *SummaryBuilder) BuildSummary(results []engine.RunResult) *ResultSummary {
	if len(results) == 0 {
		return &ResultSummary{
			TotalTests: 0,
			Timestamp:  b.timestamp,
			ConfigFile: b.configFile,
			RunIDs:     []string{},
		}
	}

	// Count results by status
	passed, failed := CountResultsByStatus(results)
	skipped := CountSkipped(results)
	totalTests := len(results)

	// Calculate performance metrics
	totalCost, totalTokens, totalDuration := CalculatePerformanceMetrics(results)
	averageCost := float64(0)
	if totalTests > 0 {
		averageCost = totalCost / float64(totalTests)
	}

	// Extract unique metadata
	runIDs := ExtractRunIDs(results)
	promptPacks := ExtractUniqueValues(results, func(r engine.RunResult) string { return r.PromptPack })
	scenarios := ExtractUniqueValues(results, func(r engine.RunResult) string { return r.ScenarioID })
	providers := ExtractUniqueValues(results, func(r engine.RunResult) string { return r.ProviderID })
	regions := ExtractUniqueValues(results, func(r engine.RunResult) string { return r.Region })

	return &ResultSummary{
		TotalTests:    totalTests,
		Passed:        passed,
		Failed:        failed,
		Errors:        0, // Arena doesn't distinguish between failures and errors currently
		Skipped:       skipped,
		TotalDuration: totalDuration,
		AverageCost:   averageCost,
		TotalCost:     totalCost,
		TotalTokens:   totalTokens,
		Timestamp:     b.timestamp,
		ConfigFile:    b.configFile,
		RunIDs:        runIDs,
		PromptPacks:   promptPacks,
		Scenarios:     scenarios,
		Providers:     providers,
		Regions:       regions,
	}
}

// RunPassed reports whether a single run passed. It is the one canonical
// "did this run pass?" predicate: the report counters (CountResultsByStatus,
// JSON/Markdown/JUnit summaries) and the TUI/console summary all decide pass
// vs fail from this same rule so they cannot diverge — the false-green in #39
// came from surfaces that consulted completion (Error == "") without consulting
// assertion outcomes.
//
// A run passed when it completed without error AND:
//   - every defined assertion (turn- and conversation-level) passed, AND
//   - any guardrail violations are accounted for by passing assertions
//     (a test that EXPECTS a guardrail to trigger); violations with no
//     assertions to explain them are unexpected and fail the run.
func RunPassed(result *engine.RunResult) bool {
	if result.Error != "" {
		return false
	}
	if HasAssertions(result) && !AllAssertionsPassed(result) {
		return false
	}
	// Violations with no assertions to account for them are unexpected.
	if len(result.Violations) > 0 && !HasAssertions(result) {
		return false
	}
	return true
}

// CountResultsByStatus counts passed and failed results using the shared
// RunPassed predicate. Skipped runs are neither passed nor failed — counting
// them as passed is the false-green that hides aborted runs, so they are
// surfaced separately (see CountSkipped).
func CountResultsByStatus(results []engine.RunResult) (passed, failed int) {
	for i := range results {
		if results[i].Skipped {
			continue
		}
		if RunPassed(&results[i]) {
			passed++
		} else {
			failed++
		}
	}
	return passed, failed
}

// CountSkipped counts results that were skipped (e.g. a transient provider
// error that was deliberately skipped rather than failed). Skipped runs are
// reported separately so they are never silently folded into the pass count.
func CountSkipped(results []engine.RunResult) int {
	skipped := 0
	for i := range results {
		if results[i].Skipped {
			skipped++
		}
	}
	return skipped
}

// AllAssertionsPassed checks if all assertions in the result passed.
// This includes both turn-level assertions (in message metadata) and
// conversation-level assertions.
func AllAssertionsPassed(result *engine.RunResult) bool {
	// Check conversation-level assertions
	if result.ConversationAssertions.Total > 0 && !result.ConversationAssertions.Passed {
		return false
	}

	// Check turn-level assertions in messages
	for i := range result.Messages {
		if meta := result.Messages[i].Meta; meta != nil {
			if assertions, ok := meta["assertions"].(map[string]interface{}); ok {
				if passed, ok := assertions["passed"].(bool); ok && !passed {
					return false
				}
			}
			// Also check eval results (Phase 3 dual-write)
			if evalResults, ok := meta["eval_results"].([]evals.EvalResult); ok {
				for j := range evalResults {
					passed, _ := evalResults[j].Value.(bool)
					if !passed {
						return false
					}
				}
			}
		}
	}

	return true
}

// HasAssertions checks if the result has any assertions defined.
// This is used to determine if violations should be treated as failures:
// - Violations WITH passing assertions = test passed (guardrails were expected)
// - Violations WITHOUT any assertions = test failed (guardrails were unexpected)
func HasAssertions(result *engine.RunResult) bool {
	// Check conversation-level assertions
	if result.ConversationAssertions.Total > 0 {
		return true
	}

	// Check turn-level assertions in messages
	for i := range result.Messages {
		if meta := result.Messages[i].Meta; meta != nil {
			if _, ok := meta["assertions"].(map[string]interface{}); ok {
				return true
			}
			// Also check eval results (Phase 3 dual-write)
			if _, ok := meta["eval_results"]; ok {
				return true
			}
		}
	}

	return false
}

// CalculatePerformanceMetrics calculates cost, token, and duration totals
func CalculatePerformanceMetrics(results []engine.RunResult) (totalCost float64, totalTokens int, totalDuration time.Duration) {
	for _, result := range results {
		totalCost += result.Cost.TotalCost
		totalTokens += result.Cost.InputTokens + result.Cost.OutputTokens
		totalDuration += result.Duration
	}
	return totalCost, totalTokens, totalDuration
}

// ExtractRunIDs extracts all run IDs from results
func ExtractRunIDs(results []engine.RunResult) []string {
	runIDs := make([]string, len(results))
	for i, result := range results {
		runIDs[i] = result.RunID
	}
	return runIDs
}

// ExtractUniqueValues extracts unique values using the provided extractor function
func ExtractUniqueValues(results []engine.RunResult, extractor func(engine.RunResult) string) []string {
	seen := make(map[string]bool)
	var values []string

	for _, result := range results {
		value := extractor(result)
		if value != "" && !seen[value] {
			seen[value] = true
			values = append(values, value)
		}
	}

	return values
}

// ValidateResults performs basic validation on results before processing
func ValidateResults(results []engine.RunResult) error {
	if results == nil {
		return NewValidationError("results", results, "results cannot be nil")
	}

	for i, result := range results {
		if result.RunID == "" {
			return NewValidationError("RunID", result.RunID, fmt.Sprintf("result %d has empty RunID", i))
		}
		if result.ScenarioID == "" {
			return NewValidationError("ScenarioID", result.ScenarioID, fmt.Sprintf("result %d has empty ScenarioID", i))
		}
		if result.ProviderID == "" {
			return NewValidationError("ProviderID", result.ProviderID, fmt.Sprintf("result %d has empty ProviderID", i))
		}
	}

	return nil
}
