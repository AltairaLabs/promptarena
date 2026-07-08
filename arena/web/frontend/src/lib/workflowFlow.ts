// workflowFlow.ts — pure selector that turns a backend WorkflowGraph, an
// optional run overlay, and the panel's toggle state into React-Flow-shaped
// elements. No React, no side effects; dagre layout is deterministic given
// the same (sorted) input, so this stays trivially unit-testable.

import dagre from "@dagrejs/dagre";
import type { RunResult, ActiveRun, WorkflowGraph, WorkflowGraphNode, WorkflowGraphEdge } from "@/types";
import { overlayWorkflowRun } from "./arenaView";

export interface FlowToggles {
  // expandedStates lists the composition-owning state ids currently drilled
  // into (per-state, not all-or-nothing) — a state expands into its group +
  // steps iff its id is a member; every other composition-owning state stays
  // a single node with the collapsed ⤵ badge.
  expandedStates: string[];
  thisRunOnly: boolean;
}

export interface FlowNodeData {
  label: string;
  // "terminator" is a frontend-only synthetic kind for the __start/__end
  // nodes buildFlowElements always adds — never a backend WorkflowGraphNode
  // kind.
  kind: WorkflowGraphNode["kind"] | "terminator";
  hasComposition?: boolean;
  dim?: boolean;
  gold?: boolean;
  isGroup?: boolean;
  // stateId recovers the owning state's raw id from its group node (id
  // `grp:<stateId>`) — WorkflowGraphView needs it to report the right id
  // back through onStateClick when a click lands on the expanded group.
  stateId?: string;
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

// Node footprints are sized to the glyph + label WorkflowNode.tsx actually
// renders (a ~15-18px glyph plus a ~13px mono label underneath it), not a
// generic box — dagre reserving big uniform cells for tiny glyphs is what
// produced the old sprawling, whitespace-heavy layout. Width scales with
// label length (~8.5px/char, matching the 13px mono label) so long labels
// (e.g. "extract_general") get room without over-spacing short ones; height
// is fixed (glyph + gap + one line of label).
const NODE_HEIGHT = 50;
const MIN_NODE_WIDTH = 56;
const LABEL_CHAR_WIDTH = 8.5;
const LABEL_PADDING = 24;
const GROUP_PADDING = 14;
const GROUP_HEADER = 20;
const DAGRE_NODESEP = 40;
const DAGRE_RANKSEP = 72;
const DAGRE_EDGESEP = 18;
// Gap kept between a terminator and the nearest group boundary once clamped
// outside it — same order of magnitude as DAGRE_RANKSEP so it reads as a
// normal rank gap rather than a cramped special case.
const TERMINATOR_GROUP_GAP = 40;

function nodeSize(label: string): { width: number; height: number } {
  return { width: Math.max(MIN_NODE_WIDTH, label.length * LABEL_CHAR_WIDTH + LABEL_PADDING), height: NODE_HEIGHT };
}

function groupId(stateId: string): string {
  return `grp:${stateId}`;
}

// Synthetic terminator node ids — always present, never real backend nodes.
const START_ID = "__start";
const END_ID = "__end";

function byId<T extends { id: string }>(a: T, b: T): number {
  return a.id.localeCompare(b.id);
}

function byFromTo(a: WorkflowGraphEdge, b: WorkflowGraphEdge): number {
  if (a.from !== b.from) return a.from.localeCompare(b.from);
  return a.to.localeCompare(b.to);
}

// runDagreLayout lays out `nodeIds` (each sized to its own label via
// `nodeSize`) using the edges between them and returns each node's TOP-LEFT
// position plus the footprint dagre laid it out with (React Flow's
// position convention; dagre itself returns centers). `labelById` supplies
// the label used to size each node — ids missing from it (shouldn't happen;
// callers populate it from the same node lists they pass here) fall back to
// the minimum-width box.
function runDagreLayout(
  nodeIds: string[],
  edges: { from: string; to: string }[],
  labelById: Map<string, string>,
): Map<string, { x: number; y: number; width: number; height: number }> {
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: "LR", nodesep: DAGRE_NODESEP, ranksep: DAGRE_RANKSEP, edgesep: DAGRE_EDGESEP });
  g.setDefaultEdgeLabel(() => ({}));
  for (const id of nodeIds) {
    g.setNode(id, nodeSize(labelById.get(id) ?? ""));
  }
  for (const e of edges) {
    if (g.hasNode(e.from) && g.hasNode(e.to)) {
      g.setEdge(e.from, e.to);
    }
  }
  dagre.layout(g);

  const positions = new Map<string, { x: number; y: number; width: number; height: number }>();
  for (const id of nodeIds) {
    const n = g.node(id);
    positions.set(id, { x: n.x - n.width / 2, y: n.y - n.height / 2, width: n.width, height: n.height });
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
  const expandedSet = new Set(toggles.expandedStates);

  const visitedStateIds = toggles.thisRunOnly
    ? new Set(stateNodes.filter((s) => !s.dim || s.id === "default").map((s) => s.id))
    : new Set(stateNodes.map((s) => s.id));

  const survivingStates = stateNodes.filter((s) => visitedStateIds.has(s.id));
  const survivingStateIds = new Set(survivingStates.map((s) => s.id));

  // A state only actually expands when it both owns a composition and is
  // named in expandedStates — a non-composition id in the array (shouldn't
  // happen, callers only ever add composition-owning ids) is inert.
  const expandedStateIds = new Set(
    survivingStates.filter((s) => hasCompositionStates.has(s.id) && expandedSet.has(s.id)).map((s) => s.id),
  );

  const survivingSteps = stepNodes.filter((s) => expandedStateIds.has(s.parent!));

  const sortedEdges = [...overlaid.edges].sort(byFromTo);

  const entryState = stateNodes.find((s) => s.entry);
  const terminalStates = stateNodes.filter((s) => s.terminal);

  return buildElements(
    survivingStates,
    survivingSteps,
    sortedEdges,
    survivingStateIds,
    hasCompositionStates,
    expandedStateIds,
    entryState,
    terminalStates,
  );
}

