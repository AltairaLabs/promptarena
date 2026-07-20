import { Card } from "@altairalabs/atlas";
import starGlyphGold from "@/assets/star-glyph-gold.svg";
import type { Standing } from "@/types";

export interface StandingsProps {
  standings: Standing[];
}

// Standings — a compact scenario-wins leaderboard derived from the trial
// matrix (`buildStandings`). The leader gets the gold star treatment that
// echoes the matrix's "best" cells.
export function Standings({ standings }: StandingsProps) {
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
      {standings.map((s) => (
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
    </Card>
  );
}
