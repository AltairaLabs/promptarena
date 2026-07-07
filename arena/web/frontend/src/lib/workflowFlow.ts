// workflowFlow.ts — pure selector that turns a backend WorkflowGraph, an
// optional run overlay, and the panel's toggle state into React-Flow-shaped
// elements. No React, no side effects; dagre layout is deterministic given
// the same (sorted) input, so this stays trivially unit-testable.

import dagre from "@dagrejs/dagre";
import type { RunResult, ActiveRun, WorkflowGraph, WorkflowGraphNode, WorkflowGraphEdge } from "@/types";
import { overlayWorkflowRun } from "./arenaView";

export interface FlowToggles {
  compositionsExpanded: boolean;
  thisRunOnly: boolean;
}

export interface FlowNodeData {
  label: string;
  kind: WorkflowGraphNode["kind"];
  hasComposition?: boolean;
  dim?: boolean;
  gold?: boolean;
  isGroup?: boolean;
}

export interface FlowNode {
  id: string;
  type: string;
  position: { x: number; y: number };
  data: FlowNodeData;
  parentId?: string;
  extent?: "parent";
  style?: Record<string, number | string>;
}

export interface FlowEdgeData {
  label?: string;
  dashed?: boolean;
  gold?: boolean;
  dim?: boolean;
}

export interface FlowEdge {
  id: string;
  source: string;
  target: string;
  data: FlowEdgeData;
}

export interface FlowElements {
  nodes: FlowNode[];
  edges: FlowEdge[];
}

const NODE_WIDTH = 180;
const NODE_HEIGHT = 56;
const GROUP_PADDING = 32;
const GROUP_HEADER = 28;
const DAGRE_NODESEP = 48;
const DAGRE_RANKSEP = 96;

function groupId(stateId: string): string {
  return `grp:${stateId}`;
}

function byId<T extends { id: string }>(a: T, b: T): number {
  return a.id.localeCompare(b.id);
}

function byFromTo(a: WorkflowGraphEdge, b: WorkflowGraphEdge): number {
  if (a.from !== b.from) return a.from.localeCompare(b.from);
  return a.to.localeCompare(b.to);
}

// runDagreLayout lays out `nodeIds` (uniform-sized boxes) using the edges
// between them and returns each node's TOP-LEFT position (React Flow's
// convention; dagre itself returns centers).
function runDagreLayout(
  nodeIds: string[],
  edges: { from: string; to: string }[],
): Map<string, { x: number; y: number }> {
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: "LR", nodesep: DAGRE_NODESEP, ranksep: DAGRE_RANKSEP });
  g.setDefaultEdgeLabel(() => ({}));
  for (const id of nodeIds) {
    g.setNode(id, { width: NODE_WIDTH, height: NODE_HEIGHT });
  }
  for (const e of edges) {
    if (g.hasNode(e.from) && g.hasNode(e.to)) {
      g.setEdge(e.from, e.to);
    }
  }
  dagre.layout(g);

  const positions = new Map<string, { x: number; y: number }>();
  for (const id of nodeIds) {
    const n = g.node(id);
    positions.set(id, { x: n.x - NODE_WIDTH / 2, y: n.y - NODE_HEIGHT / 2 });
  }
  return positions;
}

// edgeId produces a stable, deduped id of the form "source->target", falling
// back to "source->target#n" when the same pair appears more than once.
function makeEdgeId(source: string, target: string, used: Set<string>): string {
  const base = `${source}->${target}`;
  if (!used.has(base)) {
    used.add(base);
    return base;
  }
  let n = 2;
  while (used.has(`${base}#${n}`)) n++;
  const id = `${base}#${n}`;
  used.add(id);
  return id;
}

function toFlowEdgeData(e: WorkflowGraphEdge): FlowEdgeData {
  return { label: e.label, dashed: e.dashed, gold: e.gold, dim: e.dim };
}

export function buildFlowElements(
  graph: WorkflowGraph,
  run: RunResult | ActiveRun | undefined,
  toggles: FlowToggles,
): FlowElements {
  const overlaid = overlayWorkflowRun(graph, run);

  const stateNodes = overlaid.nodes.filter((n) => !n.parent).sort(byId);
  const stepNodes = overlaid.nodes.filter((n) => n.parent).sort(byId);

  const hasCompositionStates = new Set(stepNodes.map((s) => s.parent!));

  const visitedStateIds = toggles.thisRunOnly
    ? new Set(stateNodes.filter((s) => !s.dim || s.id === "default").map((s) => s.id))
    : new Set(stateNodes.map((s) => s.id));

  const survivingStates = stateNodes.filter((s) => visitedStateIds.has(s.id));
  const survivingStateIds = new Set(survivingStates.map((s) => s.id));
  const survivingSteps = stepNodes.filter((s) => survivingStateIds.has(s.parent!));

  const sortedEdges = [...overlaid.edges].sort(byFromTo);

  if (!toggles.compositionsExpanded) {
    return buildCollapsed(survivingStates, sortedEdges, survivingStateIds, hasCompositionStates);
  }
  return buildExpanded(survivingStates, survivingSteps, sortedEdges, survivingStateIds, hasCompositionStates);
}

