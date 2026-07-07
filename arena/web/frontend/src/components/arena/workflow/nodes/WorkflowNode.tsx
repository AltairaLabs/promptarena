import type { CSSProperties } from "react";
import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { FlowNodeData } from "@/lib/workflowFlow";

// KIND_STYLE — the box-styled counterpart to ConstellationGraph's KIND map:
// entry/output are the gold "star" nodes, agent is starlight, prompt is
// nebula violet, tool is ion cyan, branch is a gold diamond-flavored box.
// Borders/backgrounds are derived from the same token via color-mix() so
// there's no new hex to keep in sync across the light/dark theme flip.
const KIND_STYLE: Record<
  FlowNodeData["kind"],
  { accent: string; border: string; background: string; glow?: string; diamond?: boolean }
> = {
  entry: {
    accent: "var(--gold-500)",
    border: "var(--gold-border)",
    background: "var(--gold-tint)",
    glow: "var(--glow-gold-soft)",
  },
  output: {
    accent: "var(--gold-500)",
    border: "var(--gold-border)",
    background: "var(--gold-tint)",
    glow: "var(--glow-gold-soft)",
  },
  agent: {
    accent: "var(--starlight-300)",
    border: "var(--starlight-border)",
    background: "var(--starlight-tint)",
  },
  prompt: {
    accent: "var(--nebula-violet)",
    border: "color-mix(in srgb, var(--nebula-violet) 45%, transparent)",
    background: "color-mix(in srgb, var(--nebula-violet) 12%, transparent)",
  },
  tool: {
    accent: "var(--ion-cyan)",
    border: "color-mix(in srgb, var(--ion-cyan) 45%, transparent)",
    background: "color-mix(in srgb, var(--ion-cyan) 12%, transparent)",
  },
  branch: {
    accent: "var(--gold-500)",
    border: "var(--gold-border)",
    background: "var(--gold-tint)",
    diamond: true,
  },
};

// WorkflowNode — the box-shaped React Flow custom node for both states
// (collapsed view) and steps (expanded, inside a GroupNode). Styled by
// `data.kind`; shows a small "⤵" badge when the state owns a composition,
// and dims to a faint outline when `data.dim` (not visited this run / not
// reachable under the current toggles).
export function WorkflowNode({ data }: NodeProps & { data: FlowNodeData }) {
  const style = KIND_STYLE[data.kind] ?? KIND_STYLE.agent;

  return (
    <div
      style={{
        position: "relative",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        minWidth: 140,
        minHeight: 44,
        padding: "8px 14px",
        borderRadius: style.diamond ? "6px" : "var(--radius-lg)",
        border: `1.5px solid ${style.border}`,
        background: style.background,
        boxShadow: style.glow ? `0 0 0 1px transparent, ${style.glow}` : undefined,
        opacity: data.dim ? 0.3 : 1,
        transition: "opacity var(--dur-base, 200ms) var(--ease-standard, ease)",
      }}
    >
      <Handle type="target" position={Position.Left} style={HANDLE_STYLE} />
      <span
        style={{
          font: "600 12px var(--font-mono)",
          color: "var(--star-300)",
          textAlign: "center",
          whiteSpace: "normal",
          wordBreak: "break-word",
        }}
      >
        {data.label}
      </span>
      {data.hasComposition && (
        <span
          aria-label="has composition"
          style={{
            position: "absolute",
            top: -8,
            right: -8,
            width: 18,
            height: 18,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            borderRadius: "999px",
            background: "var(--surface-2)",
            border: `1px solid ${style.accent}`,
            color: style.accent,
            font: "11px var(--font-mono)",
            lineHeight: 1,
          }}
        >
          ⤵
        </span>
      )}
      <Handle type="source" position={Position.Right} style={HANDLE_STYLE} />
    </div>
  );
}

// Handles just need to exist for React Flow to route edges to/from this
// node in the LR layout dagre produced; they carry no visual weight of
// their own beyond a faint dot on the accent color.
const HANDLE_STYLE: CSSProperties = {
  width: 6,
  height: 6,
  background: "var(--starlight-500)",
  border: "none",
  opacity: 0.6,
};
