package engine

import (
	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
)

// ObjectiveOutcome is what happened to one self-play turn objective in one
// run, read back from the run's objective conversation result.
type ObjectiveOutcome struct {
	// TurnIndex is the scenario turn the objective is on.
	TurnIndex int
	// Description is the objective's description.
	Description string
	// Expect is the outcome that passes the run: "achieved" or "not_achieved".
	Expect string
	// Achieved reports whether the objective was reached.
	Achieved bool
	// AchievedAtExchange is the 1-based exchange at which it was reached, or 0.
	AchievedAtExchange int
	// Exchanges is how many self-play exchanges ran.
	Exchanges int
	// Evaluated is false when the success checks could not be evaluated. An
	// objective already achieved stays evaluated even if a later check errored.
	Evaluated bool
}

// ObjectiveOutcomes reads the objective outcomes from a run's conversation
// assertion results, in turn order. Results of other types are skipped.
func ObjectiveOutcomes(results []asrt.ConversationValidationResult) []ObjectiveOutcome {
	var out []ObjectiveOutcome
	for i := range results {
		if results[i].Type != objectiveResultType {
			continue
		}
		d := results[i].Details
		_, failed := d["error"]
		achieved, _ := d["achieved"].(bool)
		description, _ := d[objectiveKeyDescription].(string)
		expect, _ := d["expect"].(string)
		if expect == "" {
			expect = arenaconfig.ObjectiveExpectAchieved
		}
		out = append(out, ObjectiveOutcome{
			TurnIndex:          detailInt(d, objectiveKeyTurnIndex),
			Description:        description,
			Expect:             expect,
			Achieved:           achieved,
			AchievedAtExchange: detailInt(d, "achieved_at_exchange"),
			Exchanges:          detailInt(d, "exchanges"),
			Evaluated:          !failed || achieved,
		})
	}
	return out
}

// detailInt reads an integer detail that may have been through a JSON round
// trip, which turns it into a float64.
func detailInt(d map[string]interface{}, key string) int {
	switch v := d[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

// totalTrialCost sums the cost of every trial.
func totalTrialCost(trials []trialRunResult) float64 {
	total := 0.0
	for i := range trials {
		total += trials[i].cost
	}
	return total
}

// objectiveTrialStats summarises each objective across trials, keyed by the
// turn it is on, in the order first seen. A trial that errored without
// recording an objective another trial recorded counts as unevaluated for it:
// the run never got far enough to be judged.
func objectiveTrialStats(trials []trialRunResult) []ObjectiveTrialStats {
	var out []ObjectiveTrialStats
	index := map[int]int{}
	for i := range trials {
		for _, o := range trials[i].objectives {
			pos, ok := index[o.TurnIndex]
			if !ok {
				pos = len(out)
				index[o.TurnIndex] = pos
				out = append(out, ObjectiveTrialStats{TurnIndex: o.TurnIndex, Description: o.Description, Expect: o.Expect})
			}
			s := &out[pos]
			if !o.Evaluated {
				s.Unevaluated++
				continue
			}
			if trials[i].unreliableTurns[o.TurnIndex] {
				s.Unreliable++
				continue
			}
			s.Attempts++
			if o.Achieved {
				s.Achieved++
				s.AchievedAtExchange = append(s.AchievedAtExchange, o.AchievedAtExchange)
			}
		}
	}
	for i := range trials {
		if !trials[i].conversationError {
			continue
		}
		for pos := range out {
			if !hasObjective(trials[i].objectives, out[pos].TurnIndex) {
				out[pos].Unevaluated++
			}
		}
	}
	return out
}

func hasObjective(outcomes []ObjectiveOutcome, turnIndex int) bool {
	for _, o := range outcomes {
		if o.TurnIndex == turnIndex {
			return true
		}
	}
	return false
}
