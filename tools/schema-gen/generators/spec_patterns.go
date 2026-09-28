package generators

import (
	"fmt"

	"github.com/invopop/jsonschema"
)

// The reflector emits every identifier as a bare {"type":"string"}: packspec
// types carry no jsonschema tags, and arena's own fields have no reason to
// restate the spec. But the PromptPack spec constrains many of those values
// with a pattern, and the compiled pack is validated against it. Without the
// pattern here, a value like `task_type: CustomerSupport` or a variable named
// `user-name` passes `promptarena validate` and fails only when the pack is
// compiled.
//
// This is the pattern counterpart of specOpenObjects: each pattern is read from
// the embedded spec, never restated, so the two cannot drift.

// specPattern names a string property in a generated definition and the
// JSON Pointer of the spec node whose pattern it must carry. items marks an
// array property whose elements carry the pattern.
type specPattern struct {
	def, property string
	items         bool
	pointer       string
}

// specPatterns lists the properties whose values reach a pattern-checked pack
// field. Keep it evidenced: an entry is only correct when the value really is
// copied into that pack field. Each pointer is re-read on every run, so one
// that stops resolving fails generation rather than silently dropping a check.
var specPatterns = []specPattern{
	// A prompt's task_type becomes its pack prompt id (PackCompiler.createPackPrompt).
	{def: "Spec", property: "task_type", pointer: "/$defs/Prompt/properties/id/pattern"},
	// spec.version is copied to the pack prompt's version unchanged.
	{def: "Spec", property: "version", pointer: "/$defs/Prompt/properties/version/pattern"},
	// Variable names are copied to the pack's Variable.name (compileVariables).
	{def: "VariableMetadata", property: "name", pointer: "/$defs/Variable/properties/name/pattern"},
	// pack_metadata, or a prompt's spec.metadata, becomes the pack's metadata.
	{def: "PackMetadata", property: "language", pointer: "/properties/metadata/properties/language/pattern"},
	// The remaining definitions are packspec types embedded in the pack as-is.
	{def: "MetricDef", property: "name", pointer: "/$defs/MetricDef/properties/name/pattern"},
	{def: "ContentPart", property: "type", pointer: "/$defs/ContentPart/properties/type/pattern"},
	{def: "MediaConfig", property: "supported_types", items: true,
		pointer: "/$defs/MediaConfig/properties/supported_types/items/pattern"},
}

// applySpecPatterns copies the spec's patterns onto the generated properties
// that feed them. A definition or property absent from the schema is skipped:
// not every schema reaches every type.
func applySpecPatterns(schema *jsonschema.Schema) error {
	if schema == nil || schema.Definitions == nil {
		return nil
	}

	spec, err := parseEmbeddedSpec()
	if err != nil {
		return err
	}

	for _, sp := range specPatterns {
		pattern, err := specPatternValue(spec, sp)
		if err != nil {
			return err
		}
		for _, def := range definitionsNamed(schema, sp.def) {
			prop, err := patternTarget(def, sp)
			if err != nil {
				return err
			}
			if prop != nil {
				prop.Pattern = pattern
			}
		}
	}
	return nil
}

// specPatternValue reads sp's pattern from the spec.
func specPatternValue(spec map[string]interface{}, sp specPattern) (string, error) {
	node, err := resolvePointer(spec, sp.pointer)
	if err != nil {
		return "", fmt.Errorf("resolving %s for $defs/%s.%s: %w", sp.pointer, sp.def, sp.property, err)
	}
	pattern, ok := node.(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("%s for $defs/%s.%s is not a pattern string", sp.pointer, sp.def, sp.property)
	}
	return pattern, nil
}

// patternTarget returns the schema in def that should carry sp's pattern, or
// nil when def has no such property.
func patternTarget(def *jsonschema.Schema, sp specPattern) (*jsonschema.Schema, error) {
	if def.Properties == nil {
		return nil, nil
	}
	prop, ok := def.Properties.Get(sp.property)
	if !ok || prop == nil {
		return nil, nil
	}
	if !sp.items {
		return prop, nil
	}
	if prop.Items == nil {
		return nil, fmt.Errorf("$defs/%s.%s is not an array", sp.def, sp.property)
	}
	return prop.Items, nil
}

// toolNamePointer is the spec's pattern for a pack tool's name.
const toolNamePointer = "/$defs/Tool/properties/name/pattern"

// mustSpecPattern reads a pattern from the embedded spec for a Customize hook,
// which cannot return an error. A pointer that stops resolving is a generator
// bug, pinned by TestToolNamePatterns; panicking fails generation the same way.
func mustSpecPattern(pointer string) string {
	spec, err := parseEmbeddedSpec()
	if err != nil {
		panic(err)
	}
	pattern, err := specPatternValue(spec, specPattern{def: "-", property: "-", pointer: pointer})
	if err != nil {
		panic(err)
	}
	return pattern
}

// applyToolManifestNamePattern constrains the name a Tool manifest exposes,
// which is spec.name, or metadata.name when spec.name is absent (the runtime's
// ToolConfig.FunctionName). metadata.name is only checked in that fallback
// case: `metadata.name: weather-tool` with `spec.name: get_weather` is valid.
func applyToolManifestNamePattern(schema *jsonschema.Schema) {
	pattern := mustSpecPattern(toolNamePointer)
	for _, def := range definitionsNamed(schema, "ToolSpec") {
		if prop, ok := def.Properties.Get("name"); ok && prop != nil {
			prop.Pattern = pattern
		}
	}

	metadata := jsonschema.NewProperties()
	metadata.Set("name", &jsonschema.Schema{Pattern: pattern})
	properties := jsonschema.NewProperties()
	properties.Set("metadata", &jsonschema.Schema{Properties: metadata})
	specProps := jsonschema.NewProperties()
	specProps.Set("spec", &jsonschema.Schema{Required: []string{"name"}})
	schema.If = &jsonschema.Schema{Properties: specProps}
	schema.Else = &jsonschema.Schema{Properties: properties}
}

// applyInlineToolNamePattern constrains the keys of an arena config's
// tool_specs: an inline tool's key becomes its name, overriding any spec.name.
func applyInlineToolNamePattern(schema *jsonschema.Schema) {
	def, ok := schema.Definitions["Config"]
	if !ok || def == nil || def.Properties == nil {
		return
	}
	if prop, ok := def.Properties.Get("tool_specs"); ok && prop != nil {
		prop.PropertyNames = &jsonschema.Schema{Pattern: mustSpecPattern(toolNamePointer)}
	}
}
