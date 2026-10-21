package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/agentkb"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

// useBinarySchemas turns schema validation on (this package's tests run with
// it off) against the schemas embedded in this binary, as main does, rather
// than PromptKit's own copy.
func useBinarySchemas(t *testing.T) {
	t.Helper()
	config.SchemaValidationDisabled.Store(false)
	agentkb.UseEmbeddedSchemas()
	t.Cleanup(func() {
		config.UseSchemaFS(nil)
		config.SchemaValidationDisabled.Store(true)
	})
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
