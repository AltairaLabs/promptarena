import { describe, it, expect } from "vitest";
import { buildFlowElements } from "./workflowFlow";
import type { WorkflowGraph, RunResult } from "@/types";

// documentAnalysisGraph mirrors the backend's composition-expansion shape:
// a top-level `analyzing` state whose composition steps are prefixed
// `analyzing/...` and carry `parent: "analyzing"`; the state's sole
// out-edge into the composition targets the entry step, per
// arena/web/workflow_graph.go's expandComposition.
const documentAnalysisGraph: WorkflowGraph = {
  nodes: [
    { id: "default", label: "default", kind: "entry", entry: true, terminal: false },
    { id: "analyzing", label: "analyzing", kind: "agent", entry: false, terminal: false },
    { id: "done", label: "done", kind: "output", entry: false, terminal: true },
    { id: "analyzing/classify", label: "classify", kind: "prompt", entry: false, terminal: false, parent: "analyzing" },
    { id: "analyzing/route", label: "route", kind: "branch", entry: false, terminal: false, parent: "analyzing" },
  ],
  edges: [
    { from: "default", to: "analyzing" },
    { from: "analyzing", to: "analyzing/classify" },
    { from: "analyzing/classify", to: "analyzing/route" },
    { from: "analyzing", to: "done" },
  ],
};

function mkRun(o: Partial<RunResult>): RunResult {
  return {
    RunID: "r", PromptPack: "", Region: "", ScenarioID: "s", ProviderID: "p",
    Params: {}, Messages: [], Commit: {},
    Cost: { total_cost_usd: 0, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 },
    Violations: [], StartTime: "2026-07-07T00:00:00Z", EndTime: "2026-07-07T00:00:01Z",
    Duration: 1000, Error: "", SelfPlay: false, PersonaID: "", MediaOutputs: [], A2AAgents: [],
    ...o,
  } as RunResult;
}

function expectFinitePositions(nodes: { position: { x: number; y: number } }[]) {
  for (const n of nodes) {
    expect(Number.isFinite(n.position.x)).toBe(true);
    expect(Number.isFinite(n.position.y)).toBe(true);
  }
}

describe("buildFlowElements — collapsed", () => {
  const toggles = { expandedStates: [], thisRunOnly: false };

  it("keeps only state nodes, drops steps, and flags hasComposition on the owning state", () => {
    const { nodes, edges } = buildFlowElements(documentAnalysisGraph, undefined, toggles);

    const ids = nodes.map((n) => n.id).sort();
    expect(ids).toEqual(["__end", "__start", "analyzing", "default", "done"]);

    const analyzing = nodes.find((n) => n.id === "analyzing")!;
    expect(analyzing.data.hasComposition).toBe(true);
    expect(analyzing.type).toBe("workflowNode");

    const defaultNode = nodes.find((n) => n.id === "default")!;
    expect(defaultNode.data.hasComposition).toBeFalsy();

    // No edge should touch a step id.
    for (const e of edges) {
      expect(e.source.startsWith("analyzing/")).toBe(false);
      expect(e.target.startsWith("analyzing/")).toBe(false);
    }
    // The state->state edge survives.
    expect(edges.some((e) => e.source === "analyzing" && e.target === "done")).toBe(true);
    expect(edges.some((e) => e.source === "default" && e.target === "analyzing")).toBe(true);

    // The synthetic start/end terminators bracket the declared entry
    // ("default", entry: true) and terminal ("done", terminal: true) states.
    expect(edges.some((e) => e.source === "__start" && e.target === "default")).toBe(true);
    expect(edges.some((e) => e.source === "done" && e.target === "__end")).toBe(true);

    expectFinitePositions(nodes);
  });
});

