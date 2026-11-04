package engine

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// defaultPassThreshold applies to an assertion that sets no pass_threshold:
// it must pass in every trial.
const defaultPassThreshold = 1.0

// thresholdEpsilon absorbs float error when comparing a pass rate to a
// threshold, so 2/3 meets pass_threshold: 0.6667 written as a decimal.
const thresholdEpsilon = 1e-9

// AggregateTrialResults groups trial run results by scenario+row+provider+region,
// computes statistical metrics, and updates the first run in each group with
// the aggregated TrialResults. Returns the run IDs that represent trial groups
// (i.e., the first run ID of each group that now carries the summary).
//
// Conversation assertions are judged against the default threshold of 1.0;
// the engine resolves each scenario's pass_threshold values itself.
func AggregateTrialResults(store *statestore.ArenaStateStore, runIDs []string, combos []RunCombination) []string {
	return aggregateTrialResults(store, runIDs, combos, nil, nil)
}

// conversationConfigs returns the conversation assertions a scenario
// evaluates, in result order, so their pass_threshold values can be read.
type conversationConfigs func(scenarioID string) []assertions.AssertionConfig

func aggregateTrialResults(
	store *statestore.ArenaStateStore, runIDs []string, combos []RunCombination, thresholds conversationConfigs,
	budgets *trialBudgets,
) []string {
	// Group run IDs by trial group key
	groups := groupTrialRuns(runIDs, combos)
	if len(groups) == 0 {
		return runIDs
	}

	var summaryIDs []string
	for _, group := range groups {
		budget := budgets.result(group.key)
		// A budget that stopped a group after its first trial still reports
		// how it ended, so that group is aggregated too.
		if len(group.runIDs) <= 1 && (budget == nil || !group.trials) {
			summaryIDs = append(summaryIDs, group.runIDs...)
			continue
		}

		var convConfigs []assertions.AssertionConfig
		if thresholds != nil {
			convConfigs = thresholds(group.key.ScenarioID)
		}
		results := loadTrialResults(store, group.runIDs, convConfigs)
		trialResults := computeTrialResults(results)
		trialResults.Budget = budget
		summaryID := representativeTrial(results)
		attachTrialResults(store, summaryID, trialResults)
		applyTrialVerdict(store, results, summaryID, trialResults)
		summaryIDs = append(summaryIDs, summaryID)
	}

	return summaryIDs
}

// representativeTrial picks the run that stands for a trial group in reports:
// the first trial that ran without error, else the first trial. A failing turn
// assertion errors its run, so a trial that errored can't show a group that
// met its thresholds.
func representativeTrial(trials []trialRunResult) string {
	for _, t := range trials {
		if !t.conversationError {
			return t.runID
		}
	}
	return trials[0].runID
}

// thresholdsFor returns the conversation assertions a scenario evaluates:
// arena globals first, then the scenario's own, matching the order the
// executors evaluate them in.
func (e *Engine) thresholdsFor(scenarioID string) []assertions.AssertionConfig {
	sc := e.scenarios[scenarioID]
	if sc == nil {
		return nil
	}
	return mergeAssertionConfigs(e.config, sc.ConversationAssertions)
}

type trialGroup struct {
	key    TrialGroupKey
	runIDs []string
	// trials is set for a group of a scenario's trials, as opposed to a
	// single run.
	trials bool
}

func groupTrialRuns(runIDs []string, combos []RunCombination) []trialGroup {
	// Use ordered approach to maintain deterministic output
	keyOrder := make([]TrialGroupKey, 0)
	groupMap := make(map[TrialGroupKey][]string)

	for i, combo := range combos {
		if combo.TotalTrials <= 1 {
			continue
		}
		if i >= len(runIDs) || runIDs[i] == "" {
			continue
		}
		key := trialGroupKey(combo)
		if _, exists := groupMap[key]; !exists {
			keyOrder = append(keyOrder, key)
		}
		groupMap[key] = append(groupMap[key], runIDs[i])
	}

	groups := make([]trialGroup, 0, len(keyOrder))
	for _, key := range keyOrder {
		groups = append(groups, trialGroup{key: key, runIDs: groupMap[key], trials: true})
	}

	// Also include non-trial run IDs as single-entry groups
	for i, combo := range combos {
		if combo.TotalTrials > 1 {
			continue
		}
		if i >= len(runIDs) || runIDs[i] == "" {
			continue
		}
		groups = append(groups, trialGroup{
			key:    trialGroupKey(combo),
			runIDs: []string{runIDs[i]},
		})
	}

	return groups
}

