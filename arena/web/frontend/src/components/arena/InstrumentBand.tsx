import { Gauge, InstrumentReadout, StarTrail } from "@/components/atlas";
import { buildOverallGauge, buildMetrics, buildTrend, buildStandings } from "@/lib/arenaView";
import { Standings } from "@/components/arena/Standings";
import type { TrialMatrix, RunResult } from "@/types";

export interface InstrumentBandProps {
  matrix: TrialMatrix;
  results: RunResult[];
}

const panelCardStyle: React.CSSProperties = {
  border: "1px solid var(--hairline)",
  borderRadius: "var(--radius-2xl)",
  background: "var(--grad-surface)",
  overflow: "hidden",
};

const sectionLabelStyle: React.CSSProperties = {
  fontFamily: "var(--font-mono)",
  fontSize: 11,
  textTransform: "uppercase",
  letterSpacing: "0.1em",
  color: "var(--star-900)",
};

// InstrumentBand — the Atlas redesign's top-of-page instrument cluster:
// an overall pass-rate gauge, a metrics readout with a recent-trend
// sparkline, and the provider standings. Replaces the old SummaryCards.
// Purely presentational — every number comes from the arenaView selectors.
export function InstrumentBand({ matrix, results }: InstrumentBandProps) {
  const gauge = buildOverallGauge(matrix);
  const metrics = buildMetrics(results, matrix);
  const trend = buildTrend(results);
  const standings = buildStandings(matrix);

  const delta = trend.length > 0 ? trend[trend.length - 1] - trend[0] : 0;
  const deltaText = delta >= 0 ? `▲ +${delta}` : `▼ ${delta}`;

  return (
    <div style={{ display: "grid", gridTemplateColumns: "0.9fr 1.5fr 1.1fr", gap: 16, marginBottom: 16 }}>
      <div style={{ ...panelCardStyle, padding: 18, display: "flex", flexDirection: "column" }}>
        <span style={sectionLabelStyle}>PASS RATE · ALL TRIALS</span>
        <div style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center" }}>
          <Gauge value={gauge.passRate} max={100} unit="converged" label={gauge.caption} />
        </div>
      </div>

      <div style={{ ...panelCardStyle, overflow: "hidden", display: "flex", flexDirection: "column" }}>
        <InstrumentReadout metrics={metrics} columns={4} />
        {trend.length > 0 && (
          <div style={{ padding: "14px 18px", borderTop: "1px solid var(--hairline)" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
              <span style={sectionLabelStyle}>PASS RATE · LAST 12 RUNS</span>
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--gold-300)" }}>
                {deltaText}
              </span>
            </div>
            <StarTrail points={trend} height={64} />
          </div>
        )}
      </div>

      <Standings standings={standings} />
    </div>
  );
}
