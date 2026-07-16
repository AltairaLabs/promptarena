import { act, render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { RunResult, RunOptionsResponse, ActiveRun, WorkflowGraph } from "@/types";

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
const getConfig = vi.fn().mockResolvedValue({});
const startRun = vi.fn().mockResolvedValue({});
const clearResults = vi.fn().mockResolvedValue({});

const seededWorkflowGraph: WorkflowGraph = {
  nodes: [
    { id: "intake", label: "intake", kind: "entry", entry: true, terminal: false },
    { id: "resolve", label: "resolve", kind: "output", entry: false, terminal: true },
  ],
  edges: [{ from: "intake", to: "resolve" }],
};
const getWorkflow = vi.fn().mockResolvedValue(seededWorkflowGraph);

vi.mock("@/hooks/useArenaAPI", () => ({
  useArenaAPI: () => ({
    startRun,
    getResults,
    getResult,
    getConfig,
    getRunOptions,
    clearResults,
    getWorkflow,
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
    getWorkflow.mockClear();
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

  it("opens SessionReview when a populated matrix cell is clicked", async () => {
    render(<App />);
    const rate = await screen.findByText("100%");
    fireEvent.click(rate);
    // SessionReview replaces the matrix view; the matrix heading disappears.
    expect(screen.queryByText("TRIAL MATRIX · SCENARIO × PROVIDER")).not.toBeInTheDocument();
    // SessionReview's Transcript tab renders instead of the old TrialInspector.
    expect(await screen.findByRole("button", { name: /^transcript$/i })).toBeInTheDocument();
    await act(async () => {
      await Promise.resolve();
    });
  });

  it("fetches the workflow graph on mount and offers it as a Workflow tab", async () => {
    render(<App />);
    const rate = await screen.findByText("100%");
    fireEvent.click(rate);
    await screen.findByRole("button", { name: /^transcript$/i });

    expect(getWorkflow).toHaveBeenCalled();
    expect(await screen.findByRole("button", { name: /^workflow$/i })).toBeInTheDocument();
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

  it("clicking a transcript message opens the SessionReview Inspector", async () => {
    const runWithMessages = mk({
      RunID: "run-1",
      ScenarioID: "checkout",
      ProviderID: "claude",
      Messages: [
        { role: "user", content: "Hi" },
        { role: "assistant", content: "Hello!" },
      ],
      ConversationAssertions: { passed: true, failed: 0, total: 2, results: [] },
    });
    getResult.mockImplementationOnce((id: string) =>
      Promise.resolve(id === "run-1" ? runWithMessages : null),
    );

    render(<App />);
    const rate = await screen.findByText("100%");
    fireEvent.click(rate);
    await screen.findByRole("button", { name: /^transcript$/i });

    expect(screen.queryByText("Overview")).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("Hello!"));
    expect(await screen.findByText("Overview")).toBeInTheDocument();
  });

  it("selecting an OLDER run from the ledger shows that run's transcript, not the newer run pinned to the matrix cell", async () => {
    const olderFailingRun = mk({
      RunID: "run-old",
      ScenarioID: "checkout",
      ProviderID: "claude",
      StartTime: "2026-07-01T00:00:00Z",
      EndTime: "2026-07-01T00:00:01Z",
      Messages: [{ role: "assistant", content: "older-answer" }],
      Cost: { total_cost_usd: 0.01, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 },
      Duration: 500,
      ConversationAssertions: { passed: false, failed: 1, total: 2, results: [] },
    });
    const newerPassingRun = mk({
      RunID: "run-new",
      ScenarioID: "checkout",
      ProviderID: "claude",
      StartTime: "2026-07-07T00:00:00Z",
      EndTime: "2026-07-07T00:00:01Z",
      Messages: [{ role: "assistant", content: "newer-answer" }],
      Cost: { total_cost_usd: 0.02, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 },
      Duration: 900,
      ConversationAssertions: { passed: true, failed: 0, total: 2, results: [] },
    });
    getResults.mockResolvedValueOnce(["run-old", "run-new"]);
    getResult.mockImplementationOnce(() => Promise.resolve(olderFailingRun));
    getResult.mockImplementationOnce(() => Promise.resolve(newerPassingRun));

    render(<App />);
    // The matrix cell aggregates to the latest (passing) run — confirm the
    // dashboard is up before diving into the ledger.
    await screen.findByText("TRIAL MATRIX · SCENARIO × PROVIDER");

    fireEvent.click(screen.getByRole("button", { name: /show ledger/i }));
    await screen.findByText("Previous Runs");

    // Only the older run failed, so "Fail" uniquely identifies its row.
    fireEvent.click(screen.getByText("Fail"));

    expect(await screen.findByText("older-answer")).toBeInTheDocument();
    expect(screen.queryByText("newer-answer")).not.toBeInTheDocument();
  });

  it("renders Hero + CommandStrip above the instrument band and trial matrix", async () => {
    render(<App />);
    const commandStripLabel = await screen.findByText("CHART A RUN");
    const gaugeLabel = await screen.findByText("PASS RATE · ALL TRIALS");
    const matrixHeading = await screen.findByText("TRIAL MATRIX · SCENARIO × PROVIDER");
    expect(screen.getByText(/^THE ARENA · CHARTED /)).toBeInTheDocument();
    expect(
      commandStripLabel.compareDocumentPosition(gaugeLabel) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      gaugeLabel.compareDocumentPosition(matrixHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("selecting a scenario chip updates the CommandStrip provider label to that scenario's best provider", async () => {
    getRunOptions.mockResolvedValueOnce({
      providers: [
        { id: "claude", type: "anthropic" },
        { id: "mock", type: "mock" },
      ],
      scenarios: [{ id: "checkout" }, { id: "refund" }],
    });
    const checkoutRun = mk({
      RunID: "run-checkout",
      ScenarioID: "checkout",
      ProviderID: "claude",
      ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] },
    });
    const refundRun = mk({
      RunID: "run-refund",
      ScenarioID: "refund",
      ProviderID: "mock",
      ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] },
    });
    getResults.mockResolvedValueOnce(["run-checkout", "run-refund"]);
    getResult.mockImplementationOnce((id: string) =>
      Promise.resolve(id === "run-checkout" ? checkoutRun : null),
    );
    getResult.mockImplementationOnce((id: string) =>
      Promise.resolve(id === "run-refund" ? refundRun : null),
    );

    render(<App />);
    await screen.findByText("CHART A RUN");
    expect(await screen.findByText("claude · checkout")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "refund" }));
    expect(await screen.findByText("mock · refund")).toBeInTheDocument();
  });

  it("CommandStrip's Run trial starts the selected scenario across ALL providers", async () => {
    render(<App />);
    await screen.findByText("CHART A RUN");
    // Wait for selectedScenario's async default (set in a useEffect once
    // scenarios load) to land — otherwise the button is still disabled and
    // the click is a no-op, same race any other "disabled until ready"
    // button in this suite has to wait out.
    await screen.findByText("claude · checkout");

    // TopBar no longer renders a Run trial button — CommandStrip is the only
    // one left, so this is unambiguous.
    fireEvent.click(screen.getByText(/Run trial/));

    expect(startRun).toHaveBeenCalledWith({ providers: ["claude", "mock"], scenarios: ["checkout"] });
  });

  it("clicking an empty matrix cell starts a run for just that scenario+provider", async () => {
    render(<App />);
    // "mock" has no result for "checkout" in seededResults, so its cell is empty.
    const runCellButton = await screen.findByRole("button", { name: "Run checkout on mock" });
    fireEvent.click(runCellButton);

    expect(startRun).toHaveBeenCalledWith({ providers: ["mock"], scenarios: ["checkout"] });
  });
});