func trialGroupKey(combo RunCombination) TrialGroupKey {
	return TrialGroupKey{
		ScenarioID: combo.ScenarioID,
		DataRowID:  combo.DataRowID,
		ProviderID: combo.ProviderID,
		Region:     combo.Region,
	}
}

// trialOutcome is one assertion's result in one trial.
type trialOutcome struct {
	assertionType string
	name          string
	passed        bool
	threshold     float64
}

type trialRunResult struct {
	runID     string
	allPassed bool
	// assertionResults maps an assertion key (see conversationAssertionKey
	// and turnAssertionKey) to its outcome in this trial.
	assertionResults  map[string]trialOutcome
	conversationError bool
	errorMessage      string
	// failedTurnAssertion is set when a turn assertion failed, which is what
	// ends a run with an error part way through.
	failedTurnAssertion bool
	cost                float64
	objectives          []ObjectiveOutcome
	// unreliableTurns holds the turns whose persona invented a specific.
	unreliableTurns map[int]bool
}

// conversationAssertionKey identifies a conversation assertion across trials
// by its position, which is fixed by the config.
func conversationAssertionKey(index int, assertionType string) string {
	return fmt.Sprintf("conversation[%d]:%s", index, assertionType)
}

// turnAssertionKey identifies a turn assertion across trials by the ordinal of
// the assistant message carrying it, its configured type and params, and its
// occurrence among identically configured assertions on that message.
// Position alone isn't stable: assertions skipped by `when` are listed first.
// The result's message isn't used: without a configured message it is the
// handler's explanation, which differs from trial to trial.
func turnAssertionKey(turn int, identity string, occurrence int) string {
	return fmt.Sprintf("%s%d]:%s#%d", turnKeyPrefix, turn, identity, occurrence)
}

const turnKeyPrefix = "turn["

func loadTrialResults(
	store *statestore.ArenaStateStore, runIDs []string, convConfigs []assertions.AssertionConfig,
) []trialRunResult {
	results := make([]trialRunResult, 0, len(runIDs))
	for _, runID := range runIDs {
		rr, err := store.GetResult(nil, runID) //nolint:staticcheck // nil ctx is fine for in-memory store
		if err != nil || rr == nil {
			results = append(results, trialRunResult{
				runID: runID, conversationError: true, errorMessage: "run result not found",
			})
			continue
		}

		tr := trialRunResult{
			runID:             runID,
			allPassed:         rr.ConversationAssertions.Passed && rr.Error == "",
			assertionResults:  make(map[string]trialOutcome),
			conversationError: rr.Error != "",
			errorMessage:      rr.Error,
		}

		for i, ar := range rr.ConversationAssertions.Results {
			if endedEarly(&ar) {
				// Counted like a missing result: a failure of an errored
				// trial, unless a failing turn assertion stopped it.
				continue
			}
			tr.assertionResults[conversationAssertionKey(i, ar.Type)] = trialOutcome{
				assertionType: ar.Type,
				name:          conversationAssertionName(i, ar),
				passed:        ar.Passed,
				threshold:     conversationThreshold(convConfigs, i, ar.Type),
			}
		}
		collectTurnOutcomes(&tr, rr.Messages)
		tr.cost = rr.Cost.TotalCost
		tr.objectives = ObjectiveOutcomes(rr.ConversationAssertions.Results)
		tr.unreliableTurns = UnreliableTurns(rr.ConversationAssertions.Results)

		results = append(results, tr)
	}
	return results
}

