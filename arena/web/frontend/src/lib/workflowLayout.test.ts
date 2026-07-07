import { describe, it, expect } from "vitest";
import { layoutWorkflow } from "./workflowLayout";
import type { WorkflowGraph } from "@/types";

const g: WorkflowGraph = {
  nodes: [
    { id: "intake", label: "intake", kind: "entry", entry: true, terminal: false },
    { id: "resolve", label: "resolve", kind: "output", entry: false, terminal: true },
  ],
  edges: [{ from: "intake", to: "resolve", label: "classified" }],
};

describe("layoutWorkflow", () => {
  it("puts entry left of its successor and assigns finite coords", () => {
    const out = layoutWorkflow(g);
    const intake = out.nodes.find((n) => n.id === "intake")!;
    const resolve = out.nodes.find((n) => n.id === "resolve")!;
    expect(intake.x).toBeLessThan(resolve.x);
    for (const n of out.nodes) {
      expect(Number.isFinite(n.x)).toBe(true);
      expect(Number.isFinite(n.y)).toBe(true);
    }
    expect(out.width).toBeGreaterThan(0);
  });

  it("is cycle-safe (a back-edge doesn't hang or NaN)", () => {
    const cyc: WorkflowGraph = { nodes: g.nodes, edges: [...g.edges, { from: "resolve", to: "intake", label: "reopen" }] };
    const out = layoutWorkflow(cyc);
    for (const n of out.nodes) expect(Number.isFinite(n.x)).toBe(true);
  });

  it("places an unreachable node at layer 0", () => {
    const withOrphan: WorkflowGraph = {
      nodes: [...g.nodes, { id: "orphan", label: "orphan", kind: "tool", entry: false, terminal: false }],
      edges: g.edges,
    };
    const out = layoutWorkflow(withOrphan);
    const intake = out.nodes.find((n) => n.id === "intake")!;
    const orphan = out.nodes.find((n) => n.id === "orphan")!;
    expect(orphan.x).toBe(intake.x);
  });

  it("carries through kind/label and pass-through edge fields", () => {
    const out = layoutWorkflow({
      nodes: g.nodes,
      edges: [{ from: "intake", to: "resolve", label: "classified", dashed: true }],
    });
    const resolve = out.nodes.find((n) => n.id === "resolve")!;
    expect(resolve.kind).toBe("output");
    expect(resolve.label).toBe("resolve");
    const edge = out.edges[0];
    expect(edge.label).toBe("classified");
    expect(edge.dashed).toBe(true);
  });

  it("is pure and sizes width/height to content", () => {
    const before = JSON.stringify(g);
    const out = layoutWorkflow(g);
    expect(JSON.stringify(g)).toBe(before);
    expect(out.height).toBeGreaterThan(0);
  });

  it("bows a skip-layer edge into a curve that clears the intermediate node", () => {
    const wf = {
      nodes: [
        { id: "intake", label: "intake", kind: "entry" as const, entry: true, terminal: false },
        { id: "specialist", label: "specialist", kind: "agent" as const, entry: false, terminal: false },
        { id: "closed", label: "closed", kind: "output" as const, entry: false, terminal: true },
      ],
      edges: [
        { from: "intake", to: "specialist" },
        { from: "specialist", to: "closed" },
        { from: "intake", to: "closed", label: "Resolve" }, // spans two layers
      ],
    };
    const out = layoutWorkflow(wf);
    const adj = out.edges.find((e) => e.from === "intake" && e.to === "specialist")!;
    const skip = out.edges.find((e) => e.from === "intake" && e.to === "closed")!;
    expect(adj.curve).toBeUndefined();
    expect(skip.curve).toBeDefined();
    const intake = out.nodes.find((n) => n.id === "intake")!;
    const closed = out.nodes.find((n) => n.id === "closed")!;
    // control point is midway in x and bowed above the shared row in y
    expect(skip.curve!.cx).toBeCloseTo((intake.x + closed.x) / 2);
    expect(skip.curve!.cy).toBeLessThan(intake.y);
  });
});
