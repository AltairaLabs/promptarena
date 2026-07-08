import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { FlowNodeData } from "@/lib/workflowFlow";

// GroupNode — the labeled container a composition-owning state collapses
// into when the "expand compositions" toggle is on: its step nodes render
// as children positioned inside this box (React Flow's parentId/extent). On
// the star chart this reads as a faint charted region, not an opaque box —
// a hairline outline over a barely-there wash of the canvas color. It
// carries left/right Handles (hidden, like every node's) because edges
// into/out of the owning state get remapped onto the group id (see
// workflowFlow.ts's `remap`).
export function GroupNode({ data }: NodeProps & { data: FlowNodeData }) {
  return (
    <div
      style={{
        position: "relative",
        width: "100%",
        height: "100%",
        border: "1px solid var(--hairline)",
        background: "color-mix(in srgb, var(--c-canvas) 40%, transparent)",
        borderRadius: "var(--radius-lg)",
        opacity: data.dim ? 0.35 : 1,
        // A composition's group is always click-to-collapse (see
        // WorkflowGraphView's onNodeClick / data.stateId), so the pointer
        // cursor is unconditional here.
        cursor: "pointer",
      }}
    >
      <Handle type="target" position={Position.Left} isConnectable={false} />
      <span
        style={{
          position: "absolute",
          top: 6,
          left: 10,
          font: "600 10px var(--font-mono)",
          textTransform: "uppercase",
          letterSpacing: "0.12em",
          color: "var(--star-900)",
        }}
      >
        {data.label}
      </span>
      <Handle type="source" position={Position.Right} isConnectable={false} />
    </div>
  );
}