// conversationThreshold returns the pass_threshold of the conversation
// assertion that produced result i. Results follow config order; a result
// whose type doesn't match its config (an appended objective result, say)
// uses the default.
func conversationThreshold(configs []assertions.AssertionConfig, i int, resultType string) float64 {
	if i < len(configs) && configs[i].Type == resultType && configs[i].PassThreshold != nil {
		return *configs[i].PassThreshold
	}
	return defaultPassThreshold
}

func conversationAssertionName(i int, ar assertions.ConversationValidationResult) string {
	return fmt.Sprintf("conversation assertion %d (%s)", i+1, ar.Type)
}

// collectTurnOutcomes adds a trial's turn assertion outcomes, leaving out
// assertions skipped by `when`.
func collectTurnOutcomes(tr *trialRunResult, messages []types.Message) {
	forEachTurnAssertion(messages, func(key string, turn int, result map[string]interface{}) {
		if skipped, _ := result["skipped"].(bool); skipped {
			return
		}
		passed, _ := result["passed"].(bool)
		if !passed {
			tr.allPassed = false
			tr.failedTurnAssertion = true
		}
		assertionType, _ := result["type"].(string)
		tr.assertionResults[key] = trialOutcome{
			assertionType: assertionType,
			name:          turnAssertionName(turn, assertionType, result),
			passed:        passed,
			threshold:     turnThreshold(result),
		}
	})
}

func turnAssertionName(turn int, assertionType string, result map[string]interface{}) string {
	name := fmt.Sprintf("turn %d %s", turn+1, assertionType)
	cfg, _ := result["config"].(map[string]interface{})
	if msg, _ := cfg["message"].(string); msg != "" {
		name += " (" + msg + ")"
	}
	return name
}

// forEachTurnAssertion visits every turn assertion result recorded on the
// assistant messages, with its key and the 0-based ordinal of its message.
func forEachTurnAssertion(messages []types.Message, fn func(key string, turn int, result map[string]interface{})) {
	turn := 0
	for i := range messages {
		results := trialTurnResults(&messages[i])
		if results == nil {
			continue
		}
		seen := make(map[string]int)
		for _, result := range results {
			id := turnAssertionIdentity(result)
			fn(turnAssertionKey(turn, id, seen[id]), turn, result)
			seen[id]++
		}
		turn++
	}
}

// turnAssertionIdentity is what an assertion was configured as: its config's
// type and params, falling back to the result's type when there's no config.
func turnAssertionIdentity(result map[string]interface{}) string {
	cfg, _ := result["config"].(map[string]interface{})
	assertionType, _ := cfg["type"].(string)
	if assertionType == "" {
		assertionType, _ = result["type"].(string)
	}
	params, err := json.Marshal(cfg["params"]) // map keys marshal sorted
	if err != nil || cfg["params"] == nil {
		return assertionType
	}
	return assertionType + string(params)
}

