import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { InstrumentBand } from "./InstrumentBand";
import { buildMatrix } from "@/lib/arenaView";
import type { RunResult } from "@/types";

const providers = [
  { id: "claude", label: "claude" },
  { id: "gpt4o", label: "gpt4o" },
];
const scenarios = [{ id: "checkout", label: "checkout" }];

function mk(o: Partial<RunResult> & { RunID: string; StartTime: string; EndTime: string }): RunResult {
  return {
    PromptPack: "",
    Region: "default",
    ScenarioID: "checkout",
    ProviderID: "claude",
    Params: {},
    Messages: [],
    Commit: {},
    Cost: { total_cost_usd: 0, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 },
    Violations: [],
    Duration: 100,
    Error: "",
    SelfPlay: false,
    PersonaID: "",
    MediaOutputs: [],
    A2AAgents: [],
    ...o,
  };
}

// A full-field sweep: one batch (shared timestamp prefix) covering both cells.
function sweep(day: string, claudePass: boolean, gptPass: boolean): RunResult[] {
  const cell = (provider: string, pass: boolean) =>
    mk({
      RunID: `2026-07-${day}T00-00-00Z_${provider}_default_checkout_1`,
      ProviderID: provider,
      StartTime: `2026-07-${day}T00:00:00Z`,
      EndTime: `2026-07-${day}T00:00:01Z`,
      ConversationAssertions: { passed: pass, failed: pass ? 0 : 1, total: 1, results: [] },
    });
  return [cell("claude", claudePass), cell("gpt4o", gptPass)];
}

describe("InstrumentBand", () => {
  it("renders the gauge readout, metric labels, and standings rows; trail hidden with only one field run", () => {
    // Both runs share one batch prefix → a single field run → buildSuiteTrend
    // returns [] (no trend from one run), so the sub-block must not render.
    const results: RunResult[] = [
      mk({
        RunID: "2026-07-01T00-00-00Z_claude_default_checkout_h_1",
        ProviderID: "claude",
        StartTime: "2026-07-01T00:00:00Z",
        EndTime: "2026-07-01T00:00:01Z",
        ConversationAssertions: { passed: true, failed: 0, total: 2, results: [] },
      }),
      mk({
        RunID: "2026-07-01T00-00-00Z_gpt4o_default_checkout_h_2",
        ProviderID: "gpt4o",
        StartTime: "2026-07-02T00:00:00Z",
        EndTime: "2026-07-02T00:00:01Z",
        ConversationAssertions: { passed: false, failed: 1, total: 2, results: [] },
      }),
    ];
    const matrix = buildMatrix(results, providers, scenarios);

    render(<InstrumentBand matrix={matrix} results={results} />);

    // Gauge: 1 of 2 runs passed => 50% readout.
    expect(screen.getByText("50")).toBeInTheDocument();
    expect(screen.getByText("1 / 2 trials passed")).toBeInTheDocument();

    // InstrumentReadout metric labels (buildMetrics: TRIALS, SPEND, P50, TOKENS).
    expect(screen.getByText("TRIALS")).toBeInTheDocument();
    expect(screen.getByText("SPEND")).toBeInTheDocument();
    expect(screen.getByText("P50")).toBeInTheDocument();
    expect(screen.getByText("TOKENS")).toBeInTheDocument();

    // Standings rows for both providers.
    expect(screen.getByText("claude")).toBeInTheDocument();
    expect(screen.getByText("gpt4o")).toBeInTheDocument();

    // Trend sub-block absent: no header, no polyline.
    expect(screen.queryByText("SUITE PASS RATE · LAST 12 RUNS")).not.toBeInTheDocument();
    expect(document.querySelector("polyline")).toBeNull();
  });

  it("shows the star-trail sub-block once there are ≥2 full sweeps", () => {
    const results = [...sweep("01", true, true), ...sweep("02", false, true)];
    const matrix = buildMatrix(results, providers, scenarios);

    render(<InstrumentBand matrix={matrix} results={results} />);

    expect(screen.getByText("SUITE PASS RATE · PER SWEEP")).toBeInTheDocument();
    expect(document.querySelector("polyline")).toBeInTheDocument();
  });

  it("colors a negative trend delta red and a positive delta healthy", () => {
    // declining: sweep 100% → sweep 0%
    const declining = [...sweep("01", true, true), ...sweep("02", false, false)];
    const { unmount } = render(
      <InstrumentBand matrix={buildMatrix(declining, providers, scenarios)} results={declining} />,
    );
    expect(screen.getByText(/^▼/)).toHaveStyle({ color: "var(--signal-red-300)" });
    unmount();

    // improving: sweep 0% → sweep 100%
    const improving = [...sweep("01", false, false), ...sweep("02", true, true)];
    render(<InstrumentBand matrix={buildMatrix(improving, providers, scenarios)} results={improving} />);
    // Healthy ramp, not gold. Gold means "the one thing that matters on this
    // view", not "good" — the band spends its single gold on the trail's key
    // star (see InstrumentBand's StarTrail keyIndex).
    expect(screen.getByText(/^▲/)).toHaveStyle({ color: "var(--status-healthy-text)" });
  });

  // Atlas 0.6.0 made gold opt-in so a view spends it once, deliberately. This
  // band asks for exactly one: the trail's latest-point key star. The gauge
  // takes no color prop (starlight) and the delta text uses the healthy ramp,
  // so if a future change re-gilds either, this count catches it.
  it("spends exactly one gold moment, on the trend trail's key star", () => {
    const improving = [...sweep("01", false, false), ...sweep("02", true, true)];
    const { container } = render(
      <InstrumentBand matrix={buildMatrix(improving, providers, scenarios)} results={improving} />,
    );

    const keyStars = container.querySelectorAll(".atlas-keystar");
    expect(keyStars).toHaveLength(1);
  });
});
