import { WorkflowGraphView } from "@/components/arena/workflow/WorkflowGraphView";
import { useTheme } from "@/hooks/useTheme";
import { buildFlowElements, type FlowToggles } from "@/lib/workflowFlow";
import type { RunResult, ActiveRun, WorkflowGraph } from "@/types";

export interface AgentFlowCardProps {
  graph: WorkflowGraph | null;
  run: RunResult | ActiveRun | undefined;
}

// Fixed defaults for this unit — the interactive compositions/this-run-only
// toggles are a later unit's UI; until then every panel renders collapsed
// (states only) and shows the full topology regardless of the selected run.
const DEFAULT_TOGGLES: FlowToggles = { compositionsExpanded: false, thisRunOnly: false };

// AgentFlowCard — the Trial Inspector's right-rail top card: renders the
// real workflow topology (fetched once via getWorkflow) with the selected
// run's path overlaid — visited nodes/edges stay lit, unvisited ones dim.
// Until the graph has loaded (App's initial fetch hasn't resolved yet), the
// card renders its shell with an empty/placeholder body rather than crash.
export function AgentFlowCard({ graph, run }: AgentFlowCardProps) {
  const { theme } = useTheme();
  const elements = graph ? buildFlowElements(graph, run, DEFAULT_TOGGLES) : null;

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
      <div style={{ height: 360 }}>
        {elements ? (
          <WorkflowGraphView elements={elements} theme={theme} />
        ) : (
          <div style={{ font: "12px var(--font-mono)", color: "var(--star-600)", padding: "22px 18px" }}>
            Loading workflow…
          </div>
        )}
      </div>
    </div>
  );
}
