package objectives

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/engine"
)

func searched(r assertions.ConversationValidationResult, depth, nodes, pruned int, best float64,
) assertions.ConversationValidationResult {
	r.Details["search"] = map[string]interface{}{
		"depth_reached": depth, "nodes": nodes, "pruned_off_topic": pruned,
		"attacker_calls": nodes + pruned, "target_calls": nodes, "judge_calls": 2 * nodes, "best_score": best,
	}
	return r
}

func TestBuild_SearchTotals(t *testing.T) {
	runs := []engine.RunResult{
		run("r1", "tap", 1, nil, nil, searched(objective("get a refund", true, 2, ""), 2, 5, 1, 10)),
		run("r2", "tap", 1, nil, nil, searched(objective("get a refund", false, 0, ""), 4, 12, 3, 6)),
		run("r3", "linear", 1, nil, nil, objective("get a refund", false, 0, "")),
	}
	report := Build(runs, []string{GroupScenario})

	total := report.Totals[0].Search
	require.NotNil(t, total)
	assert.Equal(t, engine.SearchStats{Searches: 2, DepthReached: 6, Nodes: 17, PrunedOffTopic: 4,
		AttackerCalls: 21, TargetCalls: 17, JudgeCalls: 34, BestScore: 10}, *total)

	groups := report.Groupings[0].Groups
	require.Len(t, groups, 2)
	assert.Nil(t, groups[0].Search, "linear runs have no search")
	assert.NotNil(t, groups[1].Search)

	md := RenderMarkdown(report)
	assert.Contains(t, md, "| Tree search |")
	assert.Contains(t, md, "17 branches, 4 pruned, depth 3.0, best 10.0; calls: 21 attacker, 17 target, 34 judge")
	assert.Contains(t, md, "| – |\n", "a group without a search shows a dash")
}

func TestBuild_SearchTotalsFromTrials(t *testing.T) {
	r := run("r1", "tap", 1, nil, nil, objective("get a refund", true, 1, ""))
	r.TrialResults = &engine.TrialResults{Objectives: []engine.ObjectiveTrialStats{{
		Description: "get a refund", Attempts: 2, Achieved: 1,
		Search: &engine.SearchStats{Searches: 2, DepthReached: 3, Nodes: 6},
	}}}
	report := Build([]engine.RunResult{r}, nil)
	require.NotNil(t, report.Totals[0].Search)
	assert.Equal(t, 6, report.Totals[0].Search.Nodes)
}

func TestRenderMarkdown_NoSearchColumnWithoutSearch(t *testing.T) {
	report := Build([]engine.RunResult{run("r1", "s", 1, nil, nil, objective("x", true, 1, ""))}, nil)
	assert.NotContains(t, RenderMarkdown(report), "Tree search")
	assert.Equal(t, "–", searchCell(nil))
}
