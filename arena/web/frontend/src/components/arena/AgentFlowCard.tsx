import { useMemo, useState } from "react";
import { WorkflowGraphView } from "@/components/arena/workflow/WorkflowGraphView";
import { buildFlowElements } from "@/lib/workflowFlow";
import type { RunResult, ActiveRun, WorkflowGraph } from "@/types";

// compositionStateIds — every state id that owns at least one composition
// step (a node with `.parent` set), in graph order. Drives both the
// "Expand all" button's target set and (indirectly, via buildFlowElements)
// which clicked node ids actually do anything.
function compositionStateIds(graph: WorkflowGraph | null): string[] {
  if (!graph) return [];
  const ids = new Set<string>();
  for (const n of graph.nodes) {
    if (n.parent) ids.add(n.parent);
  }
  return Array.from(ids);
}

export interface AgentFlowCardProps {
  graph: WorkflowGraph | null;
  run: RunResult | ActiveRun | undefined;
  // theme is threaded down from App's single useTheme() owner rather than
  // read locally — useTheme() has independent per-call state, so a second
  // call here would go stale whenever the TopBar's toggle flips the app's
  // theme without re-mounting this card.
  theme: "light" | "dark";
}

// ToggleChip — the small pill button shared by the "This run only" header
// control. Mirrors CommandStrip's scenario-chip styling (gold when active,
// hairline outline otherwise) at a size that fits the WORKFLOW card's
// compact header.
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

// TextButton — the borderless, no-pill action button for "Expand all" /
// "Collapse all". Unlike ToggleChip it isn't a persistent on/off state, so
// it gets a plain hover color shift (star-600 -> gold-300) rather than a
// filled "active" background, mirroring TrialMatrix's transparent run-cell
// affordance at ToggleChip's type scale.
function TextButton({ label, onClick }: { label: string; onClick: () => void }) {
  const [hover, setHover] = useState(false);
  return (
    <button
      type="button"
      onClick={onClick}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        font: "500 10px var(--font-mono)",
        padding: "4px 2px",
        border: "none",
        background: "transparent",
        color: hover ? "var(--gold-300)" : "var(--star-600)",
        cursor: "pointer",
        transition: "color .15s ease",
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
// `expandedStates` tracks which composition-owning states are drilled in,
// per-state — clicking a state node in the graph (WorkflowGraphView's
// onStateClick) toggles its membership; the header's "Expand all"/"Collapse
// all" button is just a shortcut that sets/clears the whole set at once.
// Both this and the "this-run-only" toggle live as local state here and
// feed straight into buildFlowElements — everything downstream (group
// nodes, dropping unvisited states) is already handled by the selector +
// WorkflowGraphView.
export function AgentFlowCard({ graph, run, theme }: AgentFlowCardProps) {
  const [expandedStates, setExpandedStates] = useState<string[]>([]);
  const [thisRunOnly, setThisRunOnly] = useState(false);
  const elements = graph ? buildFlowElements(graph, run, { expandedStates, thisRunOnly }) : null;

  const allCompositionIds = useMemo(() => compositionStateIds(graph), [graph]);
  const allExpanded = allCompositionIds.length > 0 && allCompositionIds.every((id) => expandedStates.includes(id));

  const toggleState = (stateId: string) =>
    setExpandedStates((prev) => (prev.includes(stateId) ? prev.filter((id) => id !== stateId) : [...prev, stateId]));

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
          {allCompositionIds.length > 0 && (
            <TextButton
              label={allExpanded ? "Collapse all" : "Expand all"}
              onClick={() => setExpandedStates(allExpanded ? [] : allCompositionIds)}
            />
          )}
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
          <WorkflowGraphView elements={elements} theme={theme} onStateClick={toggleState} />
        ) : (
          <div style={{ font: "12px var(--font-mono)", color: "var(--star-600)", padding: "22px 18px" }}>
            Loading workflow…
          </div>
        )}
      </div>
    </div>
  );
}
