package generators

import (
	"strings"
	"testing"
)

func toolSpecOf(t *testing.T, doc map[string]interface{}, file string) map[string]interface{} {
	t.Helper()
	def, ok := defsOf(doc)[toolSpecDef].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: no $defs/%s", file, toolSpecDef)
	}
	return def
}

// arena registers no exec executor, so the mode description must not offer
// exec as an arena mode, and only the Tool manifest (not an inline tool_specs
// entry, decoded into config.ToolSpec) takes the file mocks the runtime reads.
func TestToolSpecSchema(t *testing.T) {
	schemas := generatedSchemas(t)
	for file, wantFiles := range map[string]bool{"tool.json": true, "arena.json": false} {
		def := toolSpecOf(t, schemas[file], file)
		props, _ := def["properties"].(map[string]interface{})

		mode, _ := props["mode"].(map[string]interface{})
		if desc, _ := mode["description"].(string); desc != toolModeDescription ||
			!strings.Contains(desc, "promptarena cannot execute it") || strings.Contains(desc, "'exec' (subprocess) -") {
			t.Errorf("%s: mode description = %q", file, desc)
		}

		for _, f := range []string{mockResultFileField, mockTemplateFileField} {
			if _, has := props[f]; has != wantFiles {
				t.Errorf("%s: has %s = %v, want %v", file, f, has, wantFiles)
			}
		}
	}
}

func TestConsentOverrideEnum(t *testing.T) {
	schemas := generatedSchemas(t)
	for _, file := range []string{"scenario.json", "arena.json"} {
		def, _ := defsOf(schemas[file])["TurnDefinition"].(map[string]interface{})
		props, _ := def["properties"].(map[string]interface{})
		co, _ := props["consent_overrides"].(map[string]interface{})
		ap, _ := co["additionalProperties"].(map[string]interface{})
		enum, _ := ap["enum"].([]interface{})
		if len(enum) != 3 || enum[0] != "grant" || enum[1] != "deny" || enum[2] != "timeout" {
			t.Errorf("%s: consent_overrides values enum = %v", file, enum)
		}
	}
}
