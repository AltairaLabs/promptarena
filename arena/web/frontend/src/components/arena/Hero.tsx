export interface HeroProps {
  scenarioCount: number;
  providerCount: number;
}

// dateBadge formats "today" the way the eyebrow wants it: lowercase,
// abbreviated month + day (e.g. "jul 7") — no year, no leading zero.
function dateBadge(d: Date): string {
  const month = d.toLocaleString("en-US", { month: "short" }).toLowerCase();
  return `${month} ${d.getDate()}`;
}

function pluralize(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}.`;
}

// Hero — the Atlas redesign's page-top banner: a gold mono eyebrow, an H1
// whose payoff clause ("N contenders.") is gold, and a muted subhead.
// Replaces the old EmptyStateLauncher's role as the page's welcome band —
// it's shown whether or not there are runs yet.
export function Hero({ scenarioCount, providerCount }: HeroProps) {
  return (
    <section style={{ padding: "40px 0 24px" }}>
      <div
        style={{
          font: "500 12px var(--font-mono)",
          letterSpacing: "0.16em",
          textTransform: "uppercase",
          color: "var(--gold-500)",
          marginBottom: 14,
        }}
      >
        THE ARENA · CHARTED {dateBadge(new Date())}
      </div>
      <h1
        style={{
          font: "600 40px/1.05 var(--font-sans)",
          letterSpacing: "-0.025em",
          color: "var(--star-100)",
          maxWidth: 760,
          margin: "0 0 14px",
        }}
      >
        {pluralize(scenarioCount, "scenario")}{" "}
        <span style={{ color: "var(--gold-500)" }}>{pluralize(providerCount, "contender")}</span>
      </h1>
      <p
        style={{
          font: "400 16px/1.6 var(--font-sans)",
          color: "var(--star-600)",
          maxWidth: 600,
          margin: 0,
        }}
      >
        Every scenario charted against every contender — pass rates, cost, and latency in one view.
      </p>
    </section>
  );
}
