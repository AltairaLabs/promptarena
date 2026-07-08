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

describe("InstrumentBand", () => {
  it("renders the gauge readout, metric labels, and standings rows; trail hidden when trend is empty", () => {
    // Only 2 results — buildTrend returns [] (D1: fewer than 3 results is not
    // enough history to plot a trail), so the sub-block must not render.
    const results: RunResult[] = [
      mk({
        RunID: "r1",
        ProviderID: "claude",
        StartTime: "2026-07-01T00:00:00Z",
        EndTime: "2026-07-01T00:00:01Z",
        ConversationAssertions: { passed: true, failed: 0, total: 2, results: [] },
      }),
      mk({
        RunID: "r2",
        ProviderID: "gpt4o",
        StartTime: "2026-07-02T00:00:00Z",
        EndTime: "2026-07-02T00:00:01Z",
        ConversationAssertions: { passed: false, failed: 1, total: 2, results: [] },
      }),
    ];
    const matrix = buildMatrix(results, providers, scenarios);

    render(<InstrumentBand matrix={matrix} results={results} />);

    // Gauge: 1 of 2 cells passed => 50% readout.
    expect(screen.getByText("50")).toBeInTheDocument();
    expect(screen.getByText("1 / 2 passed")).toBeInTheDocument();

    // InstrumentReadout metric labels (buildMetrics: TRIALS, SPEND, P50, TOKENS).
    expect(screen.getByText("TRIALS")).toBeInTheDocument();
    expect(screen.getByText("SPEND")).toBeInTheDocument();
    expect(screen.getByText("P50")).toBeInTheDocument();
    expect(screen.getByText("TOKENS")).toBeInTheDocument();

    // Standings rows for both providers.
    expect(screen.getByText("claude")).toBeInTheDocument();
    expect(screen.getByText("gpt4o")).toBeInTheDocument();

    // Trend sub-block absent: no header, no polyline.
    expect(screen.queryByText("PASS RATE · LAST 12 RUNS")).not.toBeInTheDocument();
    expect(document.querySelector("polyline")).toBeNull();
  });

  it("shows the star-trail sub-block once there are 3+ results", () => {
    const results: RunResult[] = [
      mk({
        RunID: "r1",
        ProviderID: "claude",
        StartTime: "2026-07-01T00:00:00Z",
        EndTime: "2026-07-01T00:00:01Z",
        ConversationAssertions: { passed: true, failed: 0, total: 2, results: [] },
      }),
      mk({
        RunID: "r2",
        ProviderID: "gpt4o",
        StartTime: "2026-07-02T00:00:00Z",
        EndTime: "2026-07-02T00:00:01Z",
        ConversationAssertions: { passed: false, failed: 1, total: 2, results: [] },
      }),
      mk({
        RunID: "r3",
        ProviderID: "claude",
        StartTime: "2026-07-03T00:00:00Z",
        EndTime: "2026-07-03T00:00:01Z",
        ConversationAssertions: { passed: true, failed: 0, total: 2, results: [] },
      }),
    ];
    const matrix = buildMatrix(results, providers, scenarios);

    render(<InstrumentBand matrix={matrix} results={results} />);

    expect(screen.getByText("PASS RATE · LAST 12 RUNS")).toBeInTheDocument();
    expect(document.querySelector("polyline")).toBeInTheDocument();
  });

  it("labels the trail LAST 12 RUNS matching the buildTrend(results, 12) bucket size", () => {
    const results: RunResult[] = [
      mk({ RunID: "r1", StartTime: "2026-07-01T00:00:00Z", EndTime: "2026-07-01T00:00:01Z" }),
      mk({ RunID: "r2", StartTime: "2026-07-02T00:00:00Z", EndTime: "2026-07-02T00:00:01Z" }),
      mk({ RunID: "r3", StartTime: "2026-07-03T00:00:00Z", EndTime: "2026-07-03T00:00:01Z" }),
    ];
    const matrix = buildMatrix(results, providers, scenarios);
    render(<InstrumentBand matrix={matrix} results={results} />);
    expect(screen.getByText("PASS RATE · LAST 12 RUNS")).toBeInTheDocument();
  });

  it("colors a negative trend delta red and a positive delta gold", () => {
    const declining: RunResult[] = [
      mk({
        RunID: "r1",
        StartTime: "2026-07-01T00:00:00Z",
        EndTime: "2026-07-01T00:00:01Z",
        ConversationAssertions: { passed: true, failed: 0, total: 2, results: [] },
      }),
      mk({
        RunID: "r2",
        StartTime: "2026-07-02T00:00:00Z",
        EndTime: "2026-07-02T00:00:01Z",
        ConversationAssertions: { passed: true, failed: 0, total: 2, results: [] },
      }),
      mk({
        RunID: "r3",
        StartTime: "2026-07-03T00:00:00Z",
        EndTime: "2026-07-03T00:00:01Z",
        ConversationAssertions: { passed: false, failed: 2, total: 2, results: [] },
      }),
    ];
    const decliningMatrix = buildMatrix(declining, providers, scenarios);
    const { unmount } = render(<InstrumentBand matrix={decliningMatrix} results={declining} />);
    const downDelta = screen.getByText(/^▼/);
    expect(downDelta).toHaveStyle({ color: "var(--signal-red-300)" });
    unmount();

    const improving: RunResult[] = [
      mk({
        RunID: "r1",
        StartTime: "2026-07-01T00:00:00Z",
        EndTime: "2026-07-01T00:00:01Z",
        ConversationAssertions: { passed: false, failed: 2, total: 2, results: [] },
      }),
      mk({
        RunID: "r2",
        StartTime: "2026-07-02T00:00:00Z",
        EndTime: "2026-07-02T00:00:01Z",
        ConversationAssertions: { passed: true, failed: 0, total: 2, results: [] },
      }),
      mk({
        RunID: "r3",
        StartTime: "2026-07-03T00:00:00Z",
        EndTime: "2026-07-03T00:00:01Z",
        ConversationAssertions: { passed: true, failed: 0, total: 2, results: [] },
      }),
    ];
    const improvingMatrix = buildMatrix(improving, providers, scenarios);
    render(<InstrumentBand matrix={improvingMatrix} results={improving} />);
    const upDelta = screen.getByText(/^▲/);
    expect(upDelta).toHaveStyle({ color: "var(--gold-300)" });
  });
});
