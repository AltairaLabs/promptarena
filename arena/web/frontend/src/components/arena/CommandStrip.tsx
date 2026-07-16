import { Button } from "@altairalabs/atlas";

export interface CommandStripProps {
  scenarios: { id: string; label?: string }[];
  selectedScenario: string | null;
  selectedProviderLabel: string | null;
  onSelectScenario: (scenarioId: string) => void;
  onRunTrial: () => void;
  runDisabled?: boolean;
}

// CommandStrip — "chart a run" in one glance: scenario chips (clicking one
// selects that scenario's best/first provider upstream in App) plus the
// gold Run trial action. Sits directly under the Hero, replacing the old
// RunControls/EmptyStateLauncher pickers.
export function CommandStrip({
  scenarios,
  selectedScenario,
  selectedProviderLabel,
  onSelectScenario,
  onRunTrial,
  runDisabled,
}: CommandStripProps) {
  return (
    <div
      style={{
        marginTop: 26,
        display: "flex",
        alignItems: "center",
        gap: 14,
        flexWrap: "wrap",
        padding: "14px 16px",
        border: "1px solid var(--hairline)",
        borderRadius: "var(--radius-xl)",
        background: "var(--grad-surface)",
      }}
    >
      <span
        style={{
          font: "500 10px var(--font-mono)",
          letterSpacing: "0.14em",
          textTransform: "uppercase",
          color: "var(--star-900)",
        }}
      >
        CHART A RUN
      </span>

      <div style={{ display: "flex", gap: 7, flexWrap: "wrap" }}>
        {scenarios.map((s) => {
          const active = s.id === selectedScenario;
          return (
            <button
              key={s.id}
              type="button"
              onClick={() => onSelectScenario(s.id)}
              style={{
                font: "500 12px var(--font-mono)",
                padding: "8px 12px",
                borderRadius: 999,
                transition: "all .15s ease",
                border: active ? "1px solid var(--gold-border)" : "1px solid var(--hairline-strong)",
                background: active ? "var(--gold-tint)" : "transparent",
                color: active ? "var(--gold-300)" : "var(--star-600)",
                cursor: "pointer",
              }}
            >
              {s.label ?? s.id}
            </button>
          );
        })}
      </div>

      <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 12 }}>
        <span style={{ font: "400 12px var(--font-mono)", color: "var(--star-700)" }}>
          {selectedProviderLabel ?? "—"} · {selectedScenario ?? "—"}
        </span>
        <Button variant="primary" onClick={onRunTrial} disabled={runDisabled}>
          ▶ Run trial
        </Button>
      </div>
    </div>
  );
}
