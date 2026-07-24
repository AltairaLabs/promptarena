import { Button } from "@altairalabs/atlas";
import type { FieldEstimate } from "@/types";
import { formatDuration } from "@/lib/utils";

// MAX_RUNS mirrors the backend cap (maxFieldRuns) on "Run the field × N".
export const MAX_RUNS = 25;

export interface CommandStripProps {
  scenarios: { id: string; label?: string }[];
  selected: string[];
  onToggle: (scenarioId: string) => void;
  onSelectAll: () => void;
  onSelectNone: () => void;
  providerCount: number;
  runCount: number;
  onRunCountChange: (n: number) => void;
  onRunTrial: () => void;
  runDisabled?: boolean;
  // estimate projects spend + wall-clock for the field run from history; null/
  // undefined until there's enough past data to estimate from.
  estimate?: FieldEstimate | null;
}

// formatCost renders an estimated spend compactly: sub-cent totals collapse to
// "<$0.01" rather than a misleading "$0.00", everything else is two decimals.
function formatCost(usd: number): string {
  if (usd <= 0) return "$0.00";
  if (usd < 0.01) return "<$0.01";
  return `$${usd.toFixed(2)}`;
}

// CommandStrip — Arena's "chart a run" strip. The Atlas CommandStrip is
// single-select, but a field run covers MANY scenarios at once, so this is a
// custom multi-select pill row: every scenario is a toggle (all selected by
// default upstream), plus All/None shortcuts, a readout of the blast radius,
// and the gold "Run the field" action that runs every selected scenario across
// every provider.
export function CommandStrip({
  scenarios,
  selected,
  onToggle,
  onSelectAll,
  onSelectNone,
  providerCount,
  runCount,
  onRunCountChange,
  onRunTrial,
  runDisabled,
  estimate,
}: CommandStripProps) {
  const selectedSet = new Set(selected);
  const scenarioLabel = selected.length === 1 ? "1 scenario" : `${selected.length} scenarios`;
  const contenderLabel = providerCount === 1 ? "1 contender" : `${providerCount} contenders`;
  const totalTrials = runCount * selected.length * providerCount;
  const sweepPrefix = runCount === 1 ? "" : `${runCount} sweeps · `;
  // Partial coverage (some target cells have no history) means the totals are a
  // floor, so "≥" rather than "≈"; a full estimate is "≈".
  const partial = estimate != null && estimate.covered < estimate.total;

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 16,
        flexWrap: "wrap",
        padding: "14px 18px",
        borderRadius: "var(--radius-xl)",
        border: "1px solid var(--hairline)",
        background: "var(--surface)",
      }}
    >
      <span
        style={{
          fontFamily: "var(--font-mono)",
          fontSize: "var(--text-size-mono-label)",
          fontWeight: "var(--fw-medium)",
          textTransform: "uppercase",
          letterSpacing: "var(--tracking-eyebrow)",
          color: "var(--star-900)",
          flex: "none",
        }}
      >
        CHART A RUN
      </span>

      <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", flex: 1, minWidth: 0 }}>
        {scenarios.map((s) => {
          const active = selectedSet.has(s.id);
          return (
            <button
              key={s.id}
              type="button"
              onClick={() => onToggle(s.id)}
              aria-pressed={active}
              style={{
                cursor: "pointer",
                padding: "5px 12px",
                borderRadius: "999px",
                font: "500 13px var(--font-sans)",
                background: active ? "var(--starlight-tint)" : "transparent",
                border: `1px solid ${active ? "var(--starlight-300)" : "var(--hairline)"}`,
                color: active ? "var(--star-100)" : "var(--star-600)",
                transition: "background .12s ease, color .12s ease",
              }}
            >
              {s.label ?? s.id}
            </button>
          );
        })}
        <span style={{ display: "inline-flex", gap: 8, marginLeft: 4 }}>
          <button type="button" onClick={onSelectAll} style={quickToggleStyle}>All</button>
          <button type="button" onClick={onSelectNone} style={quickToggleStyle}>None</button>
        </span>
      </div>

      <span style={{ font: "12px var(--font-mono)", color: "var(--star-700)", flex: "none" }}>
        {sweepPrefix}{scenarioLabel} · {contenderLabel} = {totalTrials} trial{totalTrials === 1 ? "" : "s"}
        {estimate && (
          <span
            style={{ color: "var(--star-500)", marginLeft: 8 }}
            title={
              partial
                ? `estimated from ${estimate.covered} of ${estimate.total} cells with history — actual will be higher`
                : `estimated from past runs of all ${estimate.total} cells`
            }
          >
            {partial ? "≥" : "≈"} {formatCost(estimate.costUsd)} · ~{formatDuration(estimate.timeMs)}
          </span>
        )}
      </span>

      <div style={{ display: "inline-flex", alignItems: "center", gap: 4, flex: "none" }}>
        <button
          type="button"
          onClick={() => onRunCountChange(Math.max(1, runCount - 1))}
          disabled={runCount <= 1}
          aria-label="fewer sweeps"
          style={stepBtnStyle}
        >
          −
        </button>
        <span style={{ font: "600 13px var(--font-mono)", color: "var(--star-200)", minWidth: 24, textAlign: "center" }}>
          ×{runCount}
        </span>
        <button
          type="button"
          onClick={() => onRunCountChange(Math.min(MAX_RUNS, runCount + 1))}
          disabled={runCount >= MAX_RUNS}
          aria-label="more sweeps"
          style={stepBtnStyle}
        >
          +
        </button>
      </div>

      <Button variant="primary" onClick={onRunTrial} disabled={runDisabled}>
        ▶ Run the field
      </Button>
    </div>
  );
}

const stepBtnStyle: React.CSSProperties = {
  cursor: "pointer",
  width: 24,
  height: 24,
  borderRadius: "var(--radius-sm)",
  border: "1px solid var(--hairline)",
  background: "var(--surface-2)",
  color: "var(--star-300)",
  font: "600 14px var(--font-mono)",
  lineHeight: 1,
};

const quickToggleStyle: React.CSSProperties = {
  cursor: "pointer",
  background: "transparent",
  border: "none",
  padding: "2px 4px",
  font: "500 11px var(--font-mono)",
  textTransform: "uppercase",
  letterSpacing: "0.08em",
  color: "var(--text-link)",
};
