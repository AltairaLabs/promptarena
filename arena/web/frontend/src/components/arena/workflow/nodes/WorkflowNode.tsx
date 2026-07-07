import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { FlowNodeData } from "@/lib/workflowFlow";

// KIND_STYLE — the Atlas star-chart counterpart to ConstellationGraph's KIND
// map: every state/step is a small glyph (a filled dot, or a diamond for
// branch) with a translucent halo, not a box. entry/output are the gold
// "star" nodes that twinkle; agent is starlight, prompt is nebula violet,
// tool is ion cyan, branch is a gold diamond. Halo values are the same
// rgba()s ConstellationGraph's KIND map used — kept as raw color so the
// halo reads as a soft ring regardless of theme.
const KIND_STYLE: Record<
  FlowNodeData["kind"],
  { color: string; halo: string; diamond?: boolean; large?: boolean; glow?: boolean }
> = {
  entry: {
    color: "var(--gold-500)",
    halo: "rgba(227,179,65,0.16)",
    large: true,
    glow: true,
  },
  output: {
    color: "var(--gold-500)",
    halo: "rgba(227,179,65,0.16)",
    large: true,
    glow: true,
  },
  agent: {
    color: "var(--node-agent)",
    halo: "rgba(147,197,253,0.14)",
  },
  prompt: {
    color: "var(--node-prompt)",
    halo: "rgba(196,181,253,0.16)",
  },
  tool: {
    color: "var(--node-tool)",
    halo: "rgba(103,232,249,0.16)",
  },
  branch: {
    color: "var(--node-branch)",
    halo: "rgba(227,179,65,0.18)",
    diamond: true,
  },
};

// WorkflowNode — the glyph-styled React Flow custom node for both states
// (collapsed view) and steps (expanded, inside a GroupNode). The node's own
// container is transparent and exactly the size of the glyph itself — a
// small dot/diamond with a halo — so the (hidden) left/right Handles React
// Flow anchors edges to sit at the glyph's center, not a box corner. Shows a
// small "⤵" badge when the state owns a composition, and dims to a faint
// glimmer when `data.dim` (not visited this run / not reachable under the
// current toggles).
export function WorkflowNode({ data }: NodeProps & { data: FlowNodeData }) {
  const style = KIND_STYLE[data.kind] ?? KIND_STYLE.agent;
  const glyphClass = [
    "atlas-node__glyph",
    style.diamond ? "atlas-node__glyph--diamond" : "",
    style.glow ? "atlas-node__glyph--glow" : "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div className={`atlas-node${style.large ? " atlas-node--large" : ""}`} style={{ opacity: data.dim ? 0.35 : 1 }}>
      <Handle type="target" position={Position.Left} isConnectable={false} />
      <span
        className={glyphClass}
        style={{
          background: style.color,
          boxShadow: style.glow
            ? `0 0 0 6px ${style.halo}, 0 0 14px rgba(227,179,65,0.55)`
            : `0 0 0 6px ${style.halo}`,
        }}
      />
      {data.hasComposition && (
        <span aria-label="has composition" className="atlas-node__badge">
          ⤵
        </span>
      )}
      <div className="atlas-node__label">{data.label}</div>
      <Handle type="source" position={Position.Right} isConnectable={false} />
    </div>
  );
}
