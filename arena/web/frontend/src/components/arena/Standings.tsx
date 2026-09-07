import { Card } from "@altairalabs/atlas";
import starGlyphGold from "@/assets/star-glyph-gold.svg";
import type { Standing } from "@/types";

// The instrument band is one grid row, so the standings card sets the height
// of the gauge and the metrics beside it. A 35-entry leaderboard where 34 have
// zero wins is not a ranking anyone needs in full — it just empties the band.
const DEFAULT_LIMIT = 8;

export interface StandingsProps {
  standings: Standing[];
  // limit caps the rows rendered; the remainder is summarised in a footer.
  limit?: number;
}

// Standings — a compact scenario-wins leaderboard derived from the trial
// matrix (`buildStandings`). The leader gets the gold star treatment that
// echoes the matrix's "best" cells.
export function Standings({ standings, limit = DEFAULT_LIMIT }: StandingsProps) {
  const shown = standings.slice(0, limit);
  const hidden = standings.length - shown.length;

  return (
    <Card padding={0} style={{ overflow: "hidden" }}>
      <div
        style={{
          padding: "13px 16px",
          borderBottom: "1px solid var(--hairline)",
          fontFamily: "var(--font-mono)",
          fontSize: "var(--text-size-mono-label)",
          fontWeight: "var(--fw-medium)",
          textTransform: "uppercase",
          letterSpacing: "var(--tracking-eyebrow)",
          color: "var(--star-900)",
        }}
      >
        STANDINGS · SCENARIO WINS
      </div>
      {shown.map((s) => (
        <div
          key={s.providerId}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 11,
            padding: "11px 16px",
            borderTop: "1px solid var(--hairline-faint)",
          }}
        >
          <span
            style={{
              width: 16,
              font: "600 12px var(--font-mono)",
              color: s.leader ? "var(--gold-300)" : "var(--star-800)",
            }}
          >
            {s.rank}
          </span>
          {s.leader ? (
            <img src={starGlyphGold} alt="leader" style={{ width: 14, height: 14 }} />
          ) : (
            <span style={{ width: 14, height: 14, display: "inline-block", flex: "none" }} />
          )}
          <span
            style={{
              flex: 1,
              font: "600 13px var(--font-sans)",
              color: s.leader ? "var(--star-100)" : "var(--star-400)",
            }}
          >
            {s.label}
          </span>
          <span style={{ font: "500 12px var(--font-mono)", color: "var(--star-600)" }}>
            {s.wins} {s.wins === 1 ? "win" : "wins"}
          </span>
        </div>
      ))}
      {hidden > 0 && (
        <div
          style={{
            padding: "10px 16px",
            borderTop: "1px solid var(--hairline-faint)",
            font: "11px var(--font-mono)",
            color: "var(--star-800)",
          }}
        >
          and {hidden} more {hidden === 1 ? "contender" : "contenders"}
        </div>
      )}
    </Card>
  );
}
