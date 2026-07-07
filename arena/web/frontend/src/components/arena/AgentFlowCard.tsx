import { useState } from "react";
import { WorkflowGraphView } from "@/components/arena/workflow/WorkflowGraphView";
import { buildFlowElements } from "@/lib/workflowFlow";
import type { RunResult, ActiveRun, WorkflowGraph } from "@/types";

export interface AgentFlowCardProps {
  graph: WorkflowGraph | null;
  run: RunResult | ActiveRun | undefined;
  // theme is threaded down from App's single useTheme() owner rather than
  // read locally — useTheme() has independent per-call state, so a second
  // call here would go stale whenever the TopBar's toggle flips the app's
  // theme without re-mounting this card.
  theme: "light" | "dark";
}

// ToggleChip — the small pill button shared by both header controls. Mirrors
// CommandStrip's scenario-chip styling (gold when active, hairline outline
// otherwise) at a size that fits the WORKFLOW card's compact header.
function ToggleChip({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      style={{
        font: "500 10px var(--font-mono)",
        padding: "4px 9px",
        border: "none",
        background: active ? "var(--gold-tint)" : "transparent",
        color: active ? "var(--gold-300)" : "var(--star-600)",
        cursor: "pointer",
        transition: "all .15s ease",
      }}
    >
      {label}
    </button>
  );
}

// AgentFlowCard — the Trial Inspector's right-rail top card: renders the
// real workflow topology (fetched once via getWorkflow) with the selected
// run's path overlaid — visited nodes/edges stay lit, unvisited ones dim.
// Until the graph has loaded (App's initial fetch hasn't resolved yet), the
// card renders its shell with an empty/placeholder body rather than crash.
// The two header toggles (compositions expand/collapse, this-run-only) live
// as local state here and feed straight into buildFlowElements — everything
// downstream (group nodes, dropping unvisited states) is already handled by
// the selector + WorkflowGraphView.
export function AgentFlowCard({ graph, run, theme }: AgentFlowCardProps) {
  const [compositionsExpanded, setCompositionsExpanded] = useState(false);
  const [thisRunOnly, setThisRunOnly] = useState(false);
  const elements = graph ? buildFlowElements(graph, run, { compositionsExpanded, thisRunOnly }) : null;

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
          display: "flex",
          alignItems: "center",
          padding: "13px 16px",
          borderBottom: "1px solid var(--hairline)",
        }}
      >
        <span
          style={{
            font: "11px var(--font-mono)",
            textTransform: "uppercase",
            letterSpacing: "0.1em",
            color: "var(--star-900)",
          }}
        >
          WORKFLOW
        </span>
        <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 8 }}>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              border: "1px solid var(--hairline-strong)",
              borderRadius: "var(--radius-sm)",
              overflow: "hidden",
            }}
          >
            <ToggleChip label="Collapsed" active={!compositionsExpanded} onClick={() => setCompositionsExpanded(false)} />
            <div style={{ width: 1, alignSelf: "stretch", background: "var(--hairline-strong)" }} />
            <ToggleChip label="Expanded" active={compositionsExpanded} onClick={() => setCompositionsExpanded(true)} />
          </div>
          <div
            style={{
              border: "1px solid var(--hairline-strong)",
              borderRadius: "var(--radius-sm)",
              overflow: "hidden",
            }}
          >
            <ToggleChip label="This run only" active={thisRunOnly} onClick={() => setThisRunOnly((v) => !v)} />
          </div>
        </div>
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
