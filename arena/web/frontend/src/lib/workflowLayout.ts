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
const DEFAULT_COL_GAP = 160;
const DEFAULT_ROW_GAP = 90;

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

  const byLayer = new Map<number, WorkflowGraphNode[]>();
  for (const n of graph.nodes) {
    const l = layers.get(n.id) ?? 0;
    const group = byLayer.get(l);
    if (group) group.push(n);
    else byLayer.set(l, [n]);
  }

  const nodes: GraphNode[] = [];
  let maxLayer = 0;
  let maxRows = 1;
  for (const [l, group] of byLayer) {
    maxLayer = Math.max(maxLayer, l);
    maxRows = Math.max(maxRows, group.length);
    const sorted = [...group].sort((a, b) => a.id.localeCompare(b.id));
    sorted.forEach((n, rowIndex) => {
      nodes.push({
        id: n.id,
        x: MARGIN + l * colGap,
        y: MARGIN + rowIndex * rowGap,
        kind: n.kind,
        label: n.label,
        dim: n.dim,
      });
    });
  }

  const edges: GraphEdge[] = graph.edges.map((e) => ({
    from: e.from,
    to: e.to,
    label: e.label,
    dashed: e.dashed,
    gold: e.gold,
    dim: e.dim,
  }));

  const width = opts.width ?? MARGIN * 2 + maxLayer * colGap;
  const height = MARGIN * 2 + (maxRows - 1) * rowGap;

  return { nodes, edges, width, height };
}