// trialTurnResults returns the assertion results on an assistant message, or
// nil when it carries none. It accepts the list shapes the assertion stage
// ([]interface{}) and the duplex executor ([]map[string]interface{}) write.
// The maps are the message's own, so writes to them update the message.
func trialTurnResults(msg *types.Message) []map[string]interface{} {
	if msg.Role != roleAssistant || msg.Meta == nil {
		return nil
	}
	summary, ok := msg.Meta["assertions"].(map[string]interface{})
	if !ok {
		return nil
	}
	switch list := summary["results"].(type) {
	case []map[string]interface{}:
		return list
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(list))
		for _, r := range list {
			if m, ok := r.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// turnThreshold reads the pass_threshold the assertion stage records in a turn
// result's config.
func turnThreshold(result map[string]interface{}) float64 {
	cfg, _ := result["config"].(map[string]interface{})
	if t, ok := cfg["pass_threshold"].(float64); ok {
		return t
	}
	return defaultPassThreshold
}

func computeTrialResults(trials []trialRunResult) *TrialResults {
	// Overall pass/fail per trial
	overallResults := make([]bool, len(trials))
	errored := 0
	for i, t := range trials {
		overallResults[i] = t.allPassed
		if t.conversationError {
			errored++
		}
	}

	// Collect assertion keys across all trials
	assertionKeys := collectAssertionTypes(trials)

	perAssertion := make(map[string]AssertionTrialStats, len(assertionKeys))
	allMet := true
	for _, key := range assertionKeys {
		stats := assertionTrialStats(trials, key)
		allMet = allMet && stats.Passed
		perAssertion[key] = stats
	}

	return &TrialResults{
		TrialCount:        len(trials),
		PassRate:          evals.PassRate(overallResults),
		FlakinessScore:    evals.FlakinessScore(overallResults),
		PerAssertionStats: perAssertion,
		// A group needs at least one trial that ran without error, and with
		// no assertions to judge it needs every trial to have run.
		Passed:        allMet && errored < len(trials) && (len(assertionKeys) > 0 || errored == 0),
		ErroredTrials: errored,
		TotalCost:     totalTrialCost(trials),
		Objectives:    objectiveTrialStats(trials),
	}
}

// assertionTrialStats judges one assertion across trials. A trial that errored
// for a reason other than a failing turn assertion counts as a failure of
// every assertion it has no result for: it never got the chance to pass. A
// trial stopped by a failing turn assertion doesn't count against the turns
// it never reached, since that failure is already counted once.
func assertionTrialStats(trials []trialRunResult, key string) AssertionTrialStats {
	results := make([]bool, 0, len(trials))
	var first *trialOutcome
	for _, t := range trials {
		if o, ok := t.assertionResults[key]; ok {
			results = append(results, o.passed)
			if first == nil {
				first = &o
			}
		} else if t.conversationError && !t.failedTurnAssertion {
			results = append(results, false)
		}
	}
	passCount := 0
	for _, r := range results {
		if r {
			passCount++
		}
	}
	passRate := evals.PassRate(results)
	return AssertionTrialStats{
		Type:           first.assertionType,
		Name:           first.name,
		PassRate:       passRate,
		PassCount:      passCount,
		FailCount:      len(results) - passCount,
		FlakinessScore: evals.FlakinessScore(results),
		PassThreshold:  first.threshold,
		Passed:         passRate+thresholdEpsilon >= first.threshold,
	}
}

func collectAssertionTypes(trials []trialRunResult) []string {
	seen := make(map[string]bool)
	var types []string
	for _, t := range trials {
		for aType := range t.assertionResults {
			if !seen[aType] {
				seen[aType] = true
				types = append(types, aType)
			}
		}
	}
	return types
}

func attachTrialResults(store *statestore.ArenaStateStore, runID string, tr *TrialResults) {
	_ = store.SetTrialResults(nil, runID, tr) //nolint:staticcheck // nil ctx is fine for in-memory store
}

// applyTrialVerdict rewrites the assertion results of the run that represents
// a trial group so every report judges the group, not one trial: each
// assertion passes when its pass rate across trials meets its pass_threshold,
// and its message says how many trials it passed. A conversation assertion
// that falls short but has no result on this run is added as a failed one.
// When the group fails for any other reason (a turn assertion below its
// threshold, or trials that errored) the run's error says why, as a failing
// turn assertion or an error does on a single run. TrialResults.Passed and
// the run's outcome therefore always agree.
func applyTrialVerdict(store *statestore.ArenaStateStore, trials []trialRunResult, runID string, tr *TrialResults) {
	state, err := store.GetArenaState(nil, runID) //nolint:staticcheck // nil ctx is fine for in-memory store
	if err != nil || state.RunMetadata == nil {
		return
	}
	shown := make(map[string]bool, len(tr.PerAssertionStats))

	convResults := state.RunMetadata.ConversationAssertionResults
	for i := range convResults {
		key := conversationAssertionKey(i, convResults[i].Type)
		stats, ok := tr.PerAssertionStats[key]
		if !ok {
			continue
		}
		shown[key] = true
		convResults[i].Passed = stats.Passed
		convResults[i].Message = trialMessage(convResults[i].Message, stats)
		convResults[i].Details = withTrialDetails(convResults[i].Details, stats)
	}

	messages := state.Messages
	forEachTurnAssertion(messages, func(key string, _ int, result map[string]interface{}) {
		stats, ok := tr.PerAssertionStats[key]
		if !ok {
			return
		}
		result["passed"] = stats.Passed
		message, _ := result["message"].(string)
		result["message"] = trialMessage(message, stats)
		details, _ := result["details"].(map[string]interface{})
		result["details"] = withTrialDetails(details, stats)
	})
	recountTurnSummaries(messages)

	convFailed := false
	var turnFailures []string
	for _, key := range failedKeys(tr) {
		stats := tr.PerAssertionStats[key]
		if strings.HasPrefix(key, turnKeyPrefix) {
			turnFailures = append(turnFailures, trialMessage(stats.Name, stats))
			continue
		}
		convFailed = true
		if !shown[key] {
			convResults = append(convResults, assertions.ConversationValidationResult{
				Type:    stats.Type,
				Passed:  false,
				Message: trialMessage(stats.Name, stats),
				Details: withTrialDetails(nil, stats),
			})
		}
	}

	runErr := state.RunMetadata.Error
	switch {
	case runErr != "" || tr.Passed:
	case len(turnFailures) > 0:
		runErr = fmt.Sprintf("turn assertions below pass_threshold across %d trials: %s",
			tr.TrialCount, strings.Join(turnFailures, "; "))
	case !convFailed:
		runErr = erroredTrialsMessage(trials)
	}
	_ = store.SetAssertionResults(nil, runID, messages, convResults, runErr) //nolint:staticcheck // nil ctx is fine
}

// erroredTrialsMessage explains a group that failed only because trials
// errored, quoting the first error.
func erroredTrialsMessage(trials []trialRunResult) string {
	errored := 0
	first := ""
	for _, t := range trials {
		if t.conversationError {
			errored++
			if first == "" {
				first = t.errorMessage
			}
		}
	}
	return fmt.Sprintf("%d of %d trials errored: %s", errored, len(trials), first)
}

// failedKeys lists, in a stable order, the assertions that fell short of
// their threshold.
func failedKeys(tr *TrialResults) []string {
	var keys []string
	for key, stats := range tr.PerAssertionStats {
		if !stats.Passed {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// recountTurnSummaries refreshes each message's assertion summary after its
// results were re-judged across trials. The assertion stage writes `passed`;
// the duplex executor writes `all_passed`.
func recountTurnSummaries(messages []types.Message) {
	for i := range messages {
		results := trialTurnResults(&messages[i])
		if results == nil {
			continue
		}
		failed := 0
		for _, result := range results {
			if passed, _ := result["passed"].(bool); !passed {
				failed++
			}
		}
		summary := messages[i].Meta["assertions"].(map[string]interface{})
		summary["failed"] = failed
		if _, ok := summary["all_passed"]; ok {
			summary["all_passed"] = failed == 0
		} else {
			summary["passed"] = failed == 0
		}
	}
}

func trialMessage(message string, stats AssertionTrialStats) string {
	return fmt.Sprintf("%s (passed %d of %d trials, pass_threshold %.2f)",
		message, stats.PassCount, stats.PassCount+stats.FailCount, stats.PassThreshold)
}

func withTrialDetails(details map[string]interface{}, stats AssertionTrialStats) map[string]interface{} {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["trials"] = map[string]interface{}{
		"pass_count":     stats.PassCount,
		"trial_count":    stats.PassCount + stats.FailCount,
		"pass_rate":      stats.PassRate,
		"pass_threshold": stats.PassThreshold,
	}
	return details
}
