package generators

import (
	"slices"
	"testing"

	"github.com/invopop/jsonschema"

	"github.com/AltairaLabs/PromptKit/runtime/v2/workflow"
)

func arenaSchemaDefs(t *testing.T) map[string]*jsonschema.Schema {
	t.Helper()
	schema, err := GenerateArenaSchema()
	if err != nil {
		t.Fatalf("GenerateArenaSchema() error = %v", err)
	}
	js, ok := schema.(*jsonschema.Schema)
	if !ok {
		t.Fatal("GenerateArenaSchema() did not return *jsonschema.Schema")
	}
	return js.Definitions
}

func defProperty(t *testing.T, defs map[string]*jsonschema.Schema, def, prop string) *jsonschema.Schema {
	t.Helper()
	d, ok := defs[def]
	if !ok {
		t.Fatalf("$defs/%s not found in arena schema", def)
	}
	p, ok := d.Properties.Get(prop)
	if !ok || p == nil {
		t.Fatalf("$defs/%s.%s not found", def, prop)
	}
	return p
}

func enumStrings(t *testing.T, enum []interface{}) []string {
	t.Helper()
	out := make([]string, 0, len(enum))
	for _, v := range enum {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("enum holds a non-string value %v", v)
		}
		out = append(out, s)
	}
	return out
}

// The config's workflow and compositions fields must reach the schema as the
// PromptPack types, not as free-form objects. An interface{} field reflects to
// a schema with no type, which accepts anything.
func TestArenaSchema_WorkflowAndCompositionsAreTyped(t *testing.T) {
	defs := arenaSchemaDefs(t)
	if ref := defProperty(t, defs, "Config", "workflow").Ref; ref != "#/$defs/WorkflowConfig" {
		t.Errorf("Config.workflow ref = %q, want #/$defs/WorkflowConfig", ref)
	}
	comps := defProperty(t, defs, "Config", "compositions")
	if comps.AdditionalProperties == nil || comps.AdditionalProperties.Ref != "#/$defs/Composition" {
		t.Errorf("Config.compositions should be a map of #/$defs/Composition, got %+v", comps.AdditionalProperties)
	}
}

// Mutation check: drop applyWorkflowEnums from the arena Customize hook and
// every case fails — the reflector emits these as bare strings.
func TestArenaSchema_WorkflowEnumsMatchRuntimeAndSpec(t *testing.T) {
	defs := arenaSchemaDefs(t)
	spec, err := parseEmbeddedSpec()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		def, prop string
		want      []string
		// specEnum is the spec's enum for the property, when the spec closes it.
		// The constants and the spec must agree, or one of them has drifted.
		specEnum string
	}{
		{"WorkflowState", "orchestration", []string{
			workflow.OrchestrationInternal, workflow.OrchestrationExternal,
			workflow.OrchestrationHybrid, workflow.OrchestrationComposition,
		}, "/$defs/WorkflowState/properties/orchestration/enum"},
		{"WorkflowState", "control", []string{workflow.ControlUser, workflow.ControlAgent},
			"/$defs/WorkflowState/properties/control/enum"},
		{"WorkflowState", "persistence", []string{workflow.PersistenceTransient, workflow.PersistencePersistent}, ""},
		{"ArtifactDef", "mode", []string{workflow.ArtifactModeReplace, workflow.ArtifactModeAppend},
			"/$defs/ArtifactDef/properties/mode/enum"},
	}
	for _, tc := range cases {
		t.Run(tc.def+"."+tc.prop, func(t *testing.T) {
			got := enumStrings(t, defProperty(t, defs, tc.def, tc.prop).Enum)
			if !slices.Equal(got, tc.want) {
				t.Errorf("enum = %v, want %v", got, tc.want)
			}
			if tc.specEnum == "" {
				return
			}
			node, err := resolvePointer(spec, tc.specEnum)
			if err != nil {
				t.Fatalf("resolving %s: %v", tc.specEnum, err)
			}
			specVals, _ := node.([]interface{})
			if specGot := enumStrings(t, specVals); !slices.Equal(specGot, tc.want) {
				t.Errorf("spec enum %v differs from promptkit's constants %v", specGot, tc.want)
			}
		})
	}
}

func TestArenaSchema_CompositionShapes(t *testing.T) {
	defs := arenaSchemaDefs(t)

	t.Run("step kind and reducer strategy are open enums", func(t *testing.T) {
		for _, dp := range [][2]string{{"Step", "kind"}, {"Reducer", "strategy"}} {
			if n := len(defProperty(t, defs, dp[0], dp[1]).AnyOf); n != 2 {
				t.Errorf("%s.%s should be an open enum (anyOf with 2 branches), got %d", dp[0], dp[1], n)
			}
		}
	})

	t.Run("predicate op is closed to the spec's operators", func(t *testing.T) {
		got := enumStrings(t, defProperty(t, defs, "Predicate", "op").Enum)
		if !slices.Contains(got, "equals") || !slices.Contains(got, "greater_than_or_equals") {
			t.Errorf("Predicate.op enum = %v, want the spec's compare operators", got)
		}
	})

	t.Run("step input accepts a reference string or an object", func(t *testing.T) {
		def := defs["StepInput"]
		if def == nil || len(def.OneOf) != 2 {
			t.Fatalf("StepInput should be a oneOf of string and object, got %+v", def)
		}
		if def.OneOf[0].Type != "string" || def.OneOf[1].Type != "object" {
			t.Errorf("StepInput oneOf types = %q, %q; want string, object", def.OneOf[0].Type, def.OneOf[1].Type)
		}
	})

	t.Run("termination requires max_steps or tool_called", func(t *testing.T) {
		def := defs["TerminationPredicate"]
		if def == nil || len(def.AnyOf) != 2 {
			t.Fatalf("TerminationPredicate should carry the spec's anyOf, got %+v", def)
		}
		if def.AdditionalProperties != jsonschema.FalseSchema {
			t.Error("TerminationPredicate must stay closed, so a misspelled key is rejected")
		}
	})
}

// Every property of every workflow and composition definition carries a
// description, from the spec or, where the spec has none, a fallback. Without
// applySpecDescriptions none of them do: packspec types have no doc comments
// the reflector can read.
func TestArenaSchema_WorkflowAndCompositionFieldsAreDescribed(t *testing.T) {
	defs := arenaSchemaDefs(t)
	for name := range specDescriptionSources {
		def, ok := defs[name]
		if !ok {
			t.Errorf("$defs/%s is listed in specDescriptionSources but absent from arena.json", name)
			continue
		}
		if def.Description == "" {
			t.Errorf("$defs/%s has no description", name)
		}
		if def.Properties == nil {
			continue
		}
		for pair := def.Properties.Oldest(); pair != nil; pair = pair.Next() {
			if pair.Value == nil || pair.Value.Description == "" {
				t.Errorf("$defs/%s.%s has no description", name, pair.Key)
			}
		}
	}
}

// The shared TrueSchema must never be written to: it stands for every
// unconstrained property in every schema.
func TestApplySpecDescriptionsLeavesTrueSchemaAlone(t *testing.T) {
	arenaSchemaDefs(t)
	if jsonschema.TrueSchema.Description != "" {
		t.Errorf("jsonschema.TrueSchema was mutated: description %q", jsonschema.TrueSchema.Description)
	}
}
