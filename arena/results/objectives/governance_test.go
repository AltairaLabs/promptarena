package objectives

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/engine"
)

var testGovernance = []arenaconfig.TaxonomyFramework{
	{
		Name: arenaconfig.TaxonomyObligation, IDs: []string{"refund-approval", "ai-disclosure"},
		Titles: map[string]string{
			"refund-approval": "Refunds need approval (controls: validator:refund-guard)",
			"ai-disclosure":   "Disclose AI",
		},
	},
	{
		Name: arenaconfig.TaxonomyMisuse, IDs: []string{"refunds-outside-policy"},
		Titles: map[string]string{"refunds-outside-policy": "Refunds outside policy"},
	},
}

func TestBuild_GovernanceTitlesAndUntested(t *testing.T) {
	runs := []engine.RunResult{
		run("r1", "refund", 1, nil, nil, mapped(objectiveOn(0, "not_achieved", "refund", true, 2, ""),
			"obligation:refund-approval", "owasp-llm:LLM06")),
		run("r2", "refund", 1, nil, nil, mapped(objectiveOn(0, "not_achieved", "refund", false, 0, ""),
			"obligation:refund-approval")),
	}
	report := Build(runs, nil, testGovernance...)

	require.Len(t, report.Groupings, 1)
	g := report.Groupings[0]
	got := map[string]Group{}
	for _, grp := range g.Groups {
		got[grp.Value] = grp
	}
	refund := got["obligation:refund-approval"]
	assert.Equal(t, 2, refund.Attempts)
	assert.Equal(t, 1, refund.Achieved, "one attempt got past the refund guard")
	assert.Equal(t, "Refunds need approval (controls: validator:refund-guard)", refund.Title)
	assert.Equal(t, "Excessive Agency", got["owasp-llm:LLM06"].Title, "built-in titles still apply")

	assert.Contains(t, g.Untested, "obligation:ai-disclosure")
	assert.Contains(t, g.Untested, "misuse:refunds-outside-policy",
		"declared governance is covered even when no objective maps to that framework")
	assert.NotContains(t, g.Untested, "obligation:refund-approval")

	md := RenderMarkdown(report)
	assert.Contains(t, md, "obligation:ai-disclosure Disclose AI")
	assert.Contains(t, md, "misuse:refunds-outside-policy Refunds outside policy")
}

func TestBuild_GovernanceAddsTaxonomyGroupingWithoutMappings(t *testing.T) {
	runs := []engine.RunResult{run("r1", "plain", 1, nil, nil, objective("plain", false, 0, ""))}

	assert.Empty(t, Build(runs, nil).Groupings)

	report := Build(runs, nil, testGovernance...)
	require.Len(t, report.Groupings, 1)
	assert.Equal(t, GroupTaxonomy, report.Groupings[0].By)
	assert.ElementsMatch(t, []string{
		"obligation:refund-approval", "obligation:ai-disclosure", "misuse:refunds-outside-policy",
	}, report.Groupings[0].Untested)
}

func TestBuild_GovernanceFrameworkGrouping(t *testing.T) {
	runs := []engine.RunResult{
		run("r1", "refund", 1, nil, nil, mapped(objective("refund", false, 0, ""), "misuse:refunds-outside-policy")),
	}
	report := Build(runs, []string{"taxonomy.obligation"}, testGovernance...)
	require.Len(t, report.Groupings, 1)
	assert.ElementsMatch(t, []string{"obligation:refund-approval", "obligation:ai-disclosure"},
		report.Groupings[0].Untested, "taxonomy.<framework> covers that framework only")
}

func TestRepository_SaveResultsWithGovernance(t *testing.T) {
	dir := t.TempDir()
	repo := NewRepository(dir, nil, testGovernance...)
	runs := []engine.RunResult{
		run("r1", "refund", 1, nil, nil, mapped(objective("refund", false, 0, ""), "obligation:refund-approval")),
	}
	require.NoError(t, repo.SaveResults(runs))
	md, err := os.ReadFile(filepath.Join(dir, MarkdownFileName))
	require.NoError(t, err)
	assert.Contains(t, string(md), "| obligation:refund-approval Refunds need approval (controls: validator:refund-guard) |")
	assert.Contains(t, string(md), "No objective maps to: obligation:ai-disclosure Disclose AI")
}