function buildCollapsed(
  survivingStates: WorkflowGraphNode[],
  sortedEdges: WorkflowGraphEdge[],
  survivingStateIds: Set<string>,
  hasCompositionStates: Set<string>,
): FlowElements {
  const nodeIds = survivingStates.map((s) => s.id);
  const stateEdges = sortedEdges.filter((e) => survivingStateIds.has(e.from) && survivingStateIds.has(e.to));
  const positions = runDagreLayout(
    nodeIds,
    stateEdges.map((e) => ({ from: e.from, to: e.to })),
  );

  const nodes: FlowNode[] = survivingStates.map((s) => ({
    id: s.id,
    type: "workflowNode",
    position: positions.get(s.id) ?? { x: 0, y: 0 },
    data: {
      label: s.label,
      kind: s.kind,
      hasComposition: hasCompositionStates.has(s.id) || undefined,
      dim: s.dim,
    },
  }));

  const usedIds = new Set<string>();
  const edges: FlowEdge[] = stateEdges.map((e) => ({
    id: makeEdgeId(e.from, e.to, usedIds),
    source: e.from,
    target: e.to,
    data: toFlowEdgeData(e),
  }));

  return { nodes, edges };
}

function buildExpanded(
  survivingStates: WorkflowGraphNode[],
  survivingSteps: WorkflowGraphNode[],
  sortedEdges: WorkflowGraphEdge[],
  survivingStateIds: Set<string>,
  hasCompositionStates: Set<string>,
): FlowElements {
  const survivingStepIds = new Set(survivingSteps.map((s) => s.id));
  const survivingIds = new Set<string>([...survivingStateIds, ...survivingStepIds]);

  // Dagre lays out every real (non-group) node — states and steps alike —
  // using the raw graph edges (including the state->entry-step edge, whose
  // "from" is the composition-owning state's own id). The state's own
  // dagre position is only used to seed the layout; it never becomes a
  // rendered node once it owns a composition (the group replaces it).
  const dagreNodeIds = [...survivingStates.map((s) => s.id), ...survivingSteps.map((s) => s.id)].sort();
  const dagreEdges = sortedEdges
    .filter((e) => survivingIds.has(e.from) && survivingIds.has(e.to))
    .map((e) => ({ from: e.from, to: e.to }));
  const positions = runDagreLayout(dagreNodeIds, dagreEdges);

  const nodes: FlowNode[] = [];

  for (const s of survivingStates) {
    if (hasCompositionStates.has(s.id)) continue; // becomes a group below
    nodes.push({
      id: s.id,
      type: "workflowNode",
      position: positions.get(s.id) ?? { x: 0, y: 0 },
      data: { label: s.label, kind: s.kind, dim: s.dim },
    });
  }

  const stepsByParent = new Map<string, WorkflowGraphNode[]>();
  for (const step of survivingSteps) {
    const list = stepsByParent.get(step.parent!);
    if (list) list.push(step);
    else stepsByParent.set(step.parent!, [step]);
  }

  for (const s of survivingStates) {
    if (!hasCompositionStates.has(s.id)) continue;
    const steps = stepsByParent.get(s.id) ?? [];
    const gid = groupId(s.id);

    if (steps.length === 0) {
      // Defensive: a composition-owning state with no surviving steps
      // (shouldn't happen — steps survive iff their parent state does)
      // falls back to a plain node rather than an empty group.
      nodes.push({
        id: s.id,
        type: "workflowNode",
        position: positions.get(s.id) ?? { x: 0, y: 0 },
        data: { label: s.label, kind: s.kind, dim: s.dim },
      });
      continue;
    }

    const stepPositions = steps.map((step) => positions.get(step.id) ?? { x: 0, y: 0 });
    const minX = Math.min(...stepPositions.map((p) => p.x));
    const minY = Math.min(...stepPositions.map((p) => p.y));
    const maxX = Math.max(...stepPositions.map((p) => p.x + NODE_WIDTH));
    const maxY = Math.max(...stepPositions.map((p) => p.y + NODE_HEIGHT));

    const groupX = minX - GROUP_PADDING;
    const groupY = minY - GROUP_PADDING - GROUP_HEADER;
    const width = maxX - minX + GROUP_PADDING * 2;
    const height = maxY - minY + GROUP_PADDING * 2 + GROUP_HEADER;

    nodes.push({
      id: gid,
      type: "group",
      position: { x: groupX, y: groupY },
      data: { label: s.label, kind: s.kind, isGroup: true, dim: s.dim },
      style: { width, height },
    });

    steps.sort(byId).forEach((step) => {
      const abs = positions.get(step.id) ?? { x: 0, y: 0 };
      nodes.push({
        id: step.id,
        type: "workflowNode",
        position: { x: abs.x - groupX, y: abs.y - groupY },
        data: { label: step.label, kind: step.kind, dim: step.dim },
        parentId: gid,
        extent: "parent",
      });
    });
  }

  // Any edge endpoint that names a composition-owning surviving state is
  // remapped to that state's group id — the group visually replaces the
  // state node, so edges must originate/terminate at the container.
  const remap = (id: string) => (hasCompositionStates.has(id) && survivingStateIds.has(id) ? groupId(id) : id);

  const usedIds = new Set<string>();
  const edges: FlowEdge[] = sortedEdges
    .filter((e) => survivingIds.has(e.from) && survivingIds.has(e.to))
    .map((e) => {
      const source = remap(e.from);
      const target = remap(e.to);
      return { id: makeEdgeId(source, target, usedIds), source, target, data: toFlowEdgeData(e) };
    });

  return { nodes, edges };
}