describe("buildFlowElements — expanded (per-state)", () => {
  const toggles = { expandedStates: ["analyzing"], thisRunOnly: false };

  it("includes step nodes nested under a group node for the owning state", () => {
    const { nodes, edges } = buildFlowElements(documentAnalysisGraph, undefined, toggles);

    const group = nodes.find((n) => n.id === "grp:analyzing");
    expect(group).toBeDefined();
    expect(group!.type).toBe("group");
    expect(group!.data.isGroup).toBe(true);
    expect(group!.data.stateId).toBe("analyzing");

    const classify = nodes.find((n) => n.id === "analyzing/classify")!;
    const route = nodes.find((n) => n.id === "analyzing/route")!;
    expect(classify.parentId).toBe("grp:analyzing");
    expect(classify.extent).toBe("parent");
    expect(route.parentId).toBe("grp:analyzing");

    // The plain "analyzing" state node is replaced by the group; there
    // should be no separate non-group node with that raw id.
    expect(nodes.some((n) => n.id === "analyzing" && n.type !== "group")).toBe(false);

    // Composition-internal edge is kept.
    expect(edges.some((e) => e.source === "analyzing/classify" && e.target === "analyzing/route")).toBe(true);
    // The state->entry-step edge is remapped to originate from the group.
    expect(edges.some((e) => e.source === "grp:analyzing" && e.target === "analyzing/classify")).toBe(true);
    // The state->state edge out of the composition-owning state is also
    // remapped to originate from the group so it renders from the container.
    expect(edges.some((e) => e.source === "grp:analyzing" && e.target === "done")).toBe(true);
    // The start/end terminators are still present (analyzing isn't the
    // entry/terminal state in this fixture, so their edges are unaffected).
    expect(edges.some((e) => e.source === "__start" && e.target === "default")).toBe(true);
    expect(edges.some((e) => e.source === "done" && e.target === "__end")).toBe(true);

    expectFinitePositions(nodes);
    // Step positions are relative to the group, so they should be smaller
    // in magnitude than the group's own absolute position in typical cases;
    // at minimum they must be finite (checked above) and the group must
    // have a sized style box.
    expect(typeof group!.style?.width).toBe("number");
    expect(typeof group!.style?.height).toBe("number");
  });

  it("leaves other composition-owning states collapsed when only one id is in expandedStates", () => {
    const graph: WorkflowGraph = {
      nodes: [
        { id: "default", label: "default", kind: "entry", entry: true, terminal: false },
        { id: "analyzing", label: "analyzing", kind: "agent", entry: false, terminal: false },
        { id: "reviewing", label: "reviewing", kind: "agent", entry: false, terminal: true },
        { id: "analyzing/classify", label: "classify", kind: "prompt", entry: false, terminal: false, parent: "analyzing" },
        { id: "reviewing/check", label: "check", kind: "prompt", entry: false, terminal: false, parent: "reviewing" },
      ],
      edges: [
        { from: "default", to: "analyzing" },
        { from: "analyzing", to: "analyzing/classify" },
        { from: "analyzing", to: "reviewing" },
        { from: "reviewing", to: "reviewing/check" },
      ],
    };
    const { nodes } = buildFlowElements(graph, undefined, { expandedStates: ["analyzing"], thisRunOnly: false });

    expect(nodes.some((n) => n.id === "grp:analyzing")).toBe(true);
    expect(nodes.some((n) => n.id === "analyzing/classify")).toBe(true);

    // "reviewing" owns a composition too but isn't in expandedStates, so it
    // stays a single node and its step doesn't render.
    const reviewing = nodes.find((n) => n.id === "reviewing")!;
    expect(reviewing).toBeDefined();
    expect(reviewing.type).toBe("workflowNode");
    expect(reviewing.data.hasComposition).toBe(true);
    expect(nodes.some((n) => n.id === "reviewing/check")).toBe(false);
    expect(nodes.some((n) => n.id === "grp:reviewing")).toBe(false);
  });
});

