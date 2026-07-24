import starGlyphGold from "@/assets/star-glyph-gold.svg";
import { Card } from "@altairalabs/atlas";
import { formatDuration } from "@/lib/utils";
import type { TrialMatrix as TrialMatrixModel, TrialCell } from "@/types";

// Per-trial marks for a cell's flakiness strip.
const STRIP_COLOR: Record<"pass" | "fail" | "error", string> = {
  pass: "var(--pulsar-500)",
  fail: "var(--signal-red)",
  error: "var(--gold-500)",
};

export interface TrialMatrixProps {
  matrix: TrialMatrixModel;
  selectedKey: string | null;
  onSelect: (key: string) => void;
  // onRunCell, when provided, turns an empty (no-data) cell into a clickable
  // "run this scenario×provider" affordance. Omitted means empty cells stay
  // inert dashes.
  onRunCell?: (scenarioId: string, providerId: string) => void;
}

// TrialMatrix — the Atlas redesign's centerpiece: a scenario × provider grid
// where each cell is a clickable trial readout (pass rate, cost, latency).
// Purely presentational — the matrix viewmodel is built upstream by
// `buildMatrix` in `lib/arenaView.ts`.
export function TrialMatrix({ matrix, selectedKey, onSelect, onRunCell }: TrialMatrixProps) {
  const gridTemplateColumns = `180px repeat(${Math.max(1, matrix.providers.length)}, 1fr)`;

  return (
    <Card padding={0} style={{ overflow: "hidden" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 16,
          padding: "14px 18px",
          borderBottom: "1px solid var(--hairline)",
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
          }}
        >
          TRIAL MATRIX · SCENARIO × PROVIDER
        </span>
        <div
          style={{
            marginLeft: "auto",
            display: "flex",
            alignItems: "center",
            gap: 16,
            fontFamily: "var(--font-mono)",
            fontSize: 11,
            color: "var(--star-700)",
          }}
        >
          <span style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <img src={starGlyphGold} alt="best" style={{ width: 12, height: 12 }} />
            best
          </span>
          <span style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <span style={{ width: 8, height: 8, borderRadius: "50%", background: "var(--pulsar-500)" }} />
            pass
          </span>
          <span style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <span style={{ width: 8, height: 8, borderRadius: "50%", background: "var(--signal-red)" }} />
            fail
          </span>
        </div>
      </div>

      <div style={{ display: "grid", gridTemplateColumns, borderBottom: "1px solid var(--hairline)" }}>
        <div
          style={{
            padding: "11px 14px",
            fontFamily: "var(--font-mono)",
            fontSize: 10,
            textTransform: "uppercase",
            letterSpacing: "0.1em",
            color: "var(--star-950)",
          }}
        >
          SCENARIO
        </div>
        {matrix.providers.map((p) => (
          <div
            key={p.id}
            style={{
              padding: "11px 14px",
              font: "600 12px var(--font-sans)",
              color: "var(--star-300)",
              borderLeft: "1px solid var(--hairline-faint)",
            }}
          >
            {p.label}
          </div>
        ))}
      </div>

      {matrix.rows.map((row) => (
        <div
          key={row.scenarioId}
          style={{ display: "grid", gridTemplateColumns, borderTop: "1px solid var(--hairline-faint)" }}
        >
          <div
            style={{
              padding: "14px 18px",
              font: "500 13px/1.3 var(--font-mono)",
              color: "var(--star-400)",
            }}
          >
            {row.label}
          </div>
          {row.cells.map((cell) => (
            <MatrixCell
              key={cell.key}
              cell={cell}
              selected={cell.key === selectedKey}
              onSelect={onSelect}
              onRunCell={onRunCell}
            />
          ))}
        </div>
      ))}
    </Card>
  );
}

function MatrixCell({
  cell,
  selected,
  onSelect,
  onRunCell,
}: {
  cell: TrialCell;
  selected: boolean;
  onSelect: (key: string) => void;
  onRunCell?: (scenarioId: string, providerId: string) => void;
}) {
  if (!cell.hasData) {
    if (!onRunCell) {
      return (
        <div
          style={{
            borderLeft: "1px solid var(--hairline-faint)",
            padding: "12px 14px",
            color: "var(--star-950)",
          }}
        >
          —
        </div>
      );
    }
    // Empty cell with a run handler wired up — a subtle, no-gold affordance
    // that runs just this scenario×provider pair.
    return (
      <button
        type="button"
        onClick={() => onRunCell(cell.scenarioId, cell.providerId)}
        aria-label={`Run ${cell.scenarioId} on ${cell.providerId}`}
        style={{
          textAlign: "left",
          border: 0,
          borderLeft: "1px solid var(--hairline-faint)",
          padding: "12px 14px",
          cursor: "pointer",
          background: "transparent",
          color: "var(--star-800)",
          font: "13px var(--font-mono)",
          transition: "color .15s ease",
        }}
      >
        ▶
      </button>
    );
  }

  const background = selected
    ? "color-mix(in srgb, var(--ion-cyan) 9%, transparent)"
    : cell.best
      ? "var(--gold-tint)"
      : "transparent";
  const boxShadow = selected ? "inset 0 0 0 1.5px var(--ion-cyan)" : "none";
  // The cell is a reliability reading across the cell's runs. Unscored (no
  // assertions judged any run) is muted "—"; otherwise coloured by rate —
  // green fully-reliable, gold best, red anything that ever failed.
  const rateColor = !cell.scored
    ? "var(--star-800)"
    : cell.best
      ? "var(--gold-300)"
      : !cell.passed
        ? "var(--signal-red-300)"
        : "var(--pulsar-300)";

  return (
    <button
      type="button"
      onClick={() => onSelect(cell.key)}
      title={
        cell.scored
          ? `${cell.passedCount}/${cell.totalRuns} runs passed`
          : "Ran, but no assertions scored these runs"
      }
      style={{
        textAlign: "left",
        border: 0,
        borderLeft: "1px solid var(--hairline-faint)",
        padding: "12px 14px",
        cursor: "pointer",
        transition: "background .15s ease",
        background,
        boxShadow,
      }}
    >
      <div style={{ display: "flex", gap: 8, alignItems: "baseline", marginBottom: 8 }}>
        {cell.best && (
          <img
            src={starGlyphGold}
            alt="best"
            style={{ width: 14, height: 14, alignSelf: "center" }}
          />
        )}
        <span style={{ font: "600 16px var(--font-mono)", color: rateColor }}>
          {cell.scored ? `${cell.passRate}%` : "—"}
        </span>
        {cell.scored && (
          <span style={{ font: "11px var(--font-mono)", color: "var(--star-800)" }}>
            {cell.passedCount}/{cell.totalRuns}
          </span>
        )}
      </div>
      {cell.history.length >= 2 && (
        <div
          style={{ display: "flex", gap: 2, marginBottom: 8 }}
          title="Recent trials, oldest → newest (green pass · red fail · amber error)"
        >
          {cell.history.map((o, i) => (
            <span
              key={i}
              style={{ width: 7, height: 7, borderRadius: 1, background: STRIP_COLOR[o], flex: "none" }}
            />
          ))}
        </div>
      )}
      <div style={{ display: "flex", gap: 12, font: "11px var(--font-mono)", color: "var(--star-800)" }}>
        <span>{cell.costUsd > 0 ? `$${cell.costUsd.toFixed(3)}` : "—"}</span>
        <span>{formatDuration(cell.latencyMs)}</span>
      </div>
    </button>
  );
}
