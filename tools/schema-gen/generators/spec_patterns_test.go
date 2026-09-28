package generators

import (
	"regexp"
	"testing"

	"github.com/invopop/jsonschema"
)

// generatedProperty returns the property schema a specPattern targets in doc,
// or nil when doc does not define it.
func generatedProperty(doc map[string]interface{}, sp specPattern) map[string]interface{} {
	defs := defsOf(doc)
	for _, candidate := range []string{sp.def, packQualifier("packspec") + sp.def} {
		def, ok := defs[candidate].(map[string]interface{})
		if !ok {
			continue
		}
		props, _ := def["properties"].(map[string]interface{})
		prop, ok := props[sp.property].(map[string]interface{})
		if !ok {
			continue
		}
		if sp.items {
			prop, _ = prop["items"].(map[string]interface{})
		}
		return prop
	}
	return nil
}

// TestSpecPatternsCarriedIntoEverySchema: every table entry lands, with the
// spec's own pattern, in each schema that defines the property. Before this,
// every one of these was a bare string, so `promptarena validate` passed values
// that the compiled pack then failed.
//
// Mutation check: drop applySpecPatterns from Generate and this fails for
// every entry.
func TestSpecPatternsCarriedIntoEverySchema(t *testing.T) {
	spec, err := parseEmbeddedSpec()
	if err != nil {
		t.Fatalf("parse embedded spec: %v", err)
	}
	schemas := generatedSchemas(t)

	for _, sp := range specPatterns {
		want, err := resolvePointer(spec, sp.pointer)
		if err != nil {
			t.Fatalf("$defs/%s.%s -> %s: %v", sp.def, sp.property, sp.pointer, err)
		}
		seen := false
		for file, doc := range schemas {
			prop := generatedProperty(doc, sp)
			if prop == nil {
				continue
			}
			seen = true
			if got := prop["pattern"]; got != want {
				t.Errorf("%s: $defs/%s.%s pattern = %v, want %v", file, sp.def, sp.property, got, want)
			}
		}
		// An entry no schema reaches protects nothing.
		if !seen {
			t.Errorf("specPatterns names $defs/%s.%s but no generated schema defines it", sp.def, sp.property)
		}
	}
}

// TestSpecPatternsRejectWhatThePackRejects pins the values that used to pass
// validate and fail compile.
func TestSpecPatternsRejectWhatThePackRejects(t *testing.T) {
	spec, err := parseEmbeddedSpec()
	if err != nil {
		t.Fatalf("parse embedded spec: %v", err)
	}
	cases := []struct {
		def, property string
		good, bad     []string
	}{
		{"Spec", "task_type", []string{"support", "technical_support", "support-v2"},
			[]string{"CustomerSupport", "support.v2", "2fa"}},
		{"Spec", "version", []string{"1.0.0", "v1.2.3", "1.0.0-rc.1"}, []string{"1.0", "latest"}},
		{"VariableMetadata", "name", []string{"user_name", "_x", "userName"}, []string{"user-name", "1st"}},
		{"PackMetadata", "language", []string{"en"}, []string{"en-US", "EN"}},
	}
	for _, c := range cases {
		var sp *specPattern
		for i := range specPatterns {
			if specPatterns[i].def == c.def && specPatterns[i].property == c.property {
				sp = &specPatterns[i]
			}
		}
		if sp == nil {
			t.Fatalf("no specPatterns entry for $defs/%s.%s", c.def, c.property)
		}
		node, err := resolvePointer(spec, sp.pointer)
		if err != nil {
			t.Fatalf("%s: %v", sp.pointer, err)
		}
		re := regexp.MustCompile(node.(string))
		for _, v := range c.good {
			if !re.MatchString(v) {
				t.Errorf("$defs/%s.%s: %q should be accepted", c.def, c.property, v)
			}
		}
		for _, v := range c.bad {
			if re.MatchString(v) {
				t.Errorf("$defs/%s.%s: %q should be rejected", c.def, c.property, v)
			}
		}
	}
}

// TestSpecPatternErrorsFailGeneration: a table entry that has drifted from the
// spec, or names the wrong shape, fails loudly instead of dropping the check.
func TestSpecPatternErrorsFailGeneration(t *testing.T) {
	spec := map[string]interface{}{
		"$defs": map[string]interface{}{"X": map[string]interface{}{"pattern": 7}},
	}
	if _, err := specPatternValue(spec, specPattern{def: "X", property: "p", pointer: "/$defs/Missing/pattern"}); err == nil {
		t.Error("unresolvable pointer should fail")
	}
	if _, err := specPatternValue(spec, specPattern{def: "X", property: "p", pointer: "/$defs/X/pattern"}); err == nil {
		t.Error("non-string pattern should fail")
	}

	def := &jsonschema.Schema{Properties: jsonschema.NewProperties()}
	def.Properties.Set("scalar", &jsonschema.Schema{Type: "string"})
	if _, err := patternTarget(def, specPattern{def: "X", property: "scalar", items: true}); err == nil {
		t.Error("items entry on a non-array property should fail")
	}
	if got, err := patternTarget(def, specPattern{def: "X", property: "absent"}); err != nil || got != nil {
		t.Errorf("absent property should be skipped, got %v, %v", got, err)
	}
	if got, err := patternTarget(&jsonschema.Schema{}, specPattern{def: "X", property: "scalar"}); err != nil || got != nil {
		t.Errorf("definition without properties should be skipped, got %v, %v", got, err)
	}
}

// TestToolNamePatterns: the tool name a pack exposes is constrained wherever
// it is authored — a Tool manifest's spec.name (or metadata.name when spec.name
// is absent) and an arena config's tool_specs keys — and nowhere else. The SDK
// runtime-config's tools never become a pack, so their names stay free.
func TestToolNamePatterns(t *testing.T) {
	want := mustSpecPattern(toolNamePointer)
	schemas := generatedSchemas(t)

	tool := schemas["tool.json"]
	if got := generatedProperty(tool, specPattern{def: "ToolSpec", property: "name"}); got["pattern"] != want {
		t.Errorf("tool.json ToolSpec.name pattern = %v, want %v", got["pattern"], want)
	}
	cond, _ := tool["if"].(map[string]interface{})
	fallback, _ := tool["else"].(map[string]interface{})
	if cond == nil || fallback == nil {
		t.Fatal("tool.json should constrain metadata.name only when spec.name is absent (if/else)")
	}
	meta, _ := fallback["properties"].(map[string]interface{})["metadata"].(map[string]interface{})
	name, _ := meta["properties"].(map[string]interface{})["name"].(map[string]interface{})
	if name["pattern"] != want {
		t.Errorf("tool.json fallback metadata.name pattern = %v, want %v", name["pattern"], want)
	}

	cfg, _ := defsOf(schemas["arena.json"])["Config"].(map[string]interface{})
	toolSpecs, _ := cfg["properties"].(map[string]interface{})["tool_specs"].(map[string]interface{})
	keys, _ := toolSpecs["propertyNames"].(map[string]interface{})
	if keys["pattern"] != want {
		t.Errorf("arena.json tool_specs key pattern = %v, want %v", keys["pattern"], want)
	}

	for _, file := range []string{"arena.json", "runtime-config.json"} {
		if got := generatedProperty(schemas[file], specPattern{def: "ToolSpec", property: "name"}); got["pattern"] != nil {
			t.Errorf("%s ToolSpec.name should carry no pattern, got %v", file, got["pattern"])
		}
	}
}

func TestMustSpecPatternPanicsOnDrift(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an unresolvable pointer should panic")
		}
	}()
	mustSpecPattern("/$defs/NoSuchDef/pattern")
}
