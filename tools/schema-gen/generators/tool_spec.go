package generators

import (
	"github.com/invopop/jsonschema"
)

// toolSpecDef is the generated definition of PromptKit's config.ToolSpec, the
// spec of a Tool manifest and of an arena config's inline tool_specs.
const toolSpecDef = "ToolSpec"

// toolModeDescription replaces the description config.ToolSpec carries on
// `mode`. PromptKit's lists 'exec', which the SDK runs but promptarena does not:
// arena registers no exec executor, so a run stops at startup on a tool with
// mode: exec. The same holds for 'local' (an in-process Go handler), which the
// arena loader rejects.
const toolModeDescription = "Execution mode. One of: 'mock' (use mock_result or mock_template) - " +
	"'live' (HTTP via 'http') - 'mcp' (MCP server) - 'client' (client-side handler; arena simulates it " +
	"with the mock fields and the turn's consent_overrides). 'exec' (subprocess) and 'local' (in-process " +
	"handler) are run by the PromptKit SDK only: promptarena cannot execute them, and a run stops at " +
	"startup on such a tool."

// File-backed mock fields. Setting a static result and a template together is
// rejected by the arena loader (validateToolMocks) rather than here: a schema
// `not` can only report "must not validate the schema", which names neither
// field.
const (
	mockResultFileField   = "mock_result_file"
	mockTemplateFileField = "mock_template_file"
)

// applyToolSpecMode corrects the `mode` description on every ToolSpec
// definition.
func applyToolSpecMode(schema *jsonschema.Schema) {
	for _, def := range definitionsNamed(schema, toolSpecDef) {
		if def.Properties == nil {
			continue
		}
		if prop, ok := def.Properties.Get("mode"); ok && prop != nil {
			prop.Description = toolModeDescription
		}
	}
}

// applyToolFileMocks adds the file-backed mock fields a Tool manifest accepts.
// PromptKit's runtime ToolDescriptor reads mock_result_file and
// mock_template_file, but config.ToolSpec, which the schema reflects, does not
// declare them, so the closed schema rejected manifests the runtime runs. They
// are added to the Tool manifest schema only: an inline tool_specs entry is
// decoded into config.ToolSpec, which would drop them.
func applyToolFileMocks(schema *jsonschema.Schema) {
	for _, def := range definitionsNamed(schema, toolSpecDef) {
		if def.Properties == nil {
			continue
		}
		def.Properties.Set(mockResultFileField, &jsonschema.Schema{
			Type: "string",
			Description: "Path to a JSON file holding the static mock result, relative to the arena config " +
				"directory. Mutually exclusive with mock_template and mock_template_file.",
		})
		def.Properties.Set(mockTemplateFileField, &jsonschema.Schema{
			Type: "string",
			Description: "Path to a file holding the mock_template, relative to the arena config directory. " +
				"Mutually exclusive with mock_result and mock_result_file.",
		})
	}
}
