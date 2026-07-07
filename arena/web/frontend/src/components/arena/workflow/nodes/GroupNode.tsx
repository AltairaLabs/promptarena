import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { FlowNodeData } from "@/lib/workflowFlow";

// GroupNode — the labeled container a composition-owning state collapses
// into when the "expand compositions" toggle is on: its step nodes render
// as children positioned inside this box (React Flow's parentId/extent).
// It carries left/right Handles because edges into/out of the owning state
// get remapped onto the group id (see workflowFlow.ts's `remap`).
export function GroupNode({ data }: NodeProps & { data: FlowNodeData }) {
  return (
    <div
      style={{
        position: "relative",
        width: "100%",
        height: "100%",
        border: "1px solid var(--hairline)",
        // `--surface` has no direct definition in atlas-tokens.css (same
        // token ConstellationGraph's label halo referenced) — fall back to
        // the semantic card surface alias so this always resolves.
        background: "color-mix(in srgb, var(--surface, var(--surface-1)) 60%, transparent)",
        borderRadius: "var(--radius-lg)",
        opacity: data.dim ? 0.3 : 1,
      }}
    >
      <Handle type="target" position={Position.Left} style={{ opacity: 0.6 }} />
      <span
        style={{
          position: "absolute",
          top: 6,
          left: 10,
          font: "600 10px var(--font-mono)",
          textTransform: "uppercase",
          letterSpacing: "0.08em",
          color: "var(--star-900)",
        }}
      >
        {data.label}
      </span>
      <Handle type="source" position={Position.Right} style={{ opacity: 0.6 }} />
    </div>
  );
}
