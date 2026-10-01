package generators

import (
	"github.com/invopop/jsonschema"

	"github.com/AltairaLabs/PromptKit/runtime/v2/composition"
	"github.com/AltairaLabs/PromptKit/runtime/v2/workflow"
)

// An arena config's `workflow:` and `compositions:` are the generated packspec
// types (workflow.Spec, composition.Composition). Like Governance, those types
// carry no jsonschema tags, so the reflector emits every enum as a bare string
// and every union as whatever its Go flattening happens to look like. The hooks
// below put back what the PromptPack spec says, so an editor can complete these
// blocks and a typo fails schema validation instead of surfacing at run time.

// applyWorkflowEnums closes the enums on a workflow state and an artifact slot.
//
// The values come from promptkit's own constants, as applyGovernanceEnums does,
// so the schema accepts exactly what the runtime's workflow validator accepts.
// orchestration, control and artifact mode are closed enums in the spec too.
// persistence has no enum in the spec, but workflow.Validate rejects anything
// but these two values, so leaving it open would only defer the error.
func applyWorkflowEnums(schema *jsonschema.Schema) {
	if schema == nil || schema.Definitions == nil {
		return
	}
	for _, def := range definitionsNamed(schema, "WorkflowState") {
		setEnum(def, "orchestration",
			workflow.OrchestrationInternal, workflow.OrchestrationExternal,
			workflow.OrchestrationHybrid, workflow.OrchestrationComposition)
		setEnum(def, "control", workflow.ControlUser, workflow.ControlAgent)
		setEnum(def, "persistence", workflow.PersistenceTransient, workflow.PersistencePersistent)
	}
	for _, def := range definitionsNamed(schema, "ArtifactDef") {
		setEnum(def, "mode", workflow.ArtifactModeReplace, workflow.ArtifactModeAppend)
	}
}

// compareOpsPointer is the spec's enum of compare-predicate operators. promptkit
// keeps its copy unexported, so the spec is the only source to read it from.
const compareOpsPointer = "/$defs/ComparePredicate/properties/op/enum"

// applyCompositionShapes repairs the composition definitions the reflector gets
// wrong or leaves loose.
//
//   - Step.kind and Reducer.strategy are free-form in the spec ("conventional
//     values"; vendor kinds such as omnia.judge, reducers reserved for future
//     RFCs), so they get an open enum: the known values are suggested, any
//     string is accepted.
//   - Predicate.op is a closed enum in the spec.
//   - StepInput is a union of a "${...}" reference string and an object. Its Go
//     type holds the two shapes in json:"-" fields, so the reflector saw no
//     properties and emitted a closed empty object that rejects both.
//   - A step requires id and kind, and a termination predicate requires at
//     least one of max_steps and tool_called.
func applyCompositionShapes(schema *jsonschema.Schema) {
	if schema == nil || schema.Definitions == nil {
		return
	}
	for _, def := range definitionsNamed(schema, "Step") {
		applyOpenTypeEnum(def, "kind", []string{
			composition.KindPrompt, composition.KindAgent, composition.KindTool,
			composition.KindBranch, composition.KindParallel,
		}, "Step kind. The v1 kinds are suggested; a runtime may support additional vendor-namespaced kinds.")
		def.Required = []string{"id", "kind"}
	}
	for _, def := range definitionsNamed(schema, "Reducer") {
		applyOpenTypeEnum(def, "strategy", []string{
			composition.ReduceAppend, composition.ReduceReplace, composition.ReduceBarrier,
		}, "How branch outputs merge: 'append' extends lists, 'replace' keeps the last write, "+
			"'barrier' collects every output into a named map.")
	}
	for _, def := range definitionsNamed(schema, "Predicate") {
		if prop, ok := def.Properties.Get("op"); ok && prop != nil {
			prop.Enum = mustSpecEnum(compareOpsPointer)
		}
	}
	for _, def := range definitionsNamed(schema, "TerminationPredicate") {
		def.AnyOf = []*jsonschema.Schema{
			{Required: []string{"max_steps"}},
			{Required: []string{"tool_called"}},
		}
	}
	for _, def := range definitionsNamed(schema, "StepInput") {
		*def = jsonschema.Schema{
			Description: def.Description,
			OneOf: []*jsonschema.Schema{
				{Type: jsonTypeString, Description: "Reference of the form '${path.to.value}'."},
				{Type: "object", AdditionalProperties: jsonschema.TrueSchema},
			},
		}
	}
}

// setEnum closes def's named property to values. A missing property is left
// alone: the hook must not invent a field the reflected type lacks.
func setEnum(def *jsonschema.Schema, prop string, values ...string) {
	if def == nil || def.Properties == nil {
		return
	}
	p, ok := def.Properties.Get(prop)
	if !ok || p == nil {
		return
	}
	enum := make([]interface{}, len(values))
	for i, v := range values {
		enum[i] = v
	}
	p.Enum = enum
}

// mustSpecEnum reads an enum array from the embedded spec for a Customize hook,
// which cannot return an error; a pointer that stops resolving panics, failing
// generation the same way mustSpecPattern does.
func mustSpecEnum(pointer string) []interface{} {
	spec, err := parseEmbeddedSpec()
	if err != nil {
		panic(err)
	}
	node, err := resolvePointer(spec, pointer)
	if err != nil {
		panic(err)
	}
	enum, ok := node.([]interface{})
	if !ok || len(enum) == 0 {
		panic(pointer + " is not a non-empty enum")
	}
	return enum
}
