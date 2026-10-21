package generators

import (
	"reflect"
	"testing"

	"github.com/invopop/jsonschema"
)

// TestSpecEnumsCarriedIntoEverySchema: every specEnums entry lands, with the
// spec's own values, in each schema that defines the property.
//
// Mutation check: drop applySpecEnums from Generate and this fails for every
// entry.
func TestSpecEnumsCarriedIntoEverySchema(t *testing.T) {
	spec, err := parseEmbeddedSpec()
	if err != nil {
		t.Fatalf("parse embedded spec: %v", err)
	}
	schemas := generatedSchemas(t)

	for _, se := range specEnums {
		want, err := resolvePointer(spec, se.pointer)
		if err != nil {
			t.Fatalf("$defs/%s.%s -> %s: %v", se.def, se.property, se.pointer, err)
		}
		seen := false
		for file, doc := range schemas {
			prop := generatedProperty(doc, specPattern{def: se.def, property: se.property, items: se.items})
			if prop == nil {
				continue
			}
			seen = true
			if got := prop["enum"]; !reflect.DeepEqual(got, want) {
				t.Errorf("%s: $defs/%s.%s enum = %v, want %v", file, se.def, se.property, got, want)
			}
		}
		if !seen {
			t.Errorf("specEnums names $defs/%s.%s but no generated schema defines it", se.def, se.property)
		}
	}
}

// TestSpecPropertyCountsCarriedIntoEverySchema: a control carries exactly one
// key wherever the generated schemas define it.
func TestSpecPropertyCountsCarriedIntoEverySchema(t *testing.T) {
	schemas := generatedSchemas(t)
	for _, pc := range specPropertyCounts {
		seen := false
		for file, doc := range schemas {
			def, ok := defsOf(doc)[pc.def].(map[string]interface{})
			if !ok {
				continue
			}
			seen = true
			if def["minProperties"] != float64(1) || def["maxProperties"] != float64(1) {
				t.Errorf("%s: $defs/%s minProperties/maxProperties = %v/%v, want 1/1",
					file, pc.def, def["minProperties"], def["maxProperties"])
			}
		}
		if !seen {
			t.Errorf("specPropertyCounts names $defs/%s but no generated schema defines it", pc.def)
		}
	}
}

// TestSpecConstraintPointersThatDoNotResolveFailGeneration: a spec rename must
// surface on the dependency bump, not silently drop the check.
func TestSpecConstraintPointersThatDoNotResolveFailGeneration(t *testing.T) {
	schema := func() *jsonschema.Schema {
		def := &jsonschema.Schema{Type: "object", Properties: jsonschema.NewProperties()}
		def.Properties.Set("p", &jsonschema.Schema{Type: "string"})
		return &jsonschema.Schema{Definitions: jsonschema.Definitions{"X": def}}
	}

	saved := specEnums
	t.Cleanup(func() { specEnums = saved })
	specEnums = []specEnum{{def: "X", property: "p", pointer: "/$defs/Governance/properties/independent_of/properties/on/items/enum"}}
	if err := applySpecEnums(schema()); err == nil {
		t.Error("an enum pointer that does not resolve should fail generation")
	}
	specEnums = []specEnum{{def: "X", property: "p", pointer: "/$defs/Governance/properties/intended_purpose"}}
	if err := applySpecEnums(schema()); err == nil {
		t.Error("a pointer that names something other than an enum should fail generation")
	}

	savedCounts := specPropertyCounts
	t.Cleanup(func() { specPropertyCounts = savedCounts })
	specPropertyCounts = []specPropertyCount{{def: "X", pointer: "/$defs/MissingControl"}}
	if err := applySpecPropertyCounts(schema()); err == nil {
		t.Error("a property-count pointer that does not resolve should fail generation")
	}
	specPropertyCounts = []specPropertyCount{{def: "X", pointer: "/$defs/Review"}}
	if err := applySpecPropertyCounts(schema()); err == nil {
		t.Error("an object the spec does not bound by property count should fail generation")
	}
}

// TestSpecEnumOnUntypedItemsDoesNotTouchTrueSchema: the reflector emits an
// untyped array element as the shared jsonschema.TrueSchema; setting an enum
// must replace it rather than write through it.
func TestSpecEnumOnUntypedItemsDoesNotTouchTrueSchema(t *testing.T) {
	def := &jsonschema.Schema{Properties: jsonschema.NewProperties()}
	def.Properties.Set("axes", &jsonschema.Schema{Type: "array", Items: jsonschema.TrueSchema})
	se := specEnum{def: "X", property: "axes", items: true}
	if err := setSpecEnum(def, se, []interface{}{"model"}); err != nil {
		t.Fatal(err)
	}
	if jsonschema.TrueSchema.Enum != nil {
		t.Fatal("setSpecEnum wrote through the shared TrueSchema")
	}
	prop, _ := def.Properties.Get("axes")
	if !reflect.DeepEqual(prop.Items.Enum, []interface{}{"model"}) {
		t.Errorf("items enum = %v", prop.Items.Enum)
	}

	def.Properties.Set("scalar", &jsonschema.Schema{Type: "string"})
	if err := setSpecEnum(def, specEnum{def: "X", property: "scalar", items: true}, nil); err == nil {
		t.Error("an items entry on a non-array property should fail")
	}
}
