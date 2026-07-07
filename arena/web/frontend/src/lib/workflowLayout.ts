// workflowLayout.ts — pure layered-DAG layout for the workflow topology
// panel. Turns a backend WorkflowGraph (no coordinates) into the
// ConstellationGraph GraphNode/GraphEdge shapes it needs to render.

import type { WorkflowGraph, WorkflowGraphNode } from "@/types";
import type { GraphNode, GraphEdge } from "@/components/atlas/types";

export interface LayoutWorkflowOptions {
  width?: number;
  colGap?: number;
  rowGap?: number;
}

export interface WorkflowLayoutResult {
  nodes: GraphNode[];
  edges: GraphEdge[];
  width: number;
  height: number;
}

const MARGIN = 48;
const DEFAULT_COL_GAP = 190;
const DEFAULT_ROW_GAP = 110;

// pickEntry chooses the layout root: the node explicitly flagged `entry`,
// else the node of kind "entry", else (degenerate graphs) the first node.
function pickEntry(nodes: WorkflowGraphNode[]): WorkflowGraphNode | undefined {
  return nodes.find((n) => n.entry) ?? nodes.find((n) => n.kind === "entry") ?? nodes[0];
}

// computeLayers assigns each node its longest-path distance (in edges) from
// the entry node via a memoized DFS. Nodes currently on the DFS stack are
// skipped on re-entry so a back-edge in a cyclic graph can't recurse
// forever; nodes never reached from entry default to layer 0.
function computeLayers(graph: WorkflowGraph, entryId: string): Map<string, number> {
  const adjacency = new Map<string, string[]>();
  for (const e of graph.edges) {
    const list = adjacency.get(e.from);
    if (list) list.push(e.to);
    else adjacency.set(e.from, [e.to]);
  }

  const layer = new Map<string, number>();
  const onStack = new Set<string>();

  function visit(id: string, depth: number): void {
    if (onStack.has(id)) return; // back-edge guard: cut the cycle here
    const existing = layer.get(id);
    if (existing !== undefined && existing >= depth) return; // already as deep or deeper

    layer.set(id, depth);
    onStack.add(id);
    for (const next of adjacency.get(id) ?? []) {
      visit(next, depth + 1);
    }
    onStack.delete(id);
  }

  visit(entryId, 0);

  for (const n of graph.nodes) {
    if (!layer.has(n.id)) layer.set(n.id, 0);
  }
  return layer;
}

export function layoutWorkflow(graph: WorkflowGraph, opts: LayoutWorkflowOptions = {}): WorkflowLayoutResult {
  const colGap = opts.colGap ?? DEFAULT_COL_GAP;
  const rowGap = opts.rowGap ?? DEFAULT_ROW_GAP;

  if (graph.nodes.length === 0) {
    return { nodes: [], edges: [], width: opts.width ?? MARGIN * 2, height: MARGIN * 2 };
  }

  const entry = pickEntry(graph.nodes)!;
  const layers = computeLayers(graph, entry.id);
  const layerOf = (id: string) => layers.get(id) ?? 0;

  // A skip-layer edge (spanning more than one column) would be drawn straight
  // THROUGH the intermediate node(s) it passes over. Bow it into an arc so it
  // routes around them; the bow grows with the span. Reserve top headroom
  // (maxBow) so the highest arc isn't clipped by the viewBox.
  const bowOf = (span: number) => (span > 1 ? 30 + 26 * (span - 1) : 0);
  let maxBow = 0;
  for (const e of graph.edges) {
    maxBow = Math.max(maxBow, bowOf(Math.abs(layerOf(e.to) - layerOf(e.from))));
  }
  const topMargin = MARGIN + maxBow;

  const byLayer = new Map<number, WorkflowGraphNode[]>();
  for (const n of graph.nodes) {
    const group = byLayer.get(layerOf(n.id));
    if (group) group.push(n);
    else byLayer.set(layerOf(n.id), [n]);
  }

  const nodes: GraphNode[] = [];
  const posById = new Map<string, { x: number; y: number }>();
  let maxLayer = 0;
  let maxRows = 1;
  for (const [l, group] of byLayer) {
    maxLayer = Math.max(maxLayer, l);
    maxRows = Math.max(maxRows, group.length);
    const sorted = [...group].sort((a, b) => a.id.localeCompare(b.id));
    sorted.forEach((n, rowIndex) => {
      const x = MARGIN + l * colGap;
      const y = topMargin + rowIndex * rowGap;
      posById.set(n.id, { x, y });
      nodes.push({ id: n.id, x, y, kind: n.kind, label: n.label, dim: n.dim });
    });
  }

  const edges: GraphEdge[] = graph.edges.map((e) => {
    const bow = bowOf(Math.abs(layerOf(e.to) - layerOf(e.from)));
    let curve: { cx: number; cy: number } | undefined;
    if (bow > 0) {
      const a = posById.get(e.from);
      const b = posById.get(e.to);
      if (a && b) curve = { cx: (a.x + b.x) / 2, cy: (a.y + b.y) / 2 - bow };
    }
    return { from: e.from, to: e.to, label: e.label, dashed: e.dashed, gold: e.gold, dim: e.dim, curve };
  });

  const width = opts.width ?? MARGIN * 2 + maxLayer * colGap;
  const height = topMargin + (maxRows - 1) * rowGap + MARGIN;

  return { nodes, edges, width, height };
}
