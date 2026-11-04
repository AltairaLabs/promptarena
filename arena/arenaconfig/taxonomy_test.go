package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTaxonomy(t *testing.T) {
	ok := []string{"owasp-llm:LLM01", "owasp-agentic:ASI10", "atlas:AML.T0051", "atlas:AML.T0051.001",
		"nist-ai-600-1:information-security", "owasp-llm:{owasp}"}
	assert.NoError(t, validateTaxonomy(ok, 1, true))

	cases := map[string]string{
		"LLM01":            "must be <framework>:<id>",
		"OWASP:LLM01":      "must be <framework>:<id>",
		"owasp-llm:LLM11":  "has no LLM11",
		"owasp-agentic:A1": "has no A1",
		"atlas:T0051":      "not a MITRE ATLAS technique id",
		"owasp-llm:{x}":    "has a placeholder but the scenario has no data",
	}
	for entry, want := range cases {
		err := validateTaxonomy([]string{entry}, 1, false)
		require.Error(t, err, entry)
		assert.Contains(t, err.Error(), "turns[1].objective.taxonomy[0]")
		assert.Contains(t, err.Error(), want, entry)
	}
}

func TestValidateTaxonomy_Duplicate(t *testing.T) {
	err := validateTaxonomy([]string{"owasp-llm:LLM01", "owasp-llm:LLM01"}, 0, false)
	assert.ErrorContains(t, err, "listed twice")
}

func TestCheckDataTaxonomy(t *testing.T) {
	obj := validObjective()
	obj.Taxonomy = []string{"owasp-llm:{owasp}"}
	s := objectiveScenario(TurnDefinition{Role: "attacker", Persona: "p", Objective: obj})
	good := []DataRow{{ID: "a", Values: map[string]string{"owasp": "LLM06"}}}
	assert.NoError(t, checkDataTaxonomy(s, good))
	bad := append(good, DataRow{ID: "b", Values: map[string]string{"owasp": "LLM6"}})
	assert.ErrorContains(t, checkDataTaxonomy(s, bad), "row b: turns[1].objective.taxonomy[0]")
}

func TestScenarioValidate_ObjectiveTaxonomy(t *testing.T) {
	obj := validObjective()
	obj.Taxonomy = []string{"owasp-llm:LLM99"}
	s := objectiveScenario(TurnDefinition{Role: "attacker", Persona: "p", Objective: obj})
	assert.ErrorContains(t, s.Validate(), "has no LLM99")
}

func TestForDataRow_SubstitutesTaxonomy(t *testing.T) {
	obj := validObjective()
	obj.Taxonomy = []string{"owasp-llm:{owasp}", "atlas:AML.T0051"}
	s := objectiveScenario(TurnDefinition{Role: "attacker", Persona: "p", Objective: obj})
	out := s.ForDataRow(DataRow{ID: "r1", Values: map[string]string{"owasp": "LLM06"}})
	assert.Equal(t, []string{"owasp-llm:LLM06", "atlas:AML.T0051"}, out.Turns[1].Objective.Taxonomy)
	assert.Equal(t, "owasp-llm:{owasp}", s.Turns[1].Objective.Taxonomy[0], "the original is untouched")
}

func TestTaxonomyFrameworkOf(t *testing.T) {
	f, id := TaxonomyFrameworkOf("owasp-llm:LLM07")
	require.NotNil(t, f)
	assert.Equal(t, "System Prompt Leakage", f.Titles[id])
	f, _ = TaxonomyFrameworkOf("atlas:AML.T0051")
	assert.Nil(t, f)
	assert.Equal(t, "Prompt Injection", TaxonomyTitle("owasp-llm:LLM01"))
	assert.Empty(t, TaxonomyTitle("atlas:AML.T0051"))
}
