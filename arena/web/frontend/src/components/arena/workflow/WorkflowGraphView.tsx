import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ReactFlow, Controls, MarkerType, useReactFlow, type ColorMode, type Edge, type Node } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import "./workflow.css";
import type { FlowElements, FlowNodeData } from "@/lib/workflowFlow";
import { WorkflowNode } from "./nodes/WorkflowNode";
import { GroupNode } from "./nodes/GroupNode";

const nodeTypes = { workflowNode: WorkflowNode, group: GroupNode };

export interface WorkflowGraphViewProps {
  elements: FlowElements;
  theme?: "light" | "dark";
  // onStateClick fires with the composition-owning state's raw id, whether
  // the click landed on its collapsed single node (data.hasComposition) or
  // its expanded group (mapped back via data.stateId) — callers don't need
  // to know which shape it's currently rendered as.
  onStateClick?: (stateId: string) => void;
}

// WorkflowGraphView — the React Flow render of a workflow topology built by
// `buildFlowElements`, styled as thin starlight wayfinding lines between
// glyph nodes (the Atlas star-chart look). Small open arrowheads carry
// direction, dashed strokes mark "else" branches, gold marks the visited
// path, and reduced opacity marks anything the current toggles have
// dimmed. Node/edge styling reads Atlas tokens so it reskins for free
// across the light/dark theme flip.
// `FlowNode`/`FlowEdge` (from workflowFlow.ts) are plain interfaces without
// index signatures, so they don't structurally satisfy React Flow's
// `Node`/`Edge` generic constraint (`Record<string, unknown>` data). They're
// otherwise shape-compatible with the defaults — go through `unknown` rather
// than duplicating the selector's types as index-signature-friendly aliases.
function toRfNodes(nodes: FlowElements["nodes"]): Node[] {
  return nodes as unknown as Node[];
}

// FitOnChange — rendered as a child of <ReactFlow>, which (per React Flow
// v12) wraps its children in an implicit ReactFlowProvider whenever the app
// doesn't supply its own, so useReactFlow() works here without
// WorkflowGraphView needing a <ReactFlowProvider> of its own. Re-frames the
// viewport whenever `signal` changes (expand/collapse, this-run-only, a
// different graph) — the `<ReactFlow fitView>` prop alone only fits once, on
// mount, so toggling a state after that leaves the panel stuck at the old
// framing until the user zooms out manually.
function FitOnChange({ signal }: { signal: string }) {
  const { fitView } = useReactFlow();
  const isFirstRun = useRef(true);

  useEffect(() => {
    // The initial mount frame is already handled by <ReactFlow fitView>;
    // re-running here would just duplicate that first fit.
    if (isFirstRun.current) {
      isFirstRun.current = false;
      return;
    }
    // Deferred a tick so newly (un)mounted nodes have been measured (React
    // Flow sizes nodes via ResizeObserver, which delivers asynchronously)
    // before fitView reads their dimensions.
    const timer = setTimeout(() => fitView({ padding: 0.22, maxZoom: 1, duration: 300 }), 0);
    return () => clearTimeout(timer);
  }, [signal, fitView]);

  return null;
}