describe("buildFlowElements — start/end terminators", () => {
  // A single state that is both entry and terminal, and owns a composition
  // — the document-analysis shape: collapsed reads start -> analyzing(⤵) ->
  // end; expanded remaps both terminator edges onto the group.
  const singleCompositionGraph: WorkflowGraph = {
    nodes: [
      { id: "analyzing", label: "analyzing", kind: "agent", entry: true, terminal: true },
      { id: "analyzing/classify", label: "classify", kind: "prompt", entry: false, terminal: false, parent: "analyzing" },
    ],
    edges: [{ from: "analyzing", to: "analyzing/classify" }],
  };

  it("brackets the sole state with start/end when collapsed", () => {
    const { nodes, edges } = buildFlowElements(singleCompositionGraph, undefined, {
      expandedStates: [],
      thisRunOnly: false,
    });

    expect(nodes.map((n) => n.id).sort()).toEqual(["__end", "__start", "analyzing"]);
    expect(edges.some((e) => e.source === "__start" && e.target === "analyzing")).toBe(true);
    expect(edges.some((e) => e.source === "analyzing" && e.target === "__end")).toBe(true);
    expectFinitePositions(nodes);
  });

  it("remaps start/end edges onto the group once the state is expanded", () => {
    const { edges } = buildFlowElements(singleCompositionGraph, undefined, {
      expandedStates: ["analyzing"],
      thisRunOnly: false,
    });

    expect(edges.some((e) => e.source === "__start" && e.target === "grp:analyzing")).toBe(true);
    expect(edges.some((e) => e.source === "grp:analyzing" && e.target === "__end")).toBe(true);
  });

  it("keeps __start/__end outside the group's bounding box when the sole state is both entry and terminal", () => {
    // "analyzing" is entry AND terminal, so dagre wires __start -> analyzing
    // and analyzing -> __end as flat siblings of analyzing's own (invisible,
    // once expanded) seed node — without a fix, __end can land at the same
    // rank as the group's first step, inside the group's x-range.
    const { nodes } = buildFlowElements(singleCompositionGraph, undefined, {
      expandedStates: ["analyzing"],
      thisRunOnly: false,
    });

    const group = nodes.find((n) => n.id === "grp:analyzing")!;
    const start = nodes.find((n) => n.id === "__start")!;
    const end = nodes.find((n) => n.id === "__end")!;
    const groupWidth = typeof group.style?.width === "number" ? group.style.width : 0;

    expect(start.position.x).toBeLessThan(group.position.x);
    expect(end.position.x).toBeGreaterThan(group.position.x + groupWidth);
  });

  it("gives the terminator nodes a terminator kind and lowercase label", () => {
    const { nodes } = buildFlowElements(singleCompositionGraph, undefined, {
      expandedStates: [],
      thisRunOnly: false,
    });

    const start = nodes.find((n) => n.id === "__start")!;
    const end = nodes.find((n) => n.id === "__end")!;
    expect(start.data).toEqual({ label: "start", kind: "terminator" });
    expect(end.data).toEqual({ label: "end", kind: "terminator" });
  });

  it("adds start/end in every mode, including this-run-only", () => {
    const { nodes, edges } = buildFlowElements(documentAnalysisGraph, undefined, {
      expandedStates: [],
      thisRunOnly: true,
    });
    expect(nodes.some((n) => n.id === "__start")).toBe(true);
    expect(nodes.some((n) => n.id === "__end")).toBe(true);
    expect(edges.some((e) => e.source === "__start" && e.target === "default")).toBe(true);
    expect(edges.some((e) => e.source === "done" && e.target === "__end")).toBe(true);
  });
});

