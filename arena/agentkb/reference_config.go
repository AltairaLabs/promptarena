package agentkb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type jsonSchemaNode struct {
	Type        string                    `json:"type"`
	Description string                    `json:"description"`
	Required    []string                  `json:"required"`
	Properties  map[string]jsonSchemaNode `json:"properties"`
	Ref         string                    `json:"$ref"`
	Defs        map[string]jsonSchemaNode `json:"$defs"`
}

// UnmarshalJSON tolerates JSON Schema's boolean form (e.g. `input_schema: true`),
// which appears as a property/schema value in some config schemas. A boolean
// schema carries no fields, so it decodes to an empty node.
func (n *jsonSchemaNode) UnmarshalJSON(data []byte) error {
	if s := string(bytes.TrimSpace(data)); s == "true" || s == "false" {
		*n = jsonSchemaNode{}
		return nil
	}
	type alias jsonSchemaNode
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*n = jsonSchemaNode(a)
	return nil
}

// GenerateConfigFieldsReference renders a compact per-kind field cheat-sheet
// from the embedded JSON schemas. It is a navigation aid; `promptarena schema
// <type>` remains the authoritative full schema.
func GenerateConfigFieldsReference() ([]byte, error) {
	names, err := SchemaNames()
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("# Config Fields\n\n")
	b.WriteString("Compact cheat-sheet generated from the embedded schemas. ")
	b.WriteString("Run `promptarena schema <type>` for the full schema; ")
	b.WriteString("`promptarena validate` enforces the binary-embedded copy.\n")

	for _, name := range names {
		raw, err := Schema(name)
		if err != nil {
			return nil, err
		}
		var root jsonSchemaNode
		if err := json.Unmarshal(raw, &root); err != nil {
			return nil, fmt.Errorf("parse schema %q: %w", name, err)
		}
		fmt.Fprintf(&b, "\n## %s\n\n", name)
		// The config envelope wraps the real fields under `spec`, usually as a
		// $ref into $defs. Resolve it and document the spec object's fields.
		node := root
		if spec, ok := root.Properties["spec"]; ok {
			resolved := resolveRef(&spec, root.Defs)
			if len(resolved.Properties) > 0 {
				node = resolved
				b.WriteString("Fields under `spec`:\n\n")
			}
		}
		writeFieldTable(&b, &node, root.Defs)
		if err := writeNestedSections(&b, name, root.Defs); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

// nestedSection documents a block below `spec` whose fields the top-level
// table can only call "object".
type nestedSection struct {
	path string // the YAML path an author writes, for the heading
	def  string // the $defs entry holding its fields
}

// nestedSections lists, per schema, the nested blocks worth their own table.
// The workflow and composition blocks are deep and typo-prone, and their fields
// are what an agent brief needs to describe; one level of `object` says nothing.
var nestedSections = map[string][]nestedSection{
	"arena": {
		{"spec.workflow", "WorkflowConfig"},
		{"spec.workflow.engine.budget", "WorkflowBudget"},
		{"spec.workflow.states.<state>", "WorkflowState"},
		{"spec.workflow.states.<state>.artifacts.<name>", "ArtifactDef"},
		{"spec.compositions.<name>", "Composition"},
		{"spec.compositions.<name>.steps[]", "Step"},
		{"steps[].termination", "TerminationPredicate"},
		{"steps[].predicate", "Predicate"},
		{"steps[].reduce", "Reducer"},
		{"steps[].modifiers", "StepModifiers"},
	},
}

// writeNestedSections writes a field table for each nested block of schema. A
// listed definition missing from the schema is an error: the section would
// otherwise vanish from the reference without anyone noticing.
func writeNestedSections(b *bytes.Buffer, schema string, defs map[string]jsonSchemaNode) error {
	for _, sec := range nestedSections[schema] {
		def, ok := defs[sec.def]
		if !ok {
			return fmt.Errorf("schema %q: $defs/%s (for %s) not found", schema, sec.def, sec.path)
		}
		fmt.Fprintf(b, "\n### `%s`\n\n", sec.path)
		writeFieldTable(b, &def, defs)
	}
	return nil
}

// resolveRef follows a single "#/$defs/Name" reference into defs. Non-ref nodes
// (or unresolvable refs) are returned unchanged.
func resolveRef(node *jsonSchemaNode, defs map[string]jsonSchemaNode) jsonSchemaNode {
	const prefix = "#/$defs/"
	if node.Ref == "" || !strings.HasPrefix(node.Ref, prefix) {
		return *node
	}
	if target, ok := defs[strings.TrimPrefix(node.Ref, prefix)]; ok {
		return target
	}
	return *node
}

func writeFieldTable(b *bytes.Buffer, node *jsonSchemaNode, defs map[string]jsonSchemaNode) {
	required := map[string]bool{}
	for _, r := range node.Required {
		required[r] = true
	}
	keys := make([]string, 0, len(node.Properties))
	for k := range node.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		b.WriteString("_(no direct fields; see full schema)_\n")
		return
	}
	b.WriteString("| field | type | required | description |\n|-------|------|----------|-------------|\n")
	for _, k := range keys {
		p := node.Properties[k]
		req := ""
		if required[k] {
			req = "✓"
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s |\n", k, fieldType(&p, defs), req, oneLine(dash(p.Description)))
	}
}

func fieldType(p *jsonSchemaNode, defs map[string]jsonSchemaNode) string {
	if p.Type != "" {
		return p.Type
	}
	if p.Ref != "" {
		resolved := resolveRef(p, defs)
		if resolved.Type != "" {
			return resolved.Type
		}
		return "object"
	}
	return "—"
}

func oneLine(s string) string {
	var out bytes.Buffer
	for _, r := range s {
		if r == '\n' || r == '|' {
			out.WriteByte(' ')
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
