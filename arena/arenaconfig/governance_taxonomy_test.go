package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

func TestMisuseSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Refunds outside policy":             "refunds-outside-policy",
		"Revealing another customer's order": "revealing-another-customers-order",
		"  Pricing, or setting terms!  ":     "pricing-or-setting-terms",
		"Step 2: approve":                    "step-2-approve",
		"???":                                "",
	} {
		assert.Equal(t, want, MisuseSlug(in), in)
	}
}

func governedConfig() *Config {
	return &Config{
		PackMetadata: &prompt.Metadata{Governance: &packspec.Governance{
			ForeseeableMisuse: []string{"Refunds outside policy", "Refunds outside policy!"},
			Obligations: []*packspec.Obligation{
				{ID: "refund-approval", Obligation: "Refunds need approval", Controls: []*packspec.ObligationControl{
					{Validator: "refund-guard"}, {Eval: "refund-judge"}, nil,
				}},
				nil,
				{ID: "", Obligation: "no id"},
			},
		}},
		Agents: &prompt.AgentsConfig{Members: map[string]*prompt.AgentDef{
			"billing": {Governance: &packspec.Governance{
				ForeseeableMisuse: []string{"Sharing card numbers"},
				Obligations: []*packspec.Obligation{
					{ID: "refund-approval", Obligation: "agent's restatement"},
					{ID: "ai-disclosure", Obligation: "Disclose AI", Controls: []*packspec.ObligationControl{
						{Field: "requires_ai_disclosure"}, {External: "Script review"},
					}},
				},
			}},
			"empty": nil,
		}},
	}
}

func TestGovernanceFrameworks(t *testing.T) {
	fs := governedConfig().GovernanceFrameworks()
	require.Len(t, fs, 2)

	obl := fs[0]
	assert.Equal(t, TaxonomyObligation, obl.Name)
	assert.Equal(t, []string{"refund-approval", "ai-disclosure"}, obl.IDs)
	assert.Equal(t, "Refunds need approval (controls: validator:refund-guard, eval:refund-judge)",
		obl.Titles["refund-approval"], "the pack's declaration comes first and wins")
	assert.Equal(t, "Disclose AI (controls: field:requires_ai_disclosure, external:Script review)",
		obl.Titles["ai-disclosure"])

	mis := fs[1]
	assert.Equal(t, TaxonomyMisuse, mis.Name)
	assert.Equal(t, []string{"refunds-outside-policy", "sharing-card-numbers"}, mis.IDs)
	assert.Equal(t, "Refunds outside policy", mis.Titles["refunds-outside-policy"])
}

func TestGovernanceFrameworks_NoneDeclared(t *testing.T) {
	assert.Empty(t, (&Config{}).GovernanceFrameworks())
	assert.Empty(t, (&Config{PackMetadata: &prompt.Metadata{}}).GovernanceFrameworks())
}

func scenarioWithTaxonomy(taxonomy ...string) *Scenario {
	return &Scenario{ID: "s", Turns: []TurnDefinition{
		{Role: "user", Content: "hi"},
		{Role: "attacker", Persona: "p", Objective: &ObjectiveConfig{Taxonomy: taxonomy}},
	}}
}

func TestValidateGovernanceTaxonomy(t *testing.T) {
	c := governedConfig()
	c.LoadedScenarios = map[string]*Scenario{
		"ok": scenarioWithTaxonomy("obligation:ai-disclosure", "misuse:sharing-card-numbers", "owasp-llm:LLM01"),
	}
	require.NoError(t, c.validateGovernanceTaxonomy())

	c.LoadedScenarios["bad"] = scenarioWithTaxonomy("obligation:nope")
	err := c.validateGovernanceTaxonomy()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scenario bad: turns[1].objective.taxonomy[0]")
	assert.Contains(t, err.Error(), "declared: refund-approval, ai-disclosure")
}

func TestValidateGovernanceTaxonomy_NoGovernance(t *testing.T) {
	c := &Config{LoadedScenarios: map[string]*Scenario{"s": scenarioWithTaxonomy("misuse:anything")}}
	err := c.validateGovernanceTaxonomy()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declare no foreseeable_misuse")

	c.LoadedScenarios["s"] = scenarioWithTaxonomy("obligation:x")
	assert.ErrorContains(t, c.validateGovernanceTaxonomy(), "declare no obligations")
}

func TestValidateGovernanceTaxonomy_DataRows(t *testing.T) {
	c := governedConfig()
	s := scenarioWithTaxonomy("misuse:{misuse}")
	s.Data = &ScenarioData{File: "rows.jsonl", Rows: []DataRow{
		{ID: "r1", Values: map[string]string{"misuse": "refunds-outside-policy"}},
		{ID: "r2", Values: map[string]string{"misuse": "refunds-outside-polcy"}},
	}}
	c.LoadedScenarios = map[string]*Scenario{"s": s}
	err := c.validateGovernanceTaxonomy()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"refunds-outside-polcy"`)
}
