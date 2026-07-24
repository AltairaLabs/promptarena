export interface HeroProps {
  scenarioCount: number;
  providerCount: number;
  // chartedAt is the timestamp of the most recent charted run (ISO string).
  // The eyebrow's dateline reflects real data — when nothing has run yet it's
  // omitted entirely rather than fibbing "charted today".
  chartedAt?: string | null;
}

// dateBadge formats a run date the way the eyebrow wants it: lowercase,
// abbreviated month + day (e.g. "jul 7") — no year, no leading zero.
function dateBadge(d: Date): string {
  const month = d.toLocaleString("en-US", { month: "short" }).toLowerCase();
  return `${month} ${d.getDate()}`;
}

function pluralize(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}.`;
}

// Hero — the Atlas redesign's page-top banner: a gold mono eyebrow over an H1
// whose payoff clause ("N contenders.") is gold. Replaces the old
// EmptyStateLauncher's role as the page's welcome band — it's shown whether or
// not there are runs yet.
export function Hero({ scenarioCount, providerCount, chartedAt }: HeroProps) {
  const chartedDate = chartedAt ? new Date(chartedAt) : null;
  const eyebrow =
    chartedDate && !Number.isNaN(chartedDate.getTime())
      ? `THE ARENA · CHARTED ${dateBadge(chartedDate)}`
      : "THE ARENA";
  return (
    <section style={{ padding: "8px 0" }}>
      <div
        style={{
          font: "500 12px var(--font-mono)",
          letterSpacing: "0.16em",
          textTransform: "uppercase",
          color: "var(--gold-500)",
          marginBottom: 14,
        }}
      >
        {eyebrow}
      </div>
      <h1
        style={{
          font: "600 40px/1.05 var(--font-sans)",
          letterSpacing: "-0.025em",
          color: "var(--star-100)",
          maxWidth: 760,
          margin: 0,
        }}
      >
        {pluralize(scenarioCount, "scenario")}{" "}
        <span style={{ color: "var(--gold-500)" }}>{pluralize(providerCount, "contender")}</span>
      </h1>
    </section>
  );
}
