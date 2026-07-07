import { act, render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { RunResult, RunOptionsResponse } from "@/types";

const mk = (o: Partial<RunResult>): RunResult => ({
  RunID: o.RunID ?? "r",
  PromptPack: "",
  Region: "default",
  ScenarioID: o.ScenarioID!,
  ProviderID: o.ProviderID!,
  Params: {},
  Messages: [],
  Commit: {},
  Cost: (o.Cost as RunResult["Cost"]) ?? {
    total_cost_usd: 0,
    input_tokens: 0,
    output_tokens: 0,
    input_cost_usd: 0,
    output_cost_usd: 0,
  },
  Violations: [],
  StartTime: o.StartTime ?? "2026-07-07T00:00:00Z",
  EndTime: o.EndTime ?? "2026-07-07T00:00:01Z",
  Duration: o.Duration ?? 1000,
  Error: o.Error ?? "",
  SelfPlay: false,
  PersonaID: "",
  MediaOutputs: [],
  A2AAgents: [],
  ...o,
});

const runOptions: RunOptionsResponse = {
  providers: [
    { id: "claude", type: "anthropic" },
    { id: "mock", type: "mock" },
  ],
  scenarios: [{ id: "checkout" }],
};

const seededResults: RunResult[] = [
  mk({
    RunID: "run-1",
    ScenarioID: "checkout",
    ProviderID: "claude",
    ConversationAssertions: { passed: true, failed: 0, total: 2, results: [] },
  }),
];

const getResults = vi.fn().mockResolvedValue(["run-1"]);
const getResult = vi.fn().mockImplementation((id: string) =>
  Promise.resolve(seededResults.find((r) => r.RunID === id)),
);
const getRunOptions = vi.fn().mockResolvedValue(runOptions);
const startRun = vi.fn().mockResolvedValue({});
const clearResults = vi.fn().mockResolvedValue({});

vi.mock("@/hooks/useArenaAPI", () => ({
  useArenaAPI: () => ({
    startRun,
    getResults,
    getResult,
    getRunOptions,
    clearResults,
    loading: false,
  }),
}));

vi.mock("@/hooks/useArenaEvents", () => ({
  useArenaEvents: () => ({
    registerInteractiveRun: vi.fn(),
    connected: true,
    runs: {},
    completedRunIds: ["run-1"],
    totalCost: 0,
    totalTokens: 0,
    logs: [],
  }),
}));

// Imported after the mocks above so App picks up the mocked hooks.
const { default: App } = await import("@/App");

describe("App — Runs view", () => {
  beforeEach(() => {
    getResults.mockClear();
    getResult.mockClear();
  });

  it("renders the trial matrix given seeded historical results", async () => {
    render(<App />);
    expect(await screen.findByText("TRIAL MATRIX · SCENARIO × PROVIDER")).toBeInTheDocument();
    expect(await screen.findByText("100%")).toBeInTheDocument();
  });

  it("hides HistoricalResults by default and shows it via the ledger toggle", async () => {
    render(<App />);
    await screen.findByText("TRIAL MATRIX · SCENARIO × PROVIDER");
    expect(screen.queryByText("Previous Runs")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /show ledger/i }));
    expect(await screen.findByText("Previous Runs")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /hide ledger/i }));
    expect(screen.queryByText("Previous Runs")).not.toBeInTheDocument();
  });

  it("opens RunDetail when a populated matrix cell is clicked", async () => {
    render(<App />);
    const rate = await screen.findByText("100%");
    fireEvent.click(rate);
    // RunDetail replaces the matrix view; the matrix's own heading disappears.
    expect(screen.queryByText("TRIAL MATRIX · SCENARIO × PROVIDER")).not.toBeInTheDocument();
    // Let RunDetail's own getResult() fetch settle before the test tears down,
    // so its state update lands inside act() instead of warning after the fact.
    await act(async () => {
      await Promise.resolve();
    });
  });
});
