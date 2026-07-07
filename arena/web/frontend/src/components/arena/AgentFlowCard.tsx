import { ConstellationGraph } from "@/components/atlas/ConstellationGraph";
import { overlayWorkflowRun } from "@/lib/arenaView";
import { layoutWorkflow } from "@/lib/workflowLayout";
import type { RunResult, ActiveRun, WorkflowGraph } from "@/types";

export interface AgentFlowCardProps {
  graph: WorkflowGraph | null;
  run: RunResult | ActiveRun | undefined;
}

// AgentFlowCard — the Trial Inspector's right-rail top card: renders the
// real workflow topology (fetched once via getWorkflow) with the selected
// run's path overlaid — visited nodes/edges stay lit, unvisited ones dim.
// Until the graph has loaded (App's initial fetch hasn't resolved yet), the
// card renders its shell with an empty/placeholder body rather than crash.
export function AgentFlowCard({ graph, run }: AgentFlowCardProps) {
  const laid = graph ? layoutWorkflow(overlayWorkflowRun(graph, run)) : null;

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
        WORKFLOW
      </div>
      <div style={{ padding: 14, maxHeight: 320, overflow: "auto" }}>
        {laid ? (
          <ConstellationGraph nodes={laid.nodes} edges={laid.edges} width={laid.width} height={laid.height} showLabels />
        ) : (
          <div style={{ font: "12px var(--font-mono)", color: "var(--star-600)", padding: "8px 4px" }}>
            Loading workflow…
          </div>
        )}
      </div>
    </div>
  );
}
