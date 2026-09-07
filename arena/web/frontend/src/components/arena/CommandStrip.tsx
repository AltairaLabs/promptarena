import { Button } from "@altairalabs/atlas";
import type { FieldEstimate } from "@/types";
import { formatDuration } from "@/lib/utils";
import { FieldPicker } from "@/components/arena/FieldPicker";
import { MAX_SLICE_PROVIDERS, MAX_SLICE_SCENARIOS } from "@/lib/fieldSlice";

// MAX_RUNS mirrors the backend cap (maxFieldRuns) on "Run the field × N".
export const MAX_RUNS = 25;

export interface CommandStripProps {
  scenarios: { id: string; label?: string }[];
  selected: string[];
  onSelectScenarios: (ids: string[]) => void;
  // The contender axis. There was no provider picker at all before, which
  // mattered because columns are the axis that overflows.
  providers: { id: string; label?: string }[];
  selectedProviders: string[];
  onSelectProviders: (ids: string[]) => void;
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
// single-select, but a field run covers MANY scenarios against MANY contenders,
// so this composes two FieldPickers (scenarios, contenders), a readout of the
// blast radius, and the gold "Run the field" action.
//
// The two selections are double-duty: they are both what the trial matrix draws
// and what a field run covers. That is why the pickers cap — a slice is what
// keeps a thousand-scenario field renderable AND keeps a run affordable.
export function CommandStrip({
  scenarios,
  selected,
  onSelectScenarios,
  providers,
  selectedProviders,
  onSelectProviders,
  runCount,
  onRunCountChange,
  onRunTrial,
  runDisabled,
  estimate,
}: CommandStripProps) {
  const providerCount = selectedProviders.length;
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
        flexDirection: "column",
        gap: 14,
        padding: "14px 18px",
        borderRadius: "var(--radius-xl)",
        border: "1px solid var(--hairline)",
        background: "var(--surface)",
      }}
    >
      {/* Control bar: the blast radius and the action. The pickers sit below it
          at full width — squeezed into a column beside these they wrap one chip
          per line, which is unreadable at any real field size. */}
      <div style={{ display: "flex", alignItems: "center", gap: 16, flexWrap: "wrap" }}>
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

        <span style={{ font: "12px var(--font-mono)", color: "var(--star-700)", marginLeft: "auto" }}>
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

      <FieldPicker
        label="Scenarios"
        items={scenarios}
        selected={selected}
        onChange={onSelectScenarios}
        cap={MAX_SLICE_SCENARIOS}
      />
      <FieldPicker
        label="Contenders"
        items={providers}
        selected={selectedProviders}
        onChange={onSelectProviders}
        cap={MAX_SLICE_PROVIDERS}
      />
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
