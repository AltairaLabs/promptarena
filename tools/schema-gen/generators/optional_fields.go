package generators

import "github.com/invopop/jsonschema"

// optionalFieldDefinitions names PromptKit provider config types whose fields
// are each optional in a provider file, although their Go fields carry no
// omitempty. The reflector marks every field without omitempty as required, so
// without this a provider whose defaults set temperature and max_tokens but
// not top_p fails to load, which v2.2.0 (before the loaders checked the schema)
// accepted. A field left out decodes to zero, and the runtime treats zero as
// "not set": the provider falls back to its own default (issue #358).
//
// The real fix is omitempty on these fields in PromptKit's pkg/config; until
// then the generator drops the required list here.
var optionalFieldDefinitions = []string{"ProviderDefaults", "Pricing", "RateLimit"}

// applyOptionalFields clears the required list of each definition named in
// optionalFieldDefinitions. A definition absent from the schema is skipped.
func applyOptionalFields(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	for _, name := range optionalFieldDefinitions {
		if def, ok := schema.Definitions[name]; ok && def != nil {
			def.Required = nil
		}
	}
}