// buildElements is the single node/edge builder for every toggle
// combination: a composition-owning state renders as a group + its steps
// iff its id is in `expandedStateIds`, otherwise (like every
// non-composition state) it's a plain single node. Two synthetic
// terminators (`__start`/`__end`) are always added, wired to the graph's
// declared entry state and terminal state(s) so the panel always reads
// start -> ... -> end rather than a lone dot.
function buildElements(
  survivingStates: WorkflowGraphNode[],
  survivingSteps: WorkflowGraphNode[],
  sortedEdges: WorkflowGraphEdge[],
  survivingStateIds: Set<string>,
  hasCompositionStates: Set<string>,
  expandedStateIds: Set<string>,
  entryState: WorkflowGraphNode | undefined,
  terminalStates: WorkflowGraphNode[],
): FlowElements {
  const survivingStepIds = new Set(survivingSteps.map((s) => s.id));
  const survivingIds = new Set<string>([...survivingStateIds, ...survivingStepIds]);

  // Any edge endpoint that names an expanded composition-owning state is
  // remapped to that state's group id — the group visually replaces the
  // state node, so edges (including the synthetic terminator edges below)
  // must originate/terminate at the container.
  const remap = (id: string) => (expandedStateIds.has(id) ? groupId(id) : id);

  // Terminator edges only target states that actually survived the current
  // toggles (e.g. This-run-only can drop the declared entry/terminal state
  // from a path that never reached it).
  const terminatorEdges: { from: string; to: string }[] = [];
  if (entryState && survivingStateIds.has(entryState.id)) {
    terminatorEdges.push({ from: START_ID, to: entryState.id });
  }
  for (const t of terminalStates) {
    if (survivingStateIds.has(t.id)) terminatorEdges.push({ from: t.id, to: END_ID });
  }

  // Dagre lays out every real (non-group) node — states, steps, and the two
  // terminators alike — using the raw graph edges (including the
  // state->entry-step edge, whose "from" is the composition-owning state's
  // own id) plus the terminator edges. A state's own dagre position is only
  // used to seed the layout; it never becomes a rendered node once it's
  // expanded (the group replaces it).
  const dagreNodeIds = [
    START_ID,
    END_ID,
    ...survivingStates.map((s) => s.id),
    ...survivingSteps.map((s) => s.id),
  ].sort();
  const dagreEdges = [
    ...sortedEdges
      .filter((e) => survivingIds.has(e.from) && survivingIds.has(e.to))
      .map((e) => ({ from: e.from, to: e.to })),
    ...terminatorEdges,
  ];
  const labelById = new Map<string, string>([
    [START_ID, "start"],
    [END_ID, "end"],
    ...survivingStates.map((s): [string, string] => [s.id, s.label]),
    ...survivingSteps.map((s): [string, string] => [s.id, s.label]),
  ]);
  const positions = runDagreLayout(dagreNodeIds, dagreEdges, labelById);

  const nodes: FlowNode[] = [
    { id: START_ID, type: "workflowNode", position: positions.get(START_ID) ?? { x: 0, y: 0 }, data: { label: "start", kind: "terminator" } },
    { id: END_ID, type: "workflowNode", position: positions.get(END_ID) ?? { x: 0, y: 0 }, data: { label: "end", kind: "terminator" } },
  ];

  for (const s of survivingStates) {
    if (expandedStateIds.has(s.id)) continue; // becomes a group below
    nodes.push({
      id: s.id,
      type: "workflowNode",
      position: positions.get(s.id) ?? { x: 0, y: 0 },
      data: {
        label: s.label,
        kind: s.kind,
        hasComposition: hasCompositionStates.has(s.id) || undefined,
        dim: s.dim,
      },
    });
  }

  const stepsByParent = new Map<string, WorkflowGraphNode[]>();
  for (const step of survivingSteps) {
    const list = stepsByParent.get(step.parent!);
    if (list) list.push(step);
    else stepsByParent.set(step.parent!, [step]);
  }

  for (const s of survivingStates) {
    if (!expandedStateIds.has(s.id)) continue;
    const steps = stepsByParent.get(s.id) ?? [];
    const gid = groupId(s.id);

    if (steps.length === 0) {
      // Defensive: an expanded state with no surviving steps (shouldn't
      // happen — steps survive iff their parent state is expanded) falls
      // back to a plain node rather than an empty group.
      nodes.push({
        id: s.id,
        type: "workflowNode",
        position: positions.get(s.id) ?? { x: 0, y: 0 },
        data: { label: s.label, kind: s.kind, hasComposition: true, dim: s.dim },
      });
      continue;
    }

    const stepPositions = steps.map(
      (step) => positions.get(step.id) ?? { x: 0, y: 0, width: MIN_NODE_WIDTH, height: NODE_HEIGHT },
    );
    const minX = Math.min(...stepPositions.map((p) => p.x));
    const minY = Math.min(...stepPositions.map((p) => p.y));
    const maxX = Math.max(...stepPositions.map((p) => p.x + p.width));
    const maxY = Math.max(...stepPositions.map((p) => p.y + p.height));

    const groupX = minX - GROUP_PADDING;
    const groupY = minY - GROUP_PADDING - GROUP_HEADER;
    const width = maxX - minX + GROUP_PADDING * 2;
    const height = maxY - minY + GROUP_PADDING * 2 + GROUP_HEADER;

    nodes.push({
      id: gid,
      type: "group",
      position: { x: groupX, y: groupY },
      data: { label: s.label, kind: s.kind, isGroup: true, dim: s.dim, stateId: s.id },
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

  // Dagre lays the terminators out as flat siblings of the composition
  // state's own (invisible, once expanded) seed node — a state that's both
  // entry/terminal and expanded can land __end at the same rank as the
  // group's first step, inside the group's x-range (and symmetrically for
  // __start against a group it precedes). Once every group's geometry is
  // known, push __start left of the leftmost group and __end right of the
  // rightmost group so a terminator never overlaps a group box; y is left
  // as dagre computed it (already sensible — seeded from the real
  // entry/terminal state's row).
  const groupNodes = nodes.filter((n) => n.type === "group");
  if (groupNodes.length > 0) {
    const minGroupLeft = Math.min(...groupNodes.map((g) => g.position.x));
    const maxGroupRight = Math.max(
      ...groupNodes.map((g) => g.position.x + (typeof g.style?.width === "number" ? g.style.width : 0)),
    );
    const startNode = nodes.find((n) => n.id === START_ID);
    const endNode = nodes.find((n) => n.id === END_ID);
    const startWidth = positions.get(START_ID)?.width ?? MIN_NODE_WIDTH;
    if (startNode) {
      startNode.position.x = Math.min(startNode.position.x, minGroupLeft - TERMINATOR_GROUP_GAP - startWidth);
    }
    if (endNode) {
      endNode.position.x = Math.max(endNode.position.x, maxGroupRight + TERMINATOR_GROUP_GAP);
    }
  }

  const usedIds = new Set<string>();
  const edges: FlowEdge[] = sortedEdges
    .filter((e) => survivingIds.has(e.from) && survivingIds.has(e.to))
    .map((e) => {
      const source = remap(e.from);
      const target = remap(e.to);
      return { id: makeEdgeId(source, target, usedIds), source, target, data: toFlowEdgeData(e) };
    });

  for (const te of terminatorEdges) {
    const source = remap(te.from);
    const target = remap(te.to);
    edges.push({ id: makeEdgeId(source, target, usedIds), source, target, data: {} });
  }

  // An edge between a group and one of its OWN child steps (the remapped
  // state->entry-step edge, e.g. grp:analyzing -> analyzing/classify) is
  // redundant — the group container already shows it contains the step — and
  // renders as a confusing arrow looping back into the group (looking like the
  // entry step points at itself). Drop group<->own-child edges.
  const parentById = new Map(
    nodes.filter((n) => n.parentId).map((n) => [n.id, n.parentId as string]),
  );
  const routableEdges = edges.filter(
    (e) => parentById.get(e.target) !== e.source && parentById.get(e.source) !== e.target,
  );

  // A terminator with no surviving edge (e.g. thisRunOnly dropped the entry
  // or terminal state it connects to, or an in-progress run hasn't reached
  // the terminal state yet) would render as a floating, edgeless dot — drop
  // it rather than show an orphan.
  const startHasEdge = routableEdges.some((e) => e.source === START_ID);
  const endHasEdge = routableEdges.some((e) => e.target === END_ID);
  const survivingNodes = nodes.filter(
    (n) => (n.id !== START_ID || startHasEdge) && (n.id !== END_ID || endHasEdge),
  );

  return { nodes: survivingNodes, edges: routableEdges };
}
