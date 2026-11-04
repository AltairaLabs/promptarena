package engine

import (
	"reflect"
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
)

func outcomeResult(turn int, achieved bool, at int, evalErr string) asrt.ConversationValidationResult {
	d := map[string]interface{}{
		objectiveKeyTurnIndex:   turn,
		objectiveKeyDescription: "get a refund",
		"achieved":              achieved,
		"exchanges":             5,
	}
	if achieved {
		d["achieved_at_exchange"] = at
	}
	if evalErr != "" {
		d["error"] = evalErr
	}
	return asrt.ConversationValidationResult{Type: objectiveResultType, Details: d}
}

func TestObjectiveOutcomes(t *testing.T) {
	results := []asrt.ConversationValidationResult{
		{Type: "contains", Passed: true},
		outcomeResult(1, true, 3, ""),
		outcomeResult(2, false, 0, "judge unavailable"),
	}
	got := ObjectiveOutcomes(results)
	want := []ObjectiveOutcome{
		{TurnIndex: 1, Description: "get a refund", Expect: "achieved", Achieved: true, AchievedAtExchange: 3,
			Exchanges: 5, Evaluated: true},
		{TurnIndex: 2, Description: "get a refund", Expect: "achieved", Exchanges: 5, Evaluated: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outcomes =\n%+v\nwant\n%+v", got, want)
	}
}

func TestObjectiveOutcomes_AfterJSONRoundTrip(t *testing.T) {
	r := asrt.ConversationValidationResult{Type: objectiveResultType, Details: map[string]interface{}{
		objectiveKeyTurnIndex: float64(4), "achieved": true, "achieved_at_exchange": float64(2), "exchanges": float64(6),
	}}
	got := ObjectiveOutcomes([]asrt.ConversationValidationResult{r})
	if len(got) != 1 || got[0].TurnIndex != 4 || got[0].AchievedAtExchange != 2 || got[0].Exchanges != 6 {
		t.Fatalf("outcomes = %+v", got)
	}
}

func TestObjectiveTrialStats(t *testing.T) {
	trials := []trialRunResult{
		{cost: 0.5, objectives: ObjectiveOutcomes([]asrt.ConversationValidationResult{outcomeResult(1, true, 2, "")})},
		{cost: 0.25, objectives: ObjectiveOutcomes([]asrt.ConversationValidationResult{outcomeResult(1, false, 0, "")})},
		{cost: 0.25, objectives: ObjectiveOutcomes([]asrt.ConversationValidationResult{outcomeResult(1, true, 4, "")})},
		{objectives: ObjectiveOutcomes([]asrt.ConversationValidationResult{outcomeResult(1, false, 0, "boom")})},
	}
	got := objectiveTrialStats(trials)
	want := []ObjectiveTrialStats{{
		TurnIndex: 1, Description: "get a refund", Expect: "achieved", Attempts: 3, Achieved: 2,
		AchievedAtExchange: []int{2, 4}, Unevaluated: 1,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stats =\n%+v\nwant\n%+v", got, want)
	}
	if c := totalTrialCost(trials); c != 1.0 {
		t.Fatalf("total cost = %v", c)
	}
}

func TestComputeTrialResults_CarriesObjectivesAndCost(t *testing.T) {
	tr := computeTrialResults([]trialRunResult{
		{allPassed: true, cost: 0.1, objectives: ObjectiveOutcomes(
			[]asrt.ConversationValidationResult{outcomeResult(0, true, 1, "")})},
		{allPassed: true, cost: 0.2},
	})
	if len(tr.Objectives) != 1 || tr.Objectives[0].Achieved != 1 {
		t.Fatalf("objectives = %+v", tr.Objectives)
	}
	if tr.TotalCost < 0.299 || tr.TotalCost > 0.301 {
		t.Fatalf("total cost = %v", tr.TotalCost)
	}
}

func TestObjectiveTrialStats_ErroredTrialIsUnevaluated(t *testing.T) {
	trials := []trialRunResult{
		{objectives: ObjectiveOutcomes([]asrt.ConversationValidationResult{outcomeResult(1, true, 2, "")})},
		{conversationError: true},
	}
	got := objectiveTrialStats(trials)
	if len(got) != 1 || got[0].Attempts != 1 || got[0].Unevaluated != 1 {
		t.Fatalf("stats = %+v, want 1 attempt and 1 unevaluated", got)
	}
}

func TestObjectiveOutcomes_AchievedThenCheckErrored(t *testing.T) {
	r := outcomeResult(1, true, 2, "state store unavailable")
	r.Details["expect"] = "not_achieved"
	got := ObjectiveOutcomes([]asrt.ConversationValidationResult{r})
	if !got[0].Evaluated || !got[0].Achieved || got[0].Expect != "not_achieved" {
		t.Fatalf("an achieved objective stays evaluated after a later check error: %+v", got[0])
	}
}

func TestObjectiveTrialStats_ErroredTrialThatRecordedIsCountedOnce(t *testing.T) {
	trials := []trialRunResult{
		{conversationError: true, objectives: ObjectiveOutcomes(
			[]asrt.ConversationValidationResult{outcomeResult(1, false, 0, "")})},
	}
	got := objectiveTrialStats(trials)
	if got[0].Attempts != 1 || got[0].Unevaluated != 0 {
		t.Fatalf("stats = %+v, want the recorded attempt and nothing unevaluated", got[0])
	}
}

func TestObjectiveTrialStats_UnreliableTrialIsNotAnAttempt(t *testing.T) {
	trials := []trialRunResult{
		{objectives: ObjectiveOutcomes([]asrt.ConversationValidationResult{outcomeResult(1, true, 2, "")})},
		{unreliableTurns: map[int]bool{1: true}, objectives: ObjectiveOutcomes([]asrt.ConversationValidationResult{outcomeResult(1, true, 1, "")})},
		{unreliableTurns: map[int]bool{1: true}, objectives: ObjectiveOutcomes([]asrt.ConversationValidationResult{outcomeResult(1, false, 0, "boom")})},
	}
	got := objectiveTrialStats(trials)
	if len(got) != 1 || got[0].Attempts != 1 || got[0].Achieved != 1 || got[0].Unreliable != 1 || got[0].Unevaluated != 1 {
		t.Fatalf("stats = %+v", got)
	}
}

func TestObjectiveBaselineIsRecordedAndReadBack(t *testing.T) {
	o := &objectiveOutcome{turnIndex: 1, cfg: &arenaconfig.ObjectiveConfig{Description: "get a refund", Baseline: true}}
	r := o.result()
	if r.Details[objectiveKeyBaseline] != true {
		t.Fatalf("details = %+v, want baseline: true", r.Details)
	}
	if _, set := (&objectiveOutcome{cfg: &arenaconfig.ObjectiveConfig{}}).result().Details[objectiveKeyBaseline]; set {
		t.Fatal("an objective that is not a baseline has no baseline detail")
	}

	outcomes := ObjectiveOutcomes([]asrt.ConversationValidationResult{r})
	if len(outcomes) != 1 || !outcomes[0].Baseline {
		t.Fatalf("outcomes = %+v, want a baseline", outcomes)
	}
	stats := objectiveTrialStats([]trialRunResult{{objectives: outcomes}})
	if len(stats) != 1 || !stats[0].Baseline {
		t.Fatalf("stats = %+v, want a baseline", stats)
	}
}
