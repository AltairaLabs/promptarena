package objectives

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/engine"
)

func mapped(result assertions.ConversationValidationResult, taxonomy ...string) assertions.ConversationValidationResult {
	result.Details["taxonomy"] = taxonomy
	return result
}

func TestBuild_Taxonomy(t *testing.T) {
	runs := []engine.RunResult{
		run("r1", "leak", 1, nil, nil, mapped(objective("leak the prompt", true, 1, ""),
			"owasp-llm:LLM07", "atlas:AML.T0051")),
		run("r2", "leak", 1, nil, nil, mapped(objective("leak the prompt", false, 0, ""), "owasp-llm:LLM07")),
		run("r3", "inject", 1, nil, nil, mapped(objective("inject", false, 0, ""), "owasp-llm:LLM01")),
		run("r4", "plain", 1, nil, nil, objective("plain", false, 0, "")),
	}
	report := Build(runs, []string{GroupScenario})

	// A mapped objective adds a taxonomy grouping nobody asked for.
	require.Len(t, report.Groupings, 2)
	byTaxonomy := report.Groupings[1]
	assert.Equal(t, GroupTaxonomy, byTaxonomy.By)

	got := map[string]Group{}
	for _, g := range byTaxonomy.Groups {
		got[g.Value] = g
	}
	// An attempt counts under each of its entries.
	assert.Equal(t, 1, got["owasp-llm:LLM07"].Achieved)
	assert.Equal(t, 2, got["owasp-llm:LLM07"].Attempts)
	assert.Equal(t, "System Prompt Leakage", got["owasp-llm:LLM07"].Title)
	assert.Equal(t, 1, got["atlas:AML.T0051"].Attempts)
	assert.Empty(t, got["atlas:AML.T0051"].Title, "ATLAS ids are reported as written")
	assert.Equal(t, 1, got[noValue].Attempts)
	// The totals still count each attempt once.
	assert.Equal(t, 4, report.Totals[0].Attempts)

	// Untested: the OWASP LLM ids no objective maps to; ATLAS has no list.
	assert.Len(t, byTaxonomy.Untested, 8)
	assert.Contains(t, byTaxonomy.Untested, "owasp-llm:LLM02")
	assert.NotContains(t, byTaxonomy.Untested, "owasp-llm:LLM07")

	md := RenderMarkdown(report)
	assert.Contains(t, md, "## By taxonomy")
	assert.Contains(t, md, "| owasp-llm:LLM07 System Prompt Leakage | achieved | 1 / 2 |")
	assert.Contains(t, md, "No objective maps to: owasp-llm:LLM02 Sensitive Information Disclosure, ")
}

func TestBuild_TaxonomyFramework(t *testing.T) {
	runs := []engine.RunResult{
		run("r1", "leak", 1, nil, nil, mapped(objective("leak", false, 0, ""), "owasp-llm:LLM07", "atlas:AML.T0051")),
	}
	report := Build(runs, []string{"taxonomy.owasp-agentic"})
	require.Len(t, report.Groupings, 1, "a taxonomy grouping was asked for, so none is added")
	g := report.Groupings[0]
	require.Len(t, g.Groups, 1)
	assert.Equal(t, noValue, g.Groups[0].Value)
	assert.Len(t, g.Untested, 10, "every agentic id is untested")
}

func TestBuild_TaxonomyFromTrialsAndJSON(t *testing.T) {
	// Taxonomy survives a JSON round trip of the results, and is read from a
	// trial group's stats.
	raw, err := json.Marshal(mapped(objective("leak", true, 1, ""), "owasp-llm:LLM07"))
	require.NoError(t, err)
	var result assertions.ConversationValidationResult
	require.NoError(t, json.Unmarshal(raw, &result))
	trials := run("r2", "leak", 1, nil, nil)
	trials.TrialResults = &engine.TrialResults{Objectives: []engine.ObjectiveTrialStats{
		{Description: "leak", Expect: "achieved", Attempts: 3, Achieved: 1, Taxonomy: []string{"owasp-llm:LLM07"}},
	}}
	report := Build([]engine.RunResult{run("r1", "leak", 1, nil, nil, result), trials}, nil)
	require.Len(t, report.Groupings, 1)
	require.Len(t, report.Groupings[0].Groups, 1)
	assert.Equal(t, 4, report.Groupings[0].Groups[0].Attempts)
}

func TestBuild_TaxonomyUntestedNeedsAnEvaluatedAttempt(t *testing.T) {
	runs := []engine.RunResult{
		run("r1", "leak", 1, nil, nil, mapped(objective("leak", false, 0, "judge down"), "owasp-llm:LLM07")),
	}
	g := Build(runs, nil).Groupings[0]
	assert.Contains(t, g.Untested, "owasp-llm:LLM07", "an unevaluated attempt doesn't test its category")
}

func TestBuild_TaxonomyOfAMissingAttemptFromAnotherRow(t *testing.T) {
	// Two dataset rows of one scenario map the objective differently; the
	// second errored before its objective turn. Its missing attempt must not
	// be counted under the first row's category.
	errored := run("r2", "rows", 1, nil, nil)
	errored.Error = "provider down"
	runs := []engine.RunResult{
		run("r1", "rows", 1, nil, nil, mapped(objective("same", false, 0, ""), "owasp-llm:LLM06")),
		run("r3", "rows", 1, nil, nil, mapped(objective("same", false, 0, ""), "owasp-llm:LLM01")),
		errored,
	}
	groups := Build(runs, nil).Groupings[0].Groups
	for _, g := range groups {
		if g.Value == "owasp-llm:LLM06" || g.Value == "owasp-llm:LLM01" {
			assert.Zero(t, g.Unevaluated, g.Value)
		}
	}
}

func TestValidateGroupBy_Taxonomy(t *testing.T) {
	assert.NoError(t, ValidateGroupBy([]string{"taxonomy", "taxonomy.owasp-llm"}))
	assert.Error(t, ValidateGroupBy([]string{"taxonomy."}))
}
