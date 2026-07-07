package web

import (
	"fmt"
	"sort"

	"github.com/AltairaLabs/PromptKit/runtime/composition"
	"github.com/AltairaLabs/PromptKit/runtime/workflow"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
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
		return WorkflowGraph{
			Nodes: []WorkflowGraphNode{
				{ID: "default", Label: "default", Kind: "entry", Terminal: true},
			},
			Edges: []WorkflowGraphEdge{},
		}, nil
	}

	spec, err := workflow.ParseConfig(cfg.Workflow)
	if err != nil {
		return WorkflowGraph{}, fmt.Errorf("parsing workflow config: %w", err)
	}

	comps, err := composition.ParseConfig(cfg.Compositions)
	if err != nil {
		return WorkflowGraph{}, fmt.Errorf("parsing compositions config: %w", err)
	}

	names := make([]string, 0, len(spec.States))
	for name := range spec.States {
		names = append(names, name)
	}
	sort.Strings(names)

	graph := WorkflowGraph{
		Nodes: make([]WorkflowGraphNode, 0, len(names)),
		Edges: []WorkflowGraphEdge{},
	}

	for _, name := range names {
		state := spec.States[name]
		terminal := state.Terminal || len(state.OnEvent) == 0
		kind := "agent"
		switch {
		case name == spec.Entry:
			kind = "entry"
		case terminal:
			kind = "output"
		}

		graph.Nodes = append(graph.Nodes, WorkflowGraphNode{
			ID:       name,
			Label:    name,
			Kind:     kind,
			Entry:    name == spec.Entry,
			Terminal: terminal,
		})

		events := make([]string, 0, len(state.OnEvent))
		for event := range state.OnEvent {
			events = append(events, event)
		}
		sort.Strings(events)

		for _, event := range events {
			graph.Edges = append(graph.Edges, WorkflowGraphEdge{
				From:  name,
				To:    state.OnEvent[event],
				Label: event,
			})
		}

		if state.OnMaxVisits != "" {
			graph.Edges = append(graph.Edges, WorkflowGraphEdge{
				From:   name,
				To:     state.OnMaxVisits,
				Label:  "max-visits",
				Dashed: true,
			})
		}

		if state.Orchestration == workflow.OrchestrationComposition && state.Composition != "" {
			if comp := comps[state.Composition]; comp != nil {
				stepNodes, stepEdges := expandComposition(name, comp)
				graph.Nodes = append(graph.Nodes, stepNodes...)
				graph.Edges = append(graph.Edges, stepEdges...)
			}
		}
	}

	graph.Edges = dedupeEdges(graph.Edges)

	return graph, nil
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
	prefix := func(id string) string { return stateName + "/" + id }

	var nodes []WorkflowGraphNode
	var edges []WorkflowGraphEdge

	// armTarget marks step ids that are the then/else target of a top-level
	// branch step. Such a step's sole implicit predecessor is the branch
	// edge added below; it must never also chain from list order or the
	// join frontier.
	armTarget := make(map[string]bool)
	for _, s := range comp.Steps {
		if s != nil && s.Kind == composition.KindBranch {
			if s.Then != "" {
				armTarget[s.Then] = true
			}
			if s.Else != "" {
				armTarget[s.Else] = true
			}
		}
	}

	// frontier is the implicit-predecessor set for whichever top-level step
	// comes next without its own DependsOn. A branch step seeds it with its
	// arms (refined below as each arm is actually walked); a parallel step
	// sets it to itself; a plain leaf step sets it to itself.
	var frontier []string
	// openArms tracks branch arms seeded into frontier that haven't been
	// walked yet; joinAccum accumulates the arms actually walked so far so
	// frontier can collapse to the true join once openArms empties.
	openArms := map[string]bool{}
	var joinAccum []string

	var walk func(steps []*composition.Step, topLevel bool)
	walk = func(steps []*composition.Step, topLevel bool) {
		for i, step := range steps {
			if step == nil {
				continue
			}
			nodes = append(nodes, WorkflowGraphNode{
				ID:     prefix(step.ID),
				Label:  step.ID,
				Kind:   compositionStepKind(step.Kind),
				Parent: stateName,
			})

			switch {
			case len(step.DependsOn) > 0:
				deps := append([]string(nil), step.DependsOn...)
				sort.Strings(deps)
				for _, dep := range deps {
					edges = append(edges, WorkflowGraphEdge{From: prefix(dep), To: prefix(step.ID)})
				}
			case topLevel && armTarget[step.ID]:
				// Predecessor already added via the branch's then/else edge.
			case topLevel && i == 0:
				edges = append(edges, WorkflowGraphEdge{From: stateName, To: prefix(step.ID)})
			case topLevel:
				for _, from := range frontier {
					edges = append(edges, WorkflowGraphEdge{From: prefix(from), To: prefix(step.ID)})
				}
			}

			// Resolve this step against any pending branch join.
			resolvedArm := false
			if topLevel && openArms[step.ID] {
				delete(openArms, step.ID)
				joinAccum = append(joinAccum, step.ID)
				resolvedArm = true
				if len(openArms) == 0 {
					frontier = append([]string(nil), joinAccum...)
				}
			}

			switch step.Kind {
			case composition.KindBranch:
				if step.Then != "" {
					edges = append(edges, WorkflowGraphEdge{From: prefix(step.ID), To: prefix(step.Then)})
				}
				if step.Else != "" {
					edges = append(edges, WorkflowGraphEdge{From: prefix(step.ID), To: prefix(step.Else), Dashed: true})
				}
				if topLevel {
					arms := distinctNonEmpty(step.Then, step.Else)
					newOpen := make(map[string]bool, len(arms))
					for _, a := range arms {
						newOpen[a] = true
					}
					openArms = newOpen
					joinAccum = nil
					frontier = arms
				}
			case composition.KindParallel:
				branches := make([]*composition.Step, 0, len(step.Branches))
				for _, b := range step.Branches {
					if b == nil {
						continue
					}
					branches = append(branches, b)
				}
				sort.Slice(branches, func(i, j int) bool { return branches[i].ID < branches[j].ID })
				for _, b := range branches {
					edges = append(edges, WorkflowGraphEdge{From: prefix(step.ID), To: prefix(b.ID)})
				}
				walk(step.Branches, false)
				if topLevel {
					frontier = []string{step.ID}
					openArms = map[string]bool{}
					joinAccum = nil
				}
			default:
				// Plain leaf step (prompt/agent/tool). If it just resolved a
				// pending branch join (whether or not that closed it out),
				// frontier bookkeeping above already reflects the right
				// state -- leave it alone. Otherwise this step becomes the
				// new single-node frontier for whatever follows.
				if topLevel && !resolvedArm {
					frontier = []string{step.ID}
					openArms = map[string]bool{}
					joinAccum = nil
				}
			}
		}
	}

	walk(comp.Steps, true)

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return !edges[i].Dashed && edges[j].Dashed
	})

	return nodes, edges
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
