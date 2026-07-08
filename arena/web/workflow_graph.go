package web

import (
	"fmt"
	"sort"

	"github.com/AltairaLabs/PromptKit/runtime/composition"
	"github.com/AltairaLabs/PromptKit/runtime/workflow"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

const (
	// defaultStateID is the synthetic single-state id used when a config has
	// no workflow spec (Arena's implicit single-state workflow).
	defaultStateID = "default"
	// kindEntry/kindOutput/kindAgent are the WorkflowGraphNode.Kind values for
	// workflow *state* nodes (as opposed to composition step nodes, see
	// compositionStepKind).
	kindEntry  = "entry"
	kindOutput = "output"
	kindAgent  = "agent"
)

// WorkflowGraphNode is a single state in the workflow topology graph.
type WorkflowGraphNode struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Kind     string `json:"kind"` // entry|output|agent (composition kinds added in a later task)
	Entry    bool   `json:"entry"`
	Terminal bool   `json:"terminal"`
	// Parent is the owning workflow state's name for composition-step nodes
	// (including nested parallel-branch steps), letting the frontend
	// group/collapse a composition's steps under their state. Empty for
	// workflow state nodes themselves.
	Parent string `json:"parent,omitempty"`
}

// WorkflowGraphEdge is a single transition between two states in the
// workflow topology graph.
type WorkflowGraphEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Label  string `json:"label,omitempty"`
	Dashed bool   `json:"dashed,omitempty"`
}

// WorkflowGraph is the full workflow topology: its states (nodes) and
// transitions (edges).
type WorkflowGraph struct {
	Nodes []WorkflowGraphNode `json:"nodes"`
	Edges []WorkflowGraphEdge `json:"edges"`
}

// BuildWorkflowGraph turns a config's workflow spec into a topology graph.
// cfg == nil or cfg.Workflow == nil produces a single "default" node with no
// edges (Arena's implicit single-state workflow). Node and edge ordering is
// deterministic (sorted by state name, then event name).
func BuildWorkflowGraph(cfg *arenaconfig.Config) (WorkflowGraph, error) {
	if cfg == nil || cfg.Workflow == nil {
		return defaultWorkflowGraph(), nil
	}

	spec, err := workflow.ParseConfig(cfg.Workflow)
	if err != nil {
		return WorkflowGraph{}, fmt.Errorf("parsing workflow config: %w", err)
	}

	comps, err := composition.ParseConfig(cfg.Compositions)
	if err != nil {
		return WorkflowGraph{}, fmt.Errorf("parsing compositions config: %w", err)
	}

	names := sortedStateNames(spec.States)

	graph := WorkflowGraph{
		Nodes: make([]WorkflowGraphNode, 0, len(names)),
		Edges: []WorkflowGraphEdge{},
	}

	for _, name := range names {
		state := spec.States[name]

		graph.Nodes = append(graph.Nodes, stateNode(name, state, spec.Entry))
		graph.Edges = append(graph.Edges, stateEdges(name, state)...)

		stepNodes, stepEdges := compositionStepsFor(name, state, comps)
		graph.Nodes = append(graph.Nodes, stepNodes...)
		graph.Edges = append(graph.Edges, stepEdges...)
	}

	graph.Edges = dedupeEdges(graph.Edges)

	return graph, nil
}

// defaultWorkflowGraph is Arena's implicit single-state workflow, used when a
// config has no workflow spec at all.
func defaultWorkflowGraph() WorkflowGraph {
	return WorkflowGraph{
		Nodes: []WorkflowGraphNode{
			{ID: defaultStateID, Label: defaultStateID, Kind: kindEntry, Terminal: true},
		},
		Edges: []WorkflowGraphEdge{},
	}
}

