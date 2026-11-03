package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func factsPersona(facts ...PersonaFact) *UserPersonaPack {
	return &UserPersonaPack{ID: "p", SystemPrompt: "x", Facts: facts}
}

func TestValidatePersonaFacts(t *testing.T) {
	_, err := validateAndReturnPersona(factsPersona(PersonaFact{ID: "order", Value: "A-100"}), "f.yaml")
	assert.NoError(t, err)

	tests := map[string]struct {
		facts []PersonaFact
		want  string
	}{
		"missing id":   {[]PersonaFact{{Value: "A-100"}}, "facts[0].id is required"},
		"empty value":  {[]PersonaFact{{ID: "order", Value: "  "}}, "facts[0] (order) needs a value"},
		"duplicate id": {[]PersonaFact{{ID: "order", Value: "A"}, {ID: "order", Value: "B"}}, `duplicate id "order"`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := validateAndReturnPersona(factsPersona(tt.facts...), "f.yaml")
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestPersonaFactPolicy(t *testing.T) {
	p := factsPersona(PersonaFact{ID: "order", Value: "A-100"})
	assert.True(t, p.ChecksFacts(), "checked by default when facts are declared")
	assert.False(t, p.FailsOnFactViolation())

	off := false
	p.FactPolicy = &PersonaFactPolicy{Check: &off, FailOnViolation: true}
	assert.False(t, p.ChecksFacts())
	assert.True(t, p.FailsOnFactViolation())

	assert.False(t, factsPersona().ChecksFacts(), "nothing to check without facts")
}

func TestPersonaFacts_OptionalVarKeptWithoutFacts(t *testing.T) {
	p := factsPersona()
	p.SystemPrompt = ""
	p.SystemTemplate = "Background: {{facts}}"
	p.OptionalVars = map[string]string{"facts": "lives in Leeds"}

	prompt, err := p.BuildSystemPrompt("us", nil)
	require.NoError(t, err)
	assert.Equal(t, "Background: lives in Leeds", prompt)
}

func TestPersonaFacts_TemplateVariable(t *testing.T) {
	p := factsPersona(PersonaFact{ID: "order_number", Value: "A-100"}, PersonaFact{ID: "order_date", Value: "28 October"})
	p.SystemPrompt = ""
	p.SystemTemplate = "You know:\n{{facts}}"

	prompt, err := p.BuildSystemPrompt("us", nil)
	require.NoError(t, err)
	assert.Equal(t, "You know:\n- order_number: A-100\n- order_date: 28 October", prompt)
}

func TestPersonaFactPolicy_OnViolation(t *testing.T) {
	facts := []PersonaFact{{ID: "o", Value: "A-100"}}
	p := &UserPersonaPack{ID: "p", Facts: facts, FactPolicy: &PersonaFactPolicy{OnViolation: FactViolationRegenerate}}
	assert.NoError(t, validatePersonaFacts(p))
	assert.True(t, p.RegeneratesOnFactViolation())

	off := false
	p.FactPolicy.Check = &off
	assert.False(t, p.RegeneratesOnFactViolation(), "regenerate needs the check")

	for _, v := range []string{"", FactViolationFlag} {
		assert.False(t, (&UserPersonaPack{Facts: facts, FactPolicy: &PersonaFactPolicy{OnViolation: v}}).RegeneratesOnFactViolation())
	}
	err := validatePersonaFacts(&UserPersonaPack{ID: "p", Facts: facts, FactPolicy: &PersonaFactPolicy{OnViolation: "retry"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `fact_policy.on_violation must be flag or regenerate, got "retry"`)

	err = validatePersonaFacts(&UserPersonaPack{ID: "p", Facts: facts,
		FactPolicy: &PersonaFactPolicy{Check: &off, OnViolation: FactViolationRegenerate}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "regenerate needs facts and the check on")
	err = validatePersonaFacts(&UserPersonaPack{ID: "p", FactPolicy: &PersonaFactPolicy{OnViolation: FactViolationRegenerate}})
	require.Error(t, err)
}