describe("buildFlowElements — thisRunOnly", () => {
  const multiStateGraph: WorkflowGraph = {
    nodes: [
      { id: "default", label: "default", kind: "entry", entry: true, terminal: false },
      { id: "intake", label: "intake", kind: "entry", entry: false, terminal: false },
      { id: "resolve", label: "resolve", kind: "output", entry: false, terminal: true },
      { id: "escalate", label: "escalate", kind: "agent", entry: false, terminal: true },
    ],
    edges: [
      { from: "intake", to: "resolve", label: "classified" },
      { from: "intake", to: "escalate", label: "unclear" },
    ],
  };

  it("drops unvisited states (and their steps) but keeps the visited path", () => {
    const run = mkRun({
      Messages: [
        { role: "system", content: "", meta: { _workflow_state: { current_state: "intake" } } },
        {
          role: "tool",
          content: "",
          meta: { _workflow_state: { current_state: "resolve", previous_state: "intake", transition: "classified" } },
        },
      ],
    });

    const { nodes, edges } = buildFlowElements(multiStateGraph, run, {
      expandedStates: [],
      thisRunOnly: true,
    });

    const ids = nodes.map((n) => n.id).sort();
    expect(ids).toEqual(["__end", "__start", "default", "intake", "resolve"]);
    expect(nodes.some((n) => n.id === "escalate")).toBe(false);
    expect(edges.some((e) => e.source === "intake" && e.target === "escalate")).toBe(false);
    expect(edges.some((e) => e.source === "intake" && e.target === "resolve")).toBe(true);
    // The dropped "escalate" terminal state doesn't get a __end edge; the
    // surviving "resolve" terminal state does.
    expect(edges.some((e) => e.source === "escalate" && e.target === "__end")).toBe(false);
    expect(edges.some((e) => e.source === "resolve" && e.target === "__end")).toBe(true);
    expectFinitePositions(nodes);
  });

  it("is a no-op when the run carries no workflow-state overlay data", () => {
    const run = mkRun({ Messages: [{ role: "user", content: "hi" }] });
    const { nodes } = buildFlowElements(multiStateGraph, run, { expandedStates: [], thisRunOnly: true });
    const ids = nodes.map((n) => n.id).sort();
    expect(ids).toEqual(["__end", "__start", "default", "escalate", "intake", "resolve"]);
  });

  it("drops the orphaned __end terminator when the run's path never reaches the terminal state, but keeps __start", () => {
    // The run only ever reaches "analyzing" — "done" (the sole terminal
    // state) is unvisited and gets filtered out by thisRunOnly, so the
    // __end terminator that would target it has no surviving edge and must
    // not render as a floating, edgeless dot. "default" (the entry state)
    // is visited, so __start keeps its edge and survives.
    const run = mkRun({
      Messages: [
        {
          role: "system",
          content: "",
          meta: { _workflow_state: { current_state: "analyzing", previous_state: "default" } },
        },
      ],
    });

    const { nodes, edges } = buildFlowElements(documentAnalysisGraph, run, {
      expandedStates: [],
      thisRunOnly: true,
    });

    expect(nodes.some((n) => n.id === "__end")).toBe(false);
    expect(edges.some((e) => e.target === "__end")).toBe(false);

    expect(nodes.some((n) => n.id === "__start")).toBe(true);
    expect(edges.some((e) => e.source === "__start" && e.target === "default")).toBe(true);
  });

  it("drops an unvisited state's composition steps and group in expanded mode", () => {
    const graph: WorkflowGraph = {
      nodes: [
        { id: "default", label: "default", kind: "entry", entry: true, terminal: false },
        { id: "analyzing", label: "analyzing", kind: "agent", entry: false, terminal: false },
        { id: "other", label: "other", kind: "agent", entry: false, terminal: true },
        { id: "analyzing/step1", label: "step1", kind: "prompt", entry: false, terminal: false, parent: "analyzing" },
      ],
      edges: [
        { from: "default", to: "analyzing" },
        { from: "analyzing", to: "analyzing/step1" },
      ],
    };
    // Run's overlay visits only "other" (unrelated path never enters "analyzing").
    const run = mkRun({
      Messages: [{ role: "system", content: "", meta: { _workflow_state: { current_state: "other" } } }],
    });
    const { nodes } = buildFlowElements(graph, run, { expandedStates: ["analyzing"], thisRunOnly: true });
    expect(nodes.some((n) => n.id === "grp:analyzing")).toBe(false);
    expect(nodes.some((n) => n.id === "analyzing/step1")).toBe(false);
  });
});

describe("buildFlowElements — overlay passthrough", () => {
  it("carries dashed/gold/dim through onto edges", () => {
    const graph: WorkflowGraph = {
      nodes: [
        { id: "a", label: "a", kind: "entry", entry: true, terminal: false },
        { id: "b", label: "b", kind: "output", entry: false, terminal: true },
      ],
      edges: [{ from: "a", to: "b", label: "go", dashed: true }],
    };
    const { edges } = buildFlowElements(graph, undefined, { expandedStates: [], thisRunOnly: false });
    const e = edges.find((e) => e.source === "a" && e.target === "b")!;
    expect(e.data.label).toBe("go");
    expect(e.data.dashed).toBe(true);
  });
});
