import starGlyphGold from "@/assets/star-glyph-gold.svg";
import type { TrialMatrix as TrialMatrixModel, TrialCell } from "@/types";

export interface TrialMatrixProps {
  matrix: TrialMatrixModel;
  selectedKey: string | null;
  onSelect: (key: string) => void;
}

// TrialMatrix — the Atlas redesign's centerpiece: a scenario × provider grid
// where each cell is a clickable trial readout (pass rate, cost, latency).
// Purely presentational — the matrix viewmodel is built upstream by
// `buildMatrix` in `lib/arenaView.ts`.
export function TrialMatrix({ matrix, selectedKey, onSelect }: TrialMatrixProps) {
  const gridTemplateColumns = `180px repeat(${Math.max(1, matrix.providers.length)}, 1fr)`;

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
          gap: 16,
          padding: "14px 18px",
          borderBottom: "1px solid var(--hairline)",
        }}
      >
        <span
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: 11,
            textTransform: "uppercase",
            letterSpacing: "0.1em",
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
            <MatrixCell key={cell.key} cell={cell} selected={cell.key === selectedKey} onSelect={onSelect} />
          ))}
        </div>
      ))}
    </div>
  );
}

function MatrixCell({
  cell,
  selected,
  onSelect,
}: {
  cell: TrialCell;
  selected: boolean;
  onSelect: (key: string) => void;
}) {
  if (!cell.hasData) {
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

  const background = selected
    ? "color-mix(in srgb, var(--ion-cyan) 9%, transparent)"
    : cell.best
      ? "var(--gold-tint)"
      : "transparent";
  const boxShadow = selected ? "inset 0 0 0 1.5px var(--ion-cyan)" : "none";
  const rateColor = cell.best ? "var(--gold-300)" : !cell.passed ? "var(--signal-red-300)" : "var(--star-200)";

  return (
    <button
      type="button"
      onClick={() => onSelect(cell.key)}
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
      <div style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 8 }}>
        {cell.best ? (
          <img
            src={starGlyphGold}
            alt="best"
            style={{ width: 15, height: 15, filter: "drop-shadow(0 0 6px rgba(227,179,65,.9))" }}
          />
        ) : (
          <span
            style={{
              width: 8,
              height: 8,
              borderRadius: "50%",
              background: cell.passed ? "var(--pulsar-500)" : "var(--signal-red)",
            }}
          />
        )}
        <span style={{ font: "600 16px var(--font-mono)", color: rateColor }}>{cell.passRate}%</span>
      </div>
      <div style={{ display: "flex", gap: 12, font: "11px var(--font-mono)", color: "var(--star-800)" }}>
        <span>{cell.costUsd ? `$${cell.costUsd.toFixed(3)}` : "free"}</span>
        <span>{cell.latencyMs}ms</span>
      </div>
    </button>
  );
}
