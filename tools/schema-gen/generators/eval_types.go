package generators

import (
	"sort"

	"github.com/invopop/jsonschema"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/AltairaLabs/PromptKit/runtime/prompt"

	// Blank import populates the default eval handler registry (handlers
	// self-register via init), so knownEvalTypes reflects exactly the set this
	// PromptKit build supports.
	_ "github.com/AltairaLabs/PromptKit/runtime/evals/handlers"
)

// knownEvalTypes returns the sorted list of eval/assertion handler types this
// PromptKit build registers. Assertions, evals and guardrails all resolve
// against this one registry, so a single source feeds every type field.
func knownEvalTypes() []string {
	types := evals.NewEvalTypeRegistry().Types()
	sort.Strings(types)
	return types
}

// applyOpenTypeEnum rewrites the named property of def into an "open enum": the
// known types are offered as suggestions (an enum branch) while any string is
// still accepted (a plain string branch). This makes the PromptKit/Arena schema
// self-documenting and editor-friendly without being strict — a different
// runtime may define types this build has never heard of, and those must not be
// rejected. Runtime correctness is still enforced separately by
// `promptarena validate` (ValidateAssertionTypes / ValidateParams).
func applyOpenTypeEnum(def *jsonschema.Schema, prop string, knownTypes []string, description string) {
	if def == nil || def.Properties == nil {
		return
	}
	if _, ok := def.Properties.Get(prop); !ok {
		return
	}

	enumVals := make([]interface{}, len(knownTypes))
	for i, t := range knownTypes {
		enumVals[i] = t
	}

	def.Properties.Set(prop, &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{
			{Enum: enumVals},
			{Type: jsonTypeString},
		},
		Description: description,
	})
}

// typeEnumDescription is the shared description for every open handler-type enum.
const typeEnumDescription = "Handler type. PromptKit-known types are suggested, " +
	"but values are not restricted to this list — a different runtime may define additional types."

// evalDefNames are the $defs keys promptkit's eval definition can land under.
// It used to be the hand-written evals.EvalDef; PromptPack 1.6 made it an alias
// for packspec.Eval, so qualifyingNamer files it as "Eval" where nothing
// contests the name and "PackspecEval" in arena.json, where arena's own replay
// Eval type holds the plain one. The shape check below keeps the arena type
// from being decorated by mistake.
var evalDefNames = []string{"EvalDef", "Eval", "PackspecEval"}

// applyKnownTypeSuggestions decorates every definition in the schema that
// carries an eval-handler `type` field (AssertionConfig, the eval definition)
// with the open type enum. Assertions, evals and guardrails all resolve against
// the same registry, so a single suggestion set applies uniformly.
func applyKnownTypeSuggestions(schema *jsonschema.Schema) {
	if schema == nil || schema.Definitions == nil {
		return
	}
	known := knownEvalTypes()
	applyOpenTypeEnum(schema.Definitions["AssertionConfig"], "type", known, typeEnumDescription)
	for _, defName := range evalDefNames {
		def := schema.Definitions[defName]
		if def == nil || def.Properties == nil {
			continue
		}
		// `trigger` is what tells an eval definition apart from arena's replay
		// Eval config, which also has a `type`-less spec but no trigger.
		if _, isEvalDef := def.Properties.Get("trigger"); !isEvalDef {
			continue
		}
		applyOpenTypeEnum(def, "type", known, typeEnumDescription)
	}
}

// applyGovernanceEnums closes the enums RFC 0013 defines on a governance
// declaration.
//
// Unlike the handler-type enums above, these are genuinely closed: the
// PromptPack schema rejects a value outside them, so `autonomy_level: reviewed`
// fails at pack compile. Arena's schema could not say so on its own —
// packspec.Governance is a read-only generated type, so it carries no
// jsonschema:"enum=..." tags and the reflector emits a bare string. Typing the
// config's agents and pack_metadata blocks bought structure and completion but
// still let a bad level through arena validation, which is the error this is
// meant to catch at authoring time rather than three steps later.
//
// The values come from promptkit's own constants, so the two cannot drift.
func applyGovernanceEnums(schema *jsonschema.Schema) {
	if schema == nil || schema.Definitions == nil {
		return
	}
	def := schema.Definitions["Governance"]
	if def == nil || def.Properties == nil {
		return
	}
	prop, ok := def.Properties.Get("autonomy_level")
	if !ok {
		return
	}
	levels := []string{
		prompt.AutonomyLevelSuggests,
		prompt.AutonomyLevelActsWithApproval,
		prompt.AutonomyLevelActsWithOversight,
		prompt.AutonomyLevelActsAutonomously,
	}
	enum := make([]interface{}, len(levels))
	for i, l := range levels {
		enum[i] = l
	}
	prop.Enum = enum
}
