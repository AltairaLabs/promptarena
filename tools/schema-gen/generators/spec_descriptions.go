package generators

import (
	"fmt"

	"github.com/invopop/jsonschema"
)

// Arena's own types get their schema descriptions from Go doc comments (see
// goCommentDirs). The packspec types promptkit generates do not: the reflector
// only reads comments from source directories under the arena module, so a
// packspec definition reaches the schema as bare property names. That left an
// author completing `workflow:` or `compositions:` with field names and nothing
// to say what they do.
//
// The descriptions already exist, in the PromptPack spec those types were
// generated from. This copies them from the embedded spec rather than restating
// them, the same way specOpenObjects and specPatterns carry openness and
// patterns, so the two cannot drift.

// specDescriptionSources maps a generated $defs name to the spec nodes that
// document it. The first pointer documents the definition itself; every pointer
// is searched, in order, for a description of each property. A spec union (a
// Step's five kinds, a Predicate's five shapes) is flattened into one generated
// type, so its property descriptions live on the variants.
//
// Each pointer is re-read on every run; one that stops resolving fails
// generation rather than silently dropping descriptions.
var specDescriptionSources = map[string][]string{
	"WorkflowConfig":       {"/$defs/WorkflowConfig"},
	"WorkflowConfigEngine": {"/$defs/WorkflowConfig/properties/engine"},
	"WorkflowBudget":       {"/$defs/WorkflowBudget"},
	"WorkflowState":        {"/$defs/WorkflowState"},
	"ArtifactDef":          {"/$defs/ArtifactDef"},
	"Composition":          {"/$defs/Composition"},
	"Step": {
		"/$defs/Step", "/$defs/PromptStep", "/$defs/AgentStep",
		"/$defs/ToolStep", "/$defs/BranchStep", "/$defs/ParallelStep",
	},
	"StepInput":            {"/$defs/StepInput"},
	"StepModifiers":        {"/$defs/StepModifiers"},
	"StepModifiersRetry":   {"/$defs/StepModifiers/properties/retry"},
	"TerminationPredicate": {"/$defs/TerminationPredicate"},
	"Reducer":              {"/$defs/Reducer"},
	"Predicate": {
		"/$defs/Predicate", "/$defs/ComparePredicate", "/$defs/ExistsPredicate",
		"/$defs/AllOfPredicate", "/$defs/AnyOfPredicate", "/$defs/NotPredicate",
	},
}

// specDescriptionFallbacks documents properties the spec leaves undescribed;
// the "" key documents the definition itself. Only used when no spec node has a
// description, so a spec that later documents one of these takes over without a
// change here.
var specDescriptionFallbacks = map[string]map[string]string{
	"TerminationPredicate": {
		"max_steps": "Maximum number of LLM rounds the agent step's tool loop may run before it exits.",
	},
	"StepModifiersRetry": {
		"":             "Re-runs the step on error, up to max_attempts.",
		"max_attempts": "Maximum number of attempts, including the first, before the step's error is returned.",
	},
	"StepModifiers": {
		"retry": "Re-runs the step on error, up to max_attempts.",
	},
	"Step": {
		"description": "Human-readable description of what this step does.",
		"branches":    "Parallel steps only: the steps to run concurrently (at least two).",
		"reduce":      "Parallel steps only: how the branch outputs are merged.",
		"predicate":   "Branch steps only: the condition that selects then or else.",
	},
	"Predicate": {
		"op":     "Comparison operator for a compare predicate (path, op, value).",
		"exists": "For an exists predicate: true if path must resolve to a value, false if it must not.",
		"all_of": "Composite predicate: true when every listed predicate is true.",
		"any_of": "Composite predicate: true when at least one listed predicate is true.",
		"not":    "Composite predicate: true when the nested predicate is false.",
	},
}

// applySpecDescriptions fills in missing descriptions on the definitions in
// specDescriptionSources. A description already present is kept. A definition
// absent from the schema is skipped: not every schema reaches every type.
func applySpecDescriptions(schema *jsonschema.Schema) error {
	if schema == nil || schema.Definitions == nil {
		return nil
	}

	spec, err := parseEmbeddedSpec()
	if err != nil {
		return err
	}

	for defName, pointers := range specDescriptionSources {
		nodes := make([]map[string]interface{}, 0, len(pointers))
		for _, pointer := range pointers {
			node, err := resolvePointer(spec, pointer)
			if err != nil {
				return fmt.Errorf("resolving %s for $defs/%s: %w", pointer, defName, err)
			}
			obj, ok := node.(map[string]interface{})
			if !ok {
				return fmt.Errorf("%s for $defs/%s does not name an object", pointer, defName)
			}
			nodes = append(nodes, obj)
		}
		for _, def := range definitionsNamed(schema, defName) {
			describeDefinition(def, nodes, specDescriptionFallbacks[defName])
		}
	}
	return nil
}

// describeDefinition sets def's own description from the first spec node, and
// each property's from the first node that documents it, else the fallback.
func describeDefinition(def *jsonschema.Schema, nodes []map[string]interface{}, fallbacks map[string]string) {
	if def.Description == "" {
		def.Description, _ = nodes[0]["description"].(string)
	}
	if def.Description == "" {
		def.Description = fallbacks[""]
	}
	if def.Properties == nil {
		return
	}
	for pair := def.Properties.Oldest(); pair != nil; pair = pair.Next() {
		name, prop := pair.Key, pair.Value
		if prop != nil && prop.Description != "" {
			continue
		}
		desc := specPropertyDescription(nodes, name)
		if desc == "" {
			desc = fallbacks[name]
		}
		if desc == "" {
			continue
		}
		// An unconstrained property (Go `any`) is the shared TrueSchema value;
		// writing to it would describe every such property in every schema.
		// An empty schema with a description accepts the same values.
		if prop == nil || prop == jsonschema.TrueSchema {
			def.Properties.Set(name, &jsonschema.Schema{Description: desc})
			continue
		}
		prop.Description = desc
	}
}

// specPropertyDescription returns the first description of property name
// across the spec nodes, or "".
func specPropertyDescription(nodes []map[string]interface{}, name string) string {
	for _, node := range nodes {
		props, _ := node["properties"].(map[string]interface{})
		prop, _ := props[name].(map[string]interface{})
		if desc, _ := prop["description"].(string); desc != "" {
			return desc
		}
	}
	return ""
}
