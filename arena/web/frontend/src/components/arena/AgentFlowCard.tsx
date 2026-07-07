import { ConstellationGraph } from "@/components/atlas/ConstellationGraph";
import type { GraphNode, GraphEdge } from "@/components/atlas/types";

export interface AgentFlowCardProps {
  flow: { nodes: GraphNode[]; edges: GraphEdge[] };
}

// AgentFlowCard — the Trial Inspector's right-rail top card: wraps the
// ConstellationGraph reading of a run's message/tool sequence, built
// upstream by `buildAgentFlow` in `lib/arenaView.ts`.
export function AgentFlowCard({ flow }: AgentFlowCardProps) {
  return (
    <div
      style={{
        border: "1px solid var(--hairline)",
        borderRadius: "var(--radius-2xl)",
        background: "var(--grad-surface)",
        overflow: "hidden",
      }}
    >
      <div
        style={{
          font: "11px var(--font-mono)",
          textTransform: "uppercase",
          letterSpacing: "0.1em",
          color: "var(--star-900)",
          padding: "13px 16px",
          borderBottom: "1px solid var(--hairline)",
        }}
      >
        AGENT FLOW
      </div>
      <div style={{ padding: 14 }}>
        <ConstellationGraph nodes={flow.nodes} edges={flow.edges} width={360} height={150} showLabels />
      </div>
    </div>
  );
}