export function WorkflowGraphView({ elements, theme, onStateClick }: WorkflowGraphViewProps) {
  // handleNodeClick — a click on a collapsed composition-owning state
  // (data.hasComposition) reports its own id; a click on that state's
  // expanded group (data.isGroup) reports the owning state's id back via
  // data.stateId instead of the group's "grp:<id>" id, so the caller's
  // expandedStates toggle logic never has to know about the group prefix.
  // Non-composition nodes and the start/end terminators are inert.
  const handleNodeClick = useCallback(
    (_event: unknown, node: Node) => {
      if (!onStateClick) return;
      const data = node.data as unknown as FlowNodeData;
      if (data.isGroup) {
        if (data.stateId) onStateClick(data.stateId);
        return;
      }
      if (data.hasComposition) onStateClick(node.id);
    },
    [onStateClick],
  );

  // A stable signal that changes exactly when the rendered node set changes
  // shape (expand/collapse, this-run-only, a different graph) — sorted node
  // ids joined, so re-ordering within the same set (which doesn't happen
  // today, but would be harmless) doesn't spuriously re-trigger a fit.
  const fitSignal = useMemo(
    () =>
      elements.nodes
        .map((n) => n.id)
        .sort()
        .join(","),
    [elements.nodes],
  );

  // Hovering a node lights up its immediate forward links — the edges to its
  // direct next node(s) — so you can see "where this step goes next" without
  // lighting the whole downstream path. `forwardEdgeIds` is the set of those
  // edge ids (or null when nothing is hovered / the node has no successors).
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const forwardEdgeIds = useMemo<Set<string> | null>(() => {
    if (!hoveredId) return null;
    const hit = new Set<string>();
    for (const e of elements.edges) {
      if (e.source === hoveredId) hit.add(e.id);
    }
    return hit.size > 0 ? hit : null;
  }, [hoveredId, elements.edges]);

  const rfEdges = useMemo<Edge[]>(
    () =>
      elements.edges.map((e): Edge => {
        const stroke = e.data.gold ? "var(--gold-500)" : "var(--starlight-500)";
        // On hover, the forward path stays full-strength, thicker, and animated
        // while the rest fades back only modestly — the path stands out without
        // recoloring or washing the rest out.
        const onPath = forwardEdgeIds?.has(e.id) ?? false;
        const hoverActive = forwardEdgeIds !== null;
        const opacity = e.data.dim ? 0.35 : hoverActive ? (onPath ? 1 : 0.28) : 0.55;
        return {
          id: e.id,
          source: e.source,
          target: e.target,
          // Default bezier — the curved wayfinding lines; the roomier dagre
          // separation keeps them from overlapping without going orthogonal.
          animated: onPath,
          data: e.data as unknown as Record<string, unknown>,
          // A small, open arrow (not the chunky filled ArrowClosed) — a
          // wayfinding tick, not a box-diagram arrowhead.
          markerEnd: { type: MarkerType.Arrow, width: 14, height: 14, color: stroke },
          label: e.data.label,
          style: {
            stroke,
            strokeWidth: onPath ? 2.6 : 1.3,
            strokeDasharray: e.data.dashed ? "4 4" : undefined,
            opacity,
          },
          labelStyle: { fill: "var(--star-500)", fontFamily: "var(--font-mono)", fontSize: 12 },
          // Label background reads over the night sky (no more "--surface"
          // fallback gap — the canvas var is always defined).
          labelBgStyle: { fill: "var(--c-canvas)" },
        };
      }),
    [elements.edges, forwardEdgeIds],
  );

  return (
    <ReactFlow
      nodes={toRfNodes(elements.nodes)}
      edges={rfEdges}
      nodeTypes={nodeTypes}
      colorMode={(theme ?? "dark") as ColorMode}
      fitView
      // Cap the auto-fit at natural scale so a small graph settles at its
      // designed glyph size instead of blowing up to fill the panel; leave
      // headroom for manual zoom in either direction.
      fitViewOptions={{ padding: 0.22, maxZoom: 1 }}
      minZoom={0.2}
      maxZoom={2.5}
      proOptions={{ hideAttribution: true }}
      nodesDraggable={false}
      nodesConnectable={false}
      elementsSelectable={false}
      onNodeClick={handleNodeClick}
      onNodeMouseEnter={(_e, node) => setHoveredId(node.id)}
      onNodeMouseLeave={() => setHoveredId(null)}
    >
      {/* No <Background/> — the night sky comes from workflow.css's atmosphere
          on the .react-flow pane itself, not React Flow's dotted grid. No
          <MiniMap/> either — the panel is small enough that the nav box just
          eats space without earning it. */}
      <FitOnChange signal={fitSignal} />
      <Controls showInteractive={false} />
    </ReactFlow>
  );
}
