import { act, render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { RunResult, RunOptionsResponse, ActiveRun } from "@/types";

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

// useArenaEventsMock is a vi.fn() (rather than a fixed object) so individual
// tests can override `runs` to simulate in-flight/just-completed live runs.
const useArenaEventsMock = vi.fn();

vi.mock("@/hooks/useArenaEvents", () => ({
  useArenaEvents: () => useArenaEventsMock(),
}));

const defaultArenaState = () => ({
  registerInteractiveRun: vi.fn(),
  connected: true,
  runs: {} as Record<string, ActiveRun>,
  completedRunIds: ["run-1"],
  totalCost: 0,
  totalTokens: 0,
  logs: [],
});

// Imported after the mocks above so App picks up the mocked hooks.
const { default: App } = await import("@/App");

describe("App — Runs view", () => {
  beforeEach(() => {
    getResults.mockClear();
    getResult.mockClear();
    useArenaEventsMock.mockReset();
    useArenaEventsMock.mockReturnValue(defaultArenaState());
  });

  it("renders the trial matrix given seeded historical results", async () => {
    render(<App />);
    expect(await screen.findByText("TRIAL MATRIX · SCENARIO × PROVIDER")).toBeInTheDocument();
    expect(await screen.findByText("100%")).toBeInTheDocument();
  });

  it("renders the instrument band above the trial matrix", async () => {
    render(<App />);
    const gaugeLabel = await screen.findByText("PASS RATE · ALL TRIALS");
    const matrixHeading = await screen.findByText("TRIAL MATRIX · SCENARIO × PROVIDER");
    expect(gaugeLabel).toBeInTheDocument();
    expect(
      gaugeLabel.compareDocumentPosition(matrixHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
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

  it("opens the TrialInspector when a populated matrix cell is clicked", async () => {
    render(<App />);
    const rate = await screen.findByText("100%");
    fireEvent.click(rate);
    // TrialInspector replaces the matrix view; the matrix's own heading disappears.
    expect(screen.queryByText("TRIAL MATRIX · SCENARIO × PROVIDER")).not.toBeInTheDocument();
    // The inspector's transcript header, agent-flow graph, and terminal render
    // instead of the old RunDetail.
    expect(await screen.findByText("TRANSCRIPT")).toBeInTheDocument();
    expect(screen.getByText(/promptarena run --scenario checkout --provider claude/)).toBeInTheDocument();
    await act(async () => {
      await Promise.resolve();
    });
  });

  it("does not let a completed-but-not-yet-refetched live run mask a failing historical result", async () => {
    // The real, persisted result for run-1 failed one of its two assertions
    // (50% pass rate). Its EndTime is deliberately earlier than the live
    // run's startTime below, so a buggy overlay that includes completed
    // ActiveRuns would pick the synthetic entry as "latest" and read it as
    // a bare 100% pass (no ConversationAssertions + no Error on the
    // synthetic RunResult falls through to a full pass).
    const failingResult = mk({
      RunID: "run-1",
      ScenarioID: "checkout",
      ProviderID: "claude",
      EndTime: "2026-07-07T00:00:01Z",
      ConversationAssertions: { passed: false, failed: 1, total: 2, results: [] },
    });
    getResult.mockImplementationOnce((id: string) =>
      Promise.resolve(id === "run-1" ? failingResult : null),
    );
    // Simulate the window between the "completed" SSE event and the async
    // getResults() refetch: state.runs still holds the completed ActiveRun.
    useArenaEventsMock.mockReturnValue({
      ...defaultArenaState(),
      runs: {
        "run-1": {
          runId: "run-1",
          scenario: "checkout",
          provider: "claude",
          region: "default",
          startTime: "2026-07-07T00:00:05Z",
          turnIndex: 3,
          messages: [],
          costs: { inputTokens: 0, outputTokens: 0, totalCost: 0 },
          status: "completed",
        } satisfies ActiveRun,
      },
    });

    render(<App />);

    expect(await screen.findByText("50%")).toBeInTheDocument();
    expect(screen.queryByText("100%")).not.toBeInTheDocument();
  });
});
