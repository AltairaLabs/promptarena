package generators

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/invopop/jsonschema"

	packschema "github.com/AltairaLabs/PromptKit/runtime/prompt/schema"
)

// The reflector runs with AllowAdditionalProperties:false, so every object it
// emits is closed. That is the right default for arena's own hand-written
// config types — an unknown key there is a typo, and catching it at authoring
// time is the whole point of the schema.
//
// It is the wrong default for the packspec types promptkit generates from the
// PromptPack spec. Those are read-only generated types carrying no jsonschema
// tags, so the reflector cannot see that the spec declares the corresponding
// object *open*, and it closes it anyway. The generated schema then rejects
// documents the pack format explicitly permits, and the error surfaces as
// "Additional property <x> is not allowed" at authoring time for a key the
// compiled artifact would have accepted.
//
// This is the same class of gap applyGovernanceEnums fixes from the other
// direction: there the spec closes an enum the reflector left open; here the
// spec opens an object the reflector closed. Both exist because packspec types
// carry no schema tags, and both are repaired here from the spec itself rather
// than restated by hand, so the two cannot drift.

// specOpenObjects maps a generated $defs name to the JSON Pointer of the object
// it mirrors in the PromptPack spec. An entry means: if the spec says this
// object is open, the generated definition must say so too.
//
// Keep this table small and evidenced. An entry is only correct when the
// generated definition really is the projection of that spec object; the
// pointer is re-read from the embedded spec on every run, so an entry that
// stops matching the spec fails loudly rather than silently over-opening.
var specOpenObjects = map[string]string{
	// pack.Metadata. The spec's `metadata` is additionalProperties:true with
	// five documented properties, so a pack may carry arbitrary metadata —
	// approval fields, change-control references, anything a firm needs inside
	// the content digest. Closing it made that unreachable from the authoring
	// path (promptarena#134) and additionally dropped `changelog` and
	// `performance`, which validated before PromptKit v1.8.0.
	"PackMetadata": "/properties/metadata",

	// packspec.MetricDef. Open in the spec for the same reason: a metric
	// definition carries handler-specific keys the spec does not enumerate.
	"MetricDef": "/$defs/MetricDef",
}

// applySpecOpenObjects opens the generated definitions that the PromptPack spec
// declares open. It reads the spec embedded in the promptkit build this binary
// links against, so the schemas cannot drift from the spec they project.
//
// A definition absent from the schema is skipped: not every packspec type is
// reachable from every arena config root, and the two schemas legitimately
// carry different subsets.
func applySpecOpenObjects(schema *jsonschema.Schema) error {
	if schema == nil || schema.Definitions == nil {
		return nil
	}

	spec, err := parseEmbeddedSpec()
	if err != nil {
		return err
	}

	for defName, pointer := range specOpenObjects {
		open, err := specObjectIsOpen(spec, pointer)
		if err != nil {
			return fmt.Errorf("resolving %s for $defs/%s: %w", pointer, defName, err)
		}
		if !open {
			// The spec closed it. Leave the reflector's closed object alone —
			// following the spec in both directions is the point.
			continue
		}
		for _, def := range definitionsNamed(schema, defName) {
			def.AdditionalProperties = jsonschema.TrueSchema
		}
	}
	return nil
}

// definitionsNamed returns the definitions matching name, accounting for
// qualifyingNamer prefixing a contested base name with its package (Eval ->
// PackspecEval). Both spellings are checked so this keeps working if a future
// arena type collides with one of these names.
func definitionsNamed(schema *jsonschema.Schema, name string) []*jsonschema.Schema {
	var out []*jsonschema.Schema
	for _, candidate := range []string{name, packQualifier("packspec") + name} {
		if def, ok := schema.Definitions[candidate]; ok && def != nil {
			out = append(out, def)
		}
	}
	return out
}

// parseEmbeddedSpec decodes the PromptPack schema embedded in promptkit.
func parseEmbeddedSpec() (map[string]interface{}, error) {
	raw := packschema.GetEmbeddedSchema()
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("promptkit's embedded PromptPack schema is empty")
	}
	var spec map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return nil, fmt.Errorf("parse embedded PromptPack schema: %w", err)
	}
	return spec, nil
}

// specObjectIsOpen resolves a JSON Pointer in the spec and reports whether the
// object it names sets additionalProperties:true. A pointer that does not
// resolve is an error: it means this table has drifted from the spec, which is
// exactly the condition worth failing generation for.
func specObjectIsOpen(spec map[string]interface{}, pointer string) (bool, error) {
	node, err := resolvePointer(spec, pointer)
	if err != nil {
		return false, err
	}
	obj, ok := node.(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("does not name an object")
	}
	open, _ := obj["additionalProperties"].(bool)
	return open, nil
}

// resolvePointer walks a JSON Pointer (RFC 6901) through decoded JSON. Only the
// object-traversal subset the table needs is supported.
func resolvePointer(root map[string]interface{}, pointer string) (interface{}, error) {
	if pointer == "" || !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("pointer %q must start with %q", pointer, "/")
	}
	var node interface{} = root
	for _, token := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		token = strings.ReplaceAll(token, "~1", "/")
		token = strings.ReplaceAll(token, "~0", "~")
		obj, ok := node.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("segment %q: parent is not an object", token)
		}
		next, ok := obj[token]
		if !ok {
			return nil, fmt.Errorf("segment %q not found", token)
		}
		node = next
	}
	return node, nil
}
