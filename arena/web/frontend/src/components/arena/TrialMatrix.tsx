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
// The widest a cell's two lines get is "★ 100% 10/10" over "$12.345 10m 34s"
// — 105px of content plus the 28px of horizontal padding. Below that a cell
// starts wrapping, so the columns overflow into the scroller instead.
const MIN_COLUMN_WIDTH = 136;
const SCENARIO_COLUMN_WIDTH = 180;

// The scenario labels stay pinned while the contenders scroll past them —
// without this a wide field scrolls the row's identity off the left edge.
const stickyColumnStyle: React.CSSProperties = {
  position: "sticky",
  left: 0,
  zIndex: 1,
  background: "var(--surface-card)",
  borderRight: "1px solid var(--hairline)",
};

// The header row stays pinned as the scenarios scroll under it — a deep field
// otherwise leaves you looking at unlabelled columns of run glyphs from about
// row twelve down.
const stickyHeaderStyle: React.CSSProperties = {
  position: "sticky",
  top: 0,
  zIndex: 2,
  background: "var(--surface-card)",
};

// The corner is where the two sticky axes cross, so it has to outrank both.
const stickyCornerStyle: React.CSSProperties = {
  ...stickyColumnStyle,
  zIndex: 3,
};

export function TrialMatrix({ matrix, selectedKey, onSelect, onRunCell }: TrialMatrixProps) {
  // `minmax(MIN, 1fr)` rather than a bare `1fr`: fr has no floor, so a wide
  // field used to collapse every column past the point of legibility and the
  // card's `overflow: hidden` clipped the remainder away silently.
  const columnCount = Math.max(1, matrix.providers.length);
  const gridTemplateColumns = `${SCENARIO_COLUMN_WIDTH}px repeat(${columnCount}, minmax(${MIN_COLUMN_WIDTH}px, 1fr))`;
  // The grid box has to span the whole scrollable width, not just the visible
  // scrollport: a sticky item can only travel inside its containing block, so
  // without this the pinned scenario column slides away once you scroll past
  // one screen. Below the threshold this is under 100% and the fr tracks
  // stretch to fill as before.
  const gridMinWidth = SCENARIO_COLUMN_WIDTH + columnCount * MIN_COLUMN_WIDTH;
  const gridStyle: React.CSSProperties = { display: "grid", gridTemplateColumns, minWidth: gridMinWidth };

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

      {/* One scroller around the header grid and every row grid: they are
          separate grids sharing one template, so they only stay in step if
          they scroll together. */}
      {/* The pane scrolls in both axes and is height-bounded, so its horizontal
          scrollbar stays on screen. Left unbounded, a 300-scenario matrix puts
          that scrollbar ~18,000px down the page where nobody finds it. */}
      <div
        data-matrix-scroll="true"
        style={{ overflowX: "auto", overflowY: "auto", maxHeight: "70vh" }}
      >
        <div
          data-matrix-grid="true"
          data-matrix-header="true"
          style={{ ...gridStyle, ...stickyHeaderStyle, borderBottom: "1px solid var(--hairline)" }}
        >
          <div
            data-matrix-corner="true"
            style={{
              ...stickyCornerStyle,
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
              title={p.label}
              style={{
                padding: "11px 14px",
                font: "600 12px var(--font-sans)",
                color: "var(--star-300)",
                borderLeft: "1px solid var(--hairline-faint)",
                overflowWrap: "anywhere",
              }}
            >
              {p.label}
            </div>
          ))}
        </div>

        {matrix.rows.map((row) => (
          <div
            key={row.scenarioId}
            data-matrix-grid="true"
            style={{ ...gridStyle, borderTop: "1px solid var(--hairline-faint)" }}
          >
            <div
              data-matrix-rowlabel="true"
              style={{
                ...stickyColumnStyle,
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
      </div>
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
