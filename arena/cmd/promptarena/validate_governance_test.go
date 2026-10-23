package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/configschema"
	"github.com/AltairaLabs/promptarena/v2/packc/compiler"
)

// useBinarySchemas turns schema validation on (this package's tests run with
// it off) against the schemas embedded in this binary.
func useBinarySchemas(t *testing.T) {
	t.Helper()
	configschema.Disabled.Store(false)
	t.Cleanup(func() { configschema.Disabled.Store(true) })
}

// governanceKit writes a kit whose pack_metadata.governance is gov, indented
// under governance:.
func governanceKit(t *testing.T, gov string) string {
	t.Helper()
	return writeSupersetKit(t, `  pack_metadata:
    domain: support
    governance:
`+gov)
}

// schemaErrors returns the schema-stage findings of a validate report.
func schemaErrors(r validateJSONReport) []validateFinding {
	var out []validateFinding
	for _, f := range r.Errors {
		if f.Stage == stageSchema {
			out = append(out, f)
		}
	}
	return out
}

// The RFC 0016 constraints the arena schema now reads from the spec. Each bad
// value used to pass the schema stage and fail only once the pack was compiled.
func TestValidate_GovernanceSchemaConstraints(t *testing.T) {
	useBinarySchemas(t)
	cases := []struct {
		name      string
		good, bad string
	}{
		{
			name: "independent_of.axes",
			good: "      independent_of:\n        axes: [model, accountable_owner]\n",
			bad:  "      independent_of:\n        axes: [model, region]\n",
		},
		{
			name: "independent_of.enforcement",
			good: "      independent_of:\n        axes: [provider]\n        enforcement: strict\n",
			bad:  "      independent_of:\n        axes: [provider]\n        enforcement: sometimes\n",
		},
		{
			name: "reviews[].cadence",
			good: "      reviews:\n        - {id: bias, type: pp:BiasTesting, cadence: P3M, owner: risk-team}\n",
			bad:  "      reviews:\n        - {id: bias, type: pp:BiasTesting, cadence: quarterly, owner: risk-team}\n",
		},
		{
			name: "obligations[].controls[] with two keys",
			good: "      obligations:\n        - id: disclose\n          obligation: eu-aiact:Article50\n" +
				"          controls:\n            - external: Disclosure banner in the client\n",
			bad: "      obligations:\n        - id: disclose\n          obligation: eu-aiact:Article50\n" +
				"          controls:\n            - {external: Banner, field: requires_ai_disclosure}\n",
		},
		{
			name: "obligations[].controls[] with no key",
			good: "      obligations:\n        - id: disclose\n          obligation: eu-aiact:Article50\n" +
				"          controls:\n            - external: Disclosure banner in the client\n",
			bad: "      obligations:\n        - id: disclose\n          obligation: eu-aiact:Article50\n" +
				"          controls:\n            - {}\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			good := validateJSONFor(t, governanceKit(t, c.good))
			assert.Empty(t, schemaErrors(good), "good value rejected: %+v", good.Errors)

			bad := validateJSONFor(t, governanceKit(t, c.bad))
			assert.False(t, bad.Valid)
			require.NotEmpty(t, schemaErrors(bad), "bad value passed the schema stage: %+v", bad.Errors)
		})
	}
}

// findingsIn returns the findings from stage.
func findingsIn(list []validateFinding, stage string) []validateFinding {
	var out []validateFinding
	for _, f := range list {
		if f.Stage == stage {
			out = append(out, f)
		}
	}
	return out
}

const disclosureObligation = `      obligations:
        - id: disclose
          obligation: eu-aiact:Article50
          controls:
            - eval: disclosure_check
`

// An obligation naming an eval the pack does not declare fails validate, with
// the message a host would give when it opened the pack; declaring the eval
// fixes it.
func TestValidate_GovernanceReferencesMustResolve(t *testing.T) {
	missing := validateJSONFor(t, governanceKit(t, disclosureObligation))
	assert.False(t, missing.Valid)
	errs := findingsIn(missing.Errors, compiler.StageGovernance)
	require.Len(t, errs, 1, "errors: %+v", missing.Errors)
	assert.Contains(t, errs[0].Description, `"disclosure_check"`)
	assert.Empty(t, findingsIn(missing.Errors, compiler.StagePack),
		"the governance finding is not repeated under the pack stage")

	fixed := validateJSONFor(t, governanceKit(t, disclosureObligation+`  pack_evals:
    - id: disclosure_check
      type: contains
      trigger: every_turn
      params:
        patterns: ["AI assistant"]
`))
	assert.True(t, fixed.Valid, "errors: %+v", fixed.Errors)
}

// RFC 0016 makes an undeclared vocabulary prefix a warning, never an error.
// pack.Validate reports it alongside the errors, so before the governance
// stage existed it failed validate and packc.
func TestValidate_GovernanceWarningsDoNotFail(t *testing.T) {
	report := validateJSONFor(t, governanceKit(t, `      obligations:
        - id: retain
          obligation: acme:RetainTranscripts
          controls:
            - external: Transcript retention in the contact-centre platform
`))
	assert.True(t, report.Valid, "errors: %+v", report.Errors)
	warns := findingsIn(report.Warnings, compiler.StageGovernance)
	require.NotEmpty(t, warns, "warnings: %+v", report.Warnings)
	assert.Contains(t, warns[0].Description, "acme")
}
