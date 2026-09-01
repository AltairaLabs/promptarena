package generators

import (
	"encoding/json"
	"testing"

	"github.com/invopop/jsonschema"
)

// generatedSchemas is every schema this generator emits, so the invariants
// below hold for all of them rather than the two that happen to be affected
// today.
func generatedSchemas(t *testing.T) map[string]map[string]interface{} {
	t.Helper()
	gens := map[string]func() (interface{}, error){
		"arena.json":          GenerateArenaSchema,
		"promptconfig.json":   GeneratePromptConfigSchema,
		"eval.json":           GenerateEvalSchema,
		"scenario.json":       GenerateScenarioSchema,
		"persona.json":        GeneratePersonaSchema,
		"provider.json":       GenerateProviderSchema,
		"tool.json":           GenerateToolSchema,
		"logging.json":        GenerateLoggingSchema,
		"runtime-config.json": GenerateRuntimeConfigSchema,
	}
	out := map[string]map[string]interface{}{}
	for name, gen := range gens {
		s, err := gen()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		var doc map[string]interface{}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("unmarshal %s: %v", name, err)
		}
		out[name] = doc
	}
	return out
}

func defsOf(doc map[string]interface{}) map[string]interface{} {
	defs, _ := doc["$defs"].(map[string]interface{})
	return defs
}

// TestSpecOpenObjectsAreOpenInEverySchema is the regression guard for
// promptarena#134. Before the fix, PackMetadata was emitted with
// additionalProperties:false in both arena.json and promptconfig.json, so
// `spec.metadata.approved_by` — which the pack format explicitly permits —
// failed schema validation at authoring time.
//
// Mutation check: revert applySpecOpenObjects (or drop either table entry) and
// this fails for that definition in both schemas.
func TestSpecOpenObjectsAreOpenInEverySchema(t *testing.T) {
	schemas := generatedSchemas(t)
	sawAny := map[string]bool{}

	for file, doc := range schemas {
		defs := defsOf(doc)
		for defName := range specOpenObjects {
			for _, candidate := range []string{defName, packQualifier("packspec") + defName} {
				def, ok := defs[candidate].(map[string]interface{})
				if !ok {
					continue
				}
				sawAny[defName] = true
				if addl, present := def["additionalProperties"]; present && addl != true {
					t.Errorf("%s: $defs/%s has additionalProperties=%v, want true — "+
						"the PromptPack spec declares this object open", file, candidate, addl)
				}
			}
		}
	}

	// A table entry naming a definition no schema emits is dead weight, and
	// would silently stop protecting anything.
	for defName := range specOpenObjects {
		if !sawAny[defName] {
			t.Errorf("specOpenObjects names %q but no generated schema defines it", defName)
		}
	}
}

// TestSpecOpenObjectsPointersResolve keeps the table honest against the spec it
// claims to mirror. If promptkit's embedded PromptPack schema moves or renames
// one of these objects, generation must fail loudly rather than quietly leaving
// the definition closed.
func TestSpecOpenObjectsPointersResolve(t *testing.T) {
	spec, err := parseEmbeddedSpec()
	if err != nil {
		t.Fatalf("parse embedded spec: %v", err)
	}
	for defName, pointer := range specOpenObjects {
		open, err := specObjectIsOpen(spec, pointer)
		if err != nil {
			t.Errorf("$defs/%s -> %s: %v", defName, pointer, err)
			continue
		}
		if !open {
			t.Errorf("$defs/%s -> %s: spec no longer declares this object open; "+
				"remove the entry or follow the spec", defName, pointer)
		}
	}
}

// TestApplySpecOpenObjectsFollowsSpecClosed is the other direction: the point is
// to mirror the spec, not to open things unconditionally. A definition whose
// spec counterpart is closed must be left as the reflector emitted it.
func TestApplySpecOpenObjectsFollowsSpecClosed(t *testing.T) {
	schema := &jsonschema.Schema{
		Definitions: jsonschema.Definitions{
			"PackMetadata": {AdditionalProperties: jsonschema.FalseSchema},
		},
	}
	spec := map[string]interface{}{
		"properties": map[string]interface{}{
			"metadata": map[string]interface{}{"additionalProperties": false},
		},
		"$defs": map[string]interface{}{
			"MetricDef": map[string]interface{}{"additionalProperties": false},
		},
	}
	for defName, pointer := range specOpenObjects {
		open, err := specObjectIsOpen(spec, pointer)
		if err != nil {
			t.Fatalf("%s: %v", defName, err)
		}
		if open {
			t.Fatalf("%s: fixture should model a closed spec object", defName)
		}
	}
	if got := schema.Definitions["PackMetadata"].AdditionalProperties; got != jsonschema.FalseSchema {
		t.Errorf("PackMetadata should still be closed, got %v", got)
	}
}

// TestResolvePointerRejectsBadPointers covers the failure modes that make
// generation fail loudly instead of silently skipping a table entry.
func TestResolvePointerRejectsBadPointers(t *testing.T) {
	root := map[string]interface{}{
		"properties": map[string]interface{}{"metadata": map[string]interface{}{}},
		"scalar":     "not-an-object",
	}
	for name, pointer := range map[string]string{
		"missing leading slash": "properties/metadata",
		"empty":                 "",
		"unknown segment":       "/properties/nope",
		"through a scalar":      "/scalar/deeper",
	} {
		if _, err := resolvePointer(root, pointer); err == nil {
			t.Errorf("%s (%q): expected an error", name, pointer)
		}
	}
	if _, err := resolvePointer(root, "/properties/metadata"); err != nil {
		t.Errorf("valid pointer failed: %v", err)
	}
}
