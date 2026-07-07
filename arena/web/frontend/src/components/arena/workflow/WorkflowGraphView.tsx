import { useMemo } from "react";
import { ReactFlow, Background, Controls, MiniMap, MarkerType, type ColorMode, type Edge, type Node } from "@xyflow/react";
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
// `buildFlowElements`. Arrowheads carry direction, dashed strokes mark
// "else" branches, gold marks the visited path, and reduced opacity marks
// anything the current toggles have dimmed. Node/edge styling reads Atlas
// tokens so it reskins for free across the light/dark theme flip.
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
          markerEnd: { type: MarkerType.ArrowClosed, color: stroke },
          label: e.data.label,
          style: {
            stroke,
            strokeWidth: 1.5,
            strokeDasharray: e.data.dashed ? "5 4" : undefined,
            opacity: e.data.dim ? 0.3 : 1,
          },
          labelStyle: { fill: "var(--star-500)", fontFamily: "var(--font-mono)", fontSize: 10 },
          // `--surface` isn't defined in atlas-tokens.css (a pre-existing
          // gap ConstellationGraph's own label halo had); fall back to the
          // semantic card surface alias so the label backdrop always paints.
          labelBgStyle: { fill: "var(--surface, var(--surface-1))" },
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
      <Background />
      <Controls showInteractive={false} />
      <MiniMap pannable zoomable />
    </ReactFlow>
  );
}