// sortedStateNames returns the workflow spec's state names in deterministic
// (alphabetical) order.
func sortedStateNames(states map[string]*workflow.State) []string {
	names := make([]string, 0, len(states))
	for name := range states {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// stateNode builds the WorkflowGraphNode for a single workflow state.
func stateNode(name string, state *workflow.State, entry string) WorkflowGraphNode {
	terminal := state.Terminal || len(state.OnEvent) == 0
	kind := kindAgent
	switch {
	case name == entry:
		kind = kindEntry
	case terminal:
		kind = kindOutput
	}

	return WorkflowGraphNode{
		ID:       name,
		Label:    name,
		Kind:     kind,
		Entry:    name == entry,
		Terminal: terminal,
	}
}

// stateEdges builds the outgoing on_event edges (sorted by event name) and,
// if present, the dashed on_max_visits edge for a single workflow state.
func stateEdges(name string, state *workflow.State) []WorkflowGraphEdge {
	events := make([]string, 0, len(state.OnEvent))
	for event := range state.OnEvent {
		events = append(events, event)
	}
	sort.Strings(events)

	edges := make([]WorkflowGraphEdge, 0, len(events)+1)
	for _, event := range events {
		edges = append(edges, WorkflowGraphEdge{
			From:  name,
			To:    state.OnEvent[event],
			Label: event,
		})
	}

	if state.OnMaxVisits != "" {
		edges = append(edges, WorkflowGraphEdge{
			From:   name,
			To:     state.OnMaxVisits,
			Label:  "max-visits",
			Dashed: true,
		})
	}

	return edges
}

// compositionStepsFor returns the composition step nodes/edges scoped under
// state name, if that state runs a composition and it's found in comps.
// Returns nil, nil otherwise.
func compositionStepsFor(
	name string, state *workflow.State, comps map[string]*composition.Composition,
) ([]WorkflowGraphNode, []WorkflowGraphEdge) {
	if state.Orchestration != workflow.OrchestrationComposition || state.Composition == "" {
		return nil, nil
	}
	comp := comps[state.Composition]
	if comp == nil {
		return nil, nil
	}
	return expandComposition(name, comp)
}

// dedupeEdges removes duplicate edges (same From, To, and Dashed) from the
// whole graph's edge set, keeping the first occurrence and preserving order.
// Duplicates can arise when a state-machine or composition edge is derivable
// two ways — e.g. a branch step's "then" target that also declares a
// depends_on back to the branch step (the runtime's recommended join-hint
// pattern) produces the same edge from both the Then/Else loop and the
// DependsOn loop in expandComposition.
func dedupeEdges(edges []WorkflowGraphEdge) []WorkflowGraphEdge {
	type key struct {
		from, to string
		dashed   bool
	}
	seen := make(map[key]bool, len(edges))
	out := make([]WorkflowGraphEdge, 0, len(edges))
	for _, e := range edges {
		k := key{e.From, e.To, e.Dashed}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out
}

// expandComposition inlines a composition's step graph as nodes/edges scoped
// under stateName. Step ids are prefixed "<stateName>/<stepID>" to avoid
// collisions with workflow state ids and other states' composition steps
// (composition.Validate enforces step id uniqueness within a composition,
// including nested parallel.branches, so the prefix alone is enough).
//
// Edges follow how the runtime's composition engine actually executes the
// step list (github.com/AltairaLabs/PromptKit/runtime/composition/engine):
// top-level steps run in list order, so a step with no explicit DependsOn
// implicitly follows whatever ran immediately before it in the flow —
// *not* necessarily the literal previous list entry:
//
//   - The composition's entry point is only the first top-level step; it
//     gets the sole edge from the state node itself.
//   - A step with an explicit DependsOn always honors it instead of
//     implicit sequencing.
//   - A branch step's Then/Else targets get their edges from the branch
//     (solid/dashed); they never additionally chain from the literal
//     previous step, which would wrongly link one arm to the other (e.g.
//     extract_paper -> extract_general).
//   - Once both a branch's arms have been walked, the next step with no
//     explicit DependsOn gets an implicit predecessor edge from *both*
//     arms (the join), not just the last one.
//   - A parallel step fans out to its branches; the next step with no
//     explicit DependsOn implicitly follows the parallel step itself
//     (the barrier/join), matching the runtime's own step-edge model
//     (runtime/composition/validate.go's buildStepEdges advances its
//     "prev" pointer over top-level steps only — nested parallel
//     branches never update it).
//
// Nested branch steps inside a parallel step's Branches never get a
// state-entry edge or implicit sequential edge — they are reached only via
// the parallel step's fan-out edge (or an explicit DependsOn of their own).
//
// Known limitation: the frontier/openArms/joinAccum bookkeeping below
// assumes a branch (or parallel) step's arms are walked — as top-level
// steps — before the frontier is consumed by whatever comes next. That
// holds for the idiomatic shape (document-analysis: branch, then both arms
// contiguously, then the join), but nothing enforces it. Two back-to-back
// top-level branches with no DependsOn (e.g. routeA(then=x,else=y) followed
// immediately by routeB(then=p,else=q), with x/y/p/q listed afterward) will
// have routeB overwrite routeA's still-open arm bookkeeping while frontier
// still holds routeA's unwalked arms, producing causally-backwards edges
// (x->routeB, y->routeB) once x and y are finally walked. See
// TestBuildWorkflowGraph_CompositionExpansion_BackToBackBranches_KnownLimitation,
// which pins this approximate output. Compositions with non-contiguous arms
// or consecutive top-level branches should use explicit DependsOn to get
// correct edges instead of relying on implicit sequencing.
func expandComposition(stateName string, comp *composition.Composition) ([]WorkflowGraphNode, []WorkflowGraphEdge) {
	ex := &compositionExpander{
		stateName: stateName,
		armTarget: branchArmTargets(comp.Steps),
		openArms:  map[string]bool{},
	}
	ex.walk(comp.Steps, true)
	return ex.sortedResult()
}

// compositionExpander holds the mutable bookkeeping expandComposition needs
// while walking a composition's step tree: the nodes/edges accumulated so
// far, and the frontier/openArms/joinAccum state used to derive implicit
// (DependsOn-free) sequencing edges. See expandComposition's doc comment for
// the edge-derivation rules this implements, including the known limitation
// around back-to-back top-level branches.
type compositionExpander struct {
	stateName string
	nodes     []WorkflowGraphNode
	edges     []WorkflowGraphEdge

	// armTarget marks step ids that are the then/else target of a top-level
	// branch step. Such a step's sole implicit predecessor is the branch
	// edge added by applyBranch; it must never also chain from list order or
	// the join frontier.
	armTarget map[string]bool

	// frontier is the implicit-predecessor set for whichever top-level step
	// comes next without its own DependsOn. A branch step seeds it with its
	// arms (refined as each arm is actually walked); a parallel step sets it
	// to itself; a plain leaf step sets it to itself.
	frontier []string
	// openArms tracks branch arms seeded into frontier that haven't been
	// walked yet; joinAccum accumulates the arms actually walked so far so
	// frontier can collapse to the true join once openArms empties.
	openArms  map[string]bool
	joinAccum []string
}

// branchArmTargets returns the set of step ids that are the then/else target
// of a top-level branch step in steps.
func branchArmTargets(steps []*composition.Step) map[string]bool {
	armTarget := make(map[string]bool)
	for _, s := range steps {
		if s != nil && s.Kind == composition.KindBranch {
			if s.Then != "" {
				armTarget[s.Then] = true
			}
			if s.Else != "" {
				armTarget[s.Else] = true
			}
		}
	}
	return armTarget
}

func (ex *compositionExpander) prefix(id string) string {
	return ex.stateName + "/" + id
}

// walk emits a node for each step in steps (recursing into parallel
// branches) and derives its incoming/outgoing edges. topLevel is false for
// steps nested inside a parallel step's Branches.
func (ex *compositionExpander) walk(steps []*composition.Step, topLevel bool) {
	for i, step := range steps {
		if step == nil {
			continue
		}
		ex.nodes = append(ex.nodes, WorkflowGraphNode{
			ID:     ex.prefix(step.ID),
			Label:  step.ID,
			Kind:   compositionStepKind(step.Kind),
			Parent: ex.stateName,
		})

		ex.addPredecessorEdges(step, i, topLevel)
		resolvedArm := ex.resolveJoin(step, topLevel)
		ex.applyStepKind(step, topLevel, resolvedArm)
	}
}

// addPredecessorEdges adds the edge(s) leading into step: an explicit
// DependsOn, a branch-arm edge already added by applyBranch, the
// composition's sole state-entry edge (the first top-level step), or the
// implicit frontier edges carried over from whatever ran before it.
func (ex *compositionExpander) addPredecessorEdges(step *composition.Step, i int, topLevel bool) {
	switch {
	case len(step.DependsOn) > 0:
		deps := append([]string(nil), step.DependsOn...)
		sort.Strings(deps)
		for _, dep := range deps {
			ex.edges = append(ex.edges, WorkflowGraphEdge{From: ex.prefix(dep), To: ex.prefix(step.ID)})
		}
	case topLevel && ex.armTarget[step.ID]:
		// Predecessor already added via the branch's then/else edge.
	case topLevel && i == 0:
		ex.edges = append(ex.edges, WorkflowGraphEdge{From: ex.stateName, To: ex.prefix(step.ID)})
	case topLevel:
		for _, from := range ex.frontier {
			ex.edges = append(ex.edges, WorkflowGraphEdge{From: ex.prefix(from), To: ex.prefix(step.ID)})
		}
	}
}

// resolveJoin checks step against any pending branch join (an arm seeded
// into openArms by applyBranch), updating joinAccum/frontier once every arm
// has been walked. Returns whether step resolved a pending arm.
func (ex *compositionExpander) resolveJoin(step *composition.Step, topLevel bool) bool {
	if !topLevel || !ex.openArms[step.ID] {
		return false
	}
	delete(ex.openArms, step.ID)
	ex.joinAccum = append(ex.joinAccum, step.ID)
	if len(ex.openArms) == 0 {
		ex.frontier = append([]string(nil), ex.joinAccum...)
	}
	return true
}

// applyStepKind adds step's own outgoing edges (branch then/else, parallel
// fan-out) and updates the frontier bookkeeping for whatever implicitly
// follows it.
func (ex *compositionExpander) applyStepKind(step *composition.Step, topLevel, resolvedArm bool) {
	switch step.Kind {
	case composition.KindBranch:
		ex.applyBranch(step, topLevel)
	case composition.KindParallel:
		ex.applyParallel(step, topLevel)
	case composition.KindPrompt, composition.KindAgent, composition.KindTool:
		ex.applyLeaf(step, topLevel, resolvedArm)
	default:
		ex.applyLeaf(step, topLevel, resolvedArm)
	}
}

// applyBranch adds the branch's solid then / dashed else edges and, for a
// top-level branch, seeds openArms/frontier with its arms.
func (ex *compositionExpander) applyBranch(step *composition.Step, topLevel bool) {
	if step.Then != "" {
		ex.edges = append(ex.edges, WorkflowGraphEdge{From: ex.prefix(step.ID), To: ex.prefix(step.Then)})
	}
	if step.Else != "" {
		ex.edges = append(ex.edges, WorkflowGraphEdge{From: ex.prefix(step.ID), To: ex.prefix(step.Else), Dashed: true})
	}
	if !topLevel {
		return
	}
	arms := distinctNonEmpty(step.Then, step.Else)
	newOpen := make(map[string]bool, len(arms))
	for _, a := range arms {
		newOpen[a] = true
	}
	ex.openArms = newOpen
	ex.joinAccum = nil
	ex.frontier = arms
}

// applyParallel fans out to step's branches (recursing via walk), and, for a
// top-level parallel step, sets the frontier to the parallel step itself
// (the barrier/join) for whatever implicitly follows.
func (ex *compositionExpander) applyParallel(step *composition.Step, topLevel bool) {
	branches := make([]*composition.Step, 0, len(step.Branches))
	for _, b := range step.Branches {
		if b == nil {
			continue
		}
		branches = append(branches, b)
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].ID < branches[j].ID })
	for _, b := range branches {
		ex.edges = append(ex.edges, WorkflowGraphEdge{From: ex.prefix(step.ID), To: ex.prefix(b.ID)})
	}
	ex.walk(step.Branches, false)
	if !topLevel {
		return
	}
	ex.frontier = []string{step.ID}
	ex.openArms = map[string]bool{}
	ex.joinAccum = nil
}

// applyLeaf handles a plain leaf step (prompt/agent/tool). If it just
// resolved a pending branch join (whether or not that closed it out), the
// frontier bookkeeping resolveJoin already applied is left alone. Otherwise
// this step becomes the new single-node frontier for whatever follows.
func (ex *compositionExpander) applyLeaf(step *composition.Step, topLevel, resolvedArm bool) {
	if topLevel && !resolvedArm {
		ex.frontier = []string{step.ID}
		ex.openArms = map[string]bool{}
		ex.joinAccum = nil
	}
}

// sortedResult sorts the accumulated nodes/edges into the deterministic
// order BuildWorkflowGraph promises (nodes by id; edges by From, then To,
// then solid-before-dashed) and returns them.
func (ex *compositionExpander) sortedResult() ([]WorkflowGraphNode, []WorkflowGraphEdge) {
	sort.Slice(ex.nodes, func(i, j int) bool { return ex.nodes[i].ID < ex.nodes[j].ID })
	sort.Slice(ex.edges, func(i, j int) bool {
		if ex.edges[i].From != ex.edges[j].From {
			return ex.edges[i].From < ex.edges[j].From
		}
		if ex.edges[i].To != ex.edges[j].To {
			return ex.edges[i].To < ex.edges[j].To
		}
		return !ex.edges[i].Dashed && ex.edges[j].Dashed
	})
	return ex.nodes, ex.edges
}

// distinctNonEmpty returns the non-empty, de-duplicated members of a and b,
// preserving order (a before b). Used to collapse a branch step's then/else
// targets into its join arms -- a and b are equal when a branch's then and
// else point at the same step (an immediate converge).
func distinctNonEmpty(a, b string) []string {
	var out []string
	if a != "" {
		out = append(out, a)
	}
	if b != "" && b != a {
		out = append(out, b)
	}
	return out
}

// compositionStepKind maps a composition.StepKind to the WorkflowGraphNode
// Kind used by the frontend. parallel collapses into "branch" alongside
// branch since both are non-leaf control-flow steps.
func compositionStepKind(kind composition.StepKind) string {
	switch kind {
	case composition.KindPrompt:
		return "prompt"
	case composition.KindAgent:
		return "agent"
	case composition.KindTool:
		return "tool"
	case composition.KindBranch, composition.KindParallel:
		return "branch"
	default:
		return string(kind)
	}
}
