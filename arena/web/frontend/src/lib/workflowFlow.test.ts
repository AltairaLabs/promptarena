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
  const toggles = { compositionsExpanded: false, thisRunOnly: false };

  it("keeps only state nodes, drops steps, and flags hasComposition on the owning state", () => {
    const { nodes, edges } = buildFlowElements(documentAnalysisGraph, undefined, toggles);

    const ids = nodes.map((n) => n.id).sort();
    expect(ids).toEqual(["analyzing", "default", "done"]);

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

    expectFinitePositions(nodes);
  });
});

describe("buildFlowElements — expanded", () => {
  const toggles = { compositionsExpanded: true, thisRunOnly: false };

  it("includes step nodes nested under a group node for the owning state", () => {
    const { nodes, edges } = buildFlowElements(documentAnalysisGraph, undefined, toggles);

    const group = nodes.find((n) => n.id === "grp:analyzing");
    expect(group).toBeDefined();
    expect(group!.type).toBe("group");
    expect(group!.data.isGroup).toBe(true);

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

    expectFinitePositions(nodes);
    // Step positions are relative to the group, so they should be smaller
    // in magnitude than the group's own absolute position in typical cases;
    // at minimum they must be finite (checked above) and the group must
    // have a sized style box.
    expect(typeof group!.style?.width).toBe("number");
    expect(typeof group!.style?.height).toBe("number");
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
      compositionsExpanded: false,
      thisRunOnly: true,
    });

    const ids = nodes.map((n) => n.id).sort();
    expect(ids).toEqual(["default", "intake", "resolve"]);
    expect(nodes.some((n) => n.id === "escalate")).toBe(false);
    expect(edges.some((e) => e.source === "intake" && e.target === "escalate")).toBe(false);
    expect(edges.some((e) => e.source === "intake" && e.target === "resolve")).toBe(true);
    expectFinitePositions(nodes);
  });

  it("is a no-op when the run carries no workflow-state overlay data", () => {
    const run = mkRun({ Messages: [{ role: "user", content: "hi" }] });
    const { nodes } = buildFlowElements(multiStateGraph, run, { compositionsExpanded: false, thisRunOnly: true });
    const ids = nodes.map((n) => n.id).sort();
    expect(ids).toEqual(["default", "escalate", "intake", "resolve"]);
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
    const { nodes } = buildFlowElements(graph, run, { compositionsExpanded: true, thisRunOnly: true });
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
    const { edges } = buildFlowElements(graph, undefined, { compositionsExpanded: false, thisRunOnly: false });
    const e = edges.find((e) => e.source === "a" && e.target === "b")!;
    expect(e.data.label).toBe("go");
    expect(e.data.dashed).toBe(true);
  });
});
