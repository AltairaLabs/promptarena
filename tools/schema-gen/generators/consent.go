package generators

import (
	"github.com/invopop/jsonschema"

	"github.com/AltairaLabs/promptarena/v2/arena/consent"
)

// applyConsentOverrideEnum constrains each consent_overrides value to the
// actions the consent hook understands. The field is a map[string]string, which
// the reflector emits with any string allowed, so a typo such as `denied`
// passed authoring and was treated as a grant.
func applyConsentOverrideEnum(schema *jsonschema.Schema) {
	enum := make([]any, len(consent.Actions))
	for i, a := range consent.Actions {
		enum[i] = a
	}
	for _, def := range definitionsNamed(schema, "TurnDefinition") {
		if def.Properties == nil {
			continue
		}
		if prop, ok := def.Properties.Get("consent_overrides"); ok && prop != nil {
			prop.AdditionalProperties = &jsonschema.Schema{Type: "string", Enum: enum}
		}
	}
}
