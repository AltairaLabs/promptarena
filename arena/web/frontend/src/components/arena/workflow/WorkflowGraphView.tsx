import { useMemo } from "react";
import { ReactFlow, Controls, MiniMap, MarkerType, type ColorMode, type Edge, type Node } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import "./workflow.css";
import type { FlowElements } from "@/lib/workflowFlow";
import { WorkflowNode } from "./nodes/WorkflowNode";
import { GroupNode } from "./nodes/GroupNode";

const nodeTypes = { workflowNode: WorkflowNode, group: GroupNode };

export interface WorkflowGraphViewProps {
  elements: FlowElements;
  theme?: "light" | "dark";
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

export function WorkflowGraphView({ elements, theme }: WorkflowGraphViewProps) {
  const rfEdges = useMemo<Edge[]>(
    () =>
      elements.edges.map((e): Edge => {
        const stroke = e.data.gold ? "var(--gold-500)" : "var(--starlight-500)";
        return {
          id: e.id,
          source: e.source,
          target: e.target,
          data: e.data as unknown as Record<string, unknown>,
          // A small, open arrow (not the chunky filled ArrowClosed) — a
          // wayfinding tick, not a box-diagram arrowhead.
          markerEnd: { type: MarkerType.Arrow, width: 14, height: 14, color: stroke },
          label: e.data.label,
          style: {
            stroke,
            strokeWidth: 1.3,
            strokeDasharray: e.data.dashed ? "4 4" : undefined,
            opacity: e.data.dim ? 0.35 : 0.55,
          },
          labelStyle: { fill: "var(--star-500)", fontFamily: "var(--font-mono)", fontSize: 10 },
          // Label background reads over the night sky (no more "--surface"
          // fallback gap — the canvas var is always defined).
          labelBgStyle: { fill: "var(--c-canvas)" },
        };
      }),
    [elements.edges],
  );

  return (
    <ReactFlow
      nodes={toRfNodes(elements.nodes)}
      edges={rfEdges}
      nodeTypes={nodeTypes}
      colorMode={(theme ?? "dark") as ColorMode}
      fitView
      proOptions={{ hideAttribution: true }}
      nodesDraggable={false}
      nodesConnectable={false}
      elementsSelectable={false}
    >
      {/* No <Background/> — the night sky comes from workflow.css's atmosphere
          on the .react-flow pane itself, not React Flow's dotted grid. */}
      <Controls showInteractive={false} />
      <MiniMap pannable zoomable />
    </ReactFlow>
  );
}
