import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { FlowNodeData } from "@/lib/workflowFlow";
import type { WorkflowGraphNode } from "@/types";

// KIND_STYLE — the Atlas star-chart counterpart to ConstellationGraph's KIND
// map: every state/step is a small glyph (a filled dot, or a diamond for
// branch) with a translucent halo, not a box. entry/output are the gold
// "star" nodes that twinkle; agent is starlight, prompt is nebula violet,
// tool is ion cyan, branch is a gold diamond. Halo values are the same
// rgba()s ConstellationGraph's KIND map used — kept as raw color so the
// halo reads as a soft ring regardless of theme. Keyed on the backend's kind
// union only — FlowNodeData["kind"] also carries the frontend-only
// "terminator" kind, which the early return below handles separately (a
// hollow ring, not a glyph from this map).
const KIND_STYLE: Record<
  WorkflowGraphNode["kind"],
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

// describeNode returns the plain-language tooltip/aria-label explaining what
// a glyph means — surfaced via the node's title attribute (native hover
// tooltip) and aria-label (screen readers) so the star-chart's shapes/colors
// are self-documenting rather than tribal knowledge.
export function describeNode(data: FlowNodeData): string {
  if (data.kind === "terminator") {
    return data.label === "start" ? "Start of the workflow" : "End of the workflow";
  }

  const base = (() => {
    switch (data.kind) {
      case "entry":
        return "Entry state — where the run begins";
      case "output":
        return "Terminal state — where the run ends";
      case "agent":
        return "Agent / workflow state";
      case "prompt":
        return "Prompt step";
      case "tool":
        return "Tool call";
      case "branch":
        return "Branch / parallel step";
      default:
        return "Agent / workflow state";
    }
  })();

  return data.hasComposition ? `${base} · click to expand its composition` : base;
}

// WorkflowNode — the glyph-styled React Flow custom node for states
// (collapsed view), steps (expanded, inside a GroupNode), and the synthetic
// __start/__end terminators. The node's own container is transparent and
// exactly the size of the glyph itself — a small dot/diamond/ring with a
// halo — so the (hidden) left/right Handles React Flow anchors edges to sit
// at the glyph's center, not a box corner. Shows a small "⤵" badge when the
// state owns a composition (also the click-to-expand affordance — see
// WorkflowGraphView's onNodeClick), and dims to a faint glimmer when
// `data.dim` (not visited this run / not reachable under the current
// toggles).
export function WorkflowNode({ data }: NodeProps & { data: FlowNodeData }) {
  if (data.kind === "terminator") {
    // Non-clickable, never dimmed — on-brand wayfinding chrome rather than
    // another state on the chart.
    const description = describeNode(data);
    return (
      <div className="atlas-node" title={description} aria-label={description}>
        <Handle type="target" position={Position.Left} isConnectable={false} />
        <span className="atlas-node__glyph atlas-node__glyph--terminator" />
        <div className="atlas-node__label atlas-node__label--terminator">{data.label}</div>
        <Handle type="source" position={Position.Right} isConnectable={false} />
      </div>
    );
  }

  const style = KIND_STYLE[data.kind] ?? KIND_STYLE.agent;
  const glyphClass = [
    "atlas-node__glyph",
    style.diamond ? "atlas-node__glyph--diamond" : "",
    style.glow ? "atlas-node__glyph--glow" : "",
  ]
    .filter(Boolean)
    .join(" ");
  // A composition-owning state is the only clickable node kind — clicking it
  // (or, once expanded, its group — see GroupNode.tsx) drills in/out.
  const clickable = Boolean(data.hasComposition);
  const description = describeNode(data);

  return (
    <div
      className={`atlas-node${style.large ? " atlas-node--large" : ""}${clickable ? " atlas-node--clickable" : ""}`}
      style={{ opacity: data.dim ? 0.35 : 1 }}
      title={description}
      aria-label={description}
    >
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
