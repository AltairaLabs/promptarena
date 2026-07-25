import { act, render, screen, fireEvent, waitFor } from "@testing-library/react";
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
    // 100% shows in both the matrix cell and the (always-visible) ledger batch.
    expect((await screen.findAllByText("100%")).length).toBeGreaterThan(0);
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

  it("shows the ledger by default, without a show/hide toggle", async () => {
    render(<App />);
    await screen.findByText("TRIAL MATRIX · SCENARIO × PROVIDER");
    expect(await screen.findByText("Ledger")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /show ledger/i })).not.toBeInTheDocument();
  });

  it("opens a run's transcript from the ledger", async () => {
    render(<App />);
    await screen.findByText("TRIAL MATRIX · SCENARIO × PROVIDER");
    // Open the seeded run from its ledger row (a passing run reads "Pass").
    fireEvent.click((await screen.findByText("Pass")).closest("button")!);
    // SessionReview replaces the dashboard; the matrix heading disappears.
    expect(screen.queryByText("TRIAL MATRIX · SCENARIO × PROVIDER")).not.toBeInTheDocument();
    expect(await screen.findByRole("button", { name: /^transcript$/i })).toBeInTheDocument();
    await act(async () => { await Promise.resolve(); });
  });

  it("steps through ledger runs with a position indicator, then highlights the row on return", async () => {
    const r1 = mk({
      RunID: "run-1", ScenarioID: "checkout", ProviderID: "claude", EndTime: "2026-07-07T00:00:03Z",
      ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] },
    });
    const r2 = mk({
      RunID: "run-2", ScenarioID: "checkout", ProviderID: "mock", EndTime: "2026-07-07T00:00:02Z",
      ConversationAssertions: { passed: false, failed: 1, total: 1, results: [] },
    });
    const r3 = mk({
      RunID: "run-3", ScenarioID: "checkout", ProviderID: "claude", EndTime: "2026-07-07T00:00:01Z",
      Error: "403",
    });
    const byId: Record<string, RunResult> = { "run-1": r1, "run-2": r2, "run-3": r3 };
    getResults.mockResolvedValueOnce(["run-1", "run-2", "run-3"]);
    getResult
      .mockImplementationOnce((id: string) => Promise.resolve(byId[id] ?? null))
      .mockImplementationOnce((id: string) => Promise.resolve(byId[id] ?? null))
      .mockImplementationOnce((id: string) => Promise.resolve(byId[id] ?? null));

    render(<App />);
    // Ledger is newest-first: run-1 (Pass), run-2 (Fail), run-3 (Error).
    // Open the middle row → "2 of 3".
    fireEvent.click((await screen.findByText("Fail")).closest("button")!);
    expect(await screen.findByText("2 of 3")).toBeInTheDocument();

    // Next advances to the last run → "3 of 3", and Next is then disabled.
    fireEvent.click(screen.getByRole("button", { name: /next run/i }));
    expect(await screen.findByText("3 of 3")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /next run/i })).toBeDisabled();

    // Back returns to the dashboard with that run's ledger row highlighted.
    fireEvent.click(screen.getByText(/Back to ledger/));
    const errorRow = (await screen.findByText("Error")).closest("button")!;
    expect(errorRow).toHaveAttribute("aria-current", "true");
  });

  it("a stale reload that resolves last does not clobber the full ledger", async () => {
    // Reproduces the ×N flood race: an early fetch (started when few runs
    // existed) resolving AFTER a later fetch must not overwrite the ledger.
    let resolveStale: (r: RunResult | null) => void = () => {};
    const stalePromise = new Promise<RunResult | null>((res) => { resolveStale = res; });
    const staleRun = mk({
      RunID: "stale", ScenarioID: "checkout", ProviderID: "claude",
      ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] }, // "Pass"
    });
    const r1 = mk({
      RunID: "r1", ScenarioID: "checkout", ProviderID: "claude",
      ConversationAssertions: { passed: false, failed: 1, total: 1, results: [] }, // "Fail"
    });
    const r2 = mk({ RunID: "r2", ScenarioID: "checkout", ProviderID: "mock", Error: "403" }); // "Error"
    const byId: Record<string, RunResult> = { r1, r2 };

    // First reload sees only the stale run (its getResult is deferred); the
    // second reload sees the real pair and resolves immediately.
    getResults.mockResolvedValueOnce(["stale"]).mockResolvedValueOnce(["r1", "r2"]);
    const resolveById = (id: string) =>
      id === "stale" ? stalePromise : Promise.resolve(byId[id] ?? null);
    getResult
      .mockImplementationOnce(resolveById)
      .mockImplementationOnce(resolveById)
      .mockImplementationOnce(resolveById);

    const { rerender } = render(<App />);
    // Let the first (debounced) reload actually fire and go in-flight on the
    // stale run — only then is there a real in-flight fetch to be superseded.
    await waitFor(() => expect(getResult).toHaveBeenCalledWith("stale"));

    // More runs complete → a later reload supersedes the first.
    useArenaEventsMock.mockReturnValue({ ...defaultArenaState(), completedRunIds: ["run-1", "run-2"] });
    rerender(<App />);

    // The second reload lands: ledger shows the real pair.
    expect(await screen.findByText("Fail")).toBeInTheDocument();
    expect(screen.getByText("Error")).toBeInTheDocument();

    // Now the stale first reload finally resolves — it must be ignored.
    await act(async () => { resolveStale(staleRun); await Promise.resolve(); });

    expect(screen.getByText("Fail")).toBeInTheDocument();
    expect(screen.getByText("Error")).toBeInTheDocument();
    expect(screen.queryByText("Pass")).not.toBeInTheDocument(); // stale run did not clobber
  });

  it("only fetches newly-completed runs on reload (immutable cache, no refetch storm)", async () => {
    const r1 = mk({
      RunID: "run-1", ScenarioID: "checkout", ProviderID: "claude",
      ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] },
    });
    const r2 = mk({
      RunID: "run-2", ScenarioID: "checkout", ProviderID: "mock",
      ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] },
    });
    const byId: Record<string, RunResult> = { "run-1": r1, "run-2": r2 };
    getResults.mockResolvedValueOnce(["run-1"]).mockResolvedValueOnce(["run-1", "run-2"]);
    getResult
      .mockImplementationOnce((id: string) => Promise.resolve(byId[id] ?? null))
      .mockImplementationOnce((id: string) => Promise.resolve(byId[id] ?? null));

    const { rerender } = render(<App />);
    await waitFor(() => expect(getResult).toHaveBeenCalledWith("run-1"));

    // A new run completes → reload sees ["run-1","run-2"] but only fetches run-2.
    useArenaEventsMock.mockReturnValue({ ...defaultArenaState(), completedRunIds: ["run-1", "run-2"] });
    rerender(<App />);
    await waitFor(() => expect(getResult).toHaveBeenCalledWith("run-2"));

    // run-1 was fetched exactly once — the cache means it's never refetched.
    expect(getResult.mock.calls.filter((c) => c[0] === "run-1")).toHaveLength(1);
  });

  it("clicking a matrix cell scopes the ledger to that scenario×provider", async () => {
    render(<App />);
    // Clicking an aggregate cell filters the ledger to its runs — it does NOT
    // open a single run, and stays on the dashboard.
    fireEvent.click((await screen.findAllByText("100%"))[0]);
    expect(await screen.findByText(/checkout × claude/)).toBeInTheDocument();
    expect(screen.getByText("TRIAL MATRIX · SCENARIO × PROVIDER")).toBeInTheDocument();
  });

  it("fetches the workflow graph on mount and offers it as a Workflow tab", async () => {
    render(<App />);
    await screen.findByText("TRIAL MATRIX · SCENARIO × PROVIDER");
    fireEvent.click((await screen.findByText("Pass")).closest("button")!);
    await screen.findByRole("button", { name: /^transcript$/i });
    expect(getWorkflow).toHaveBeenCalled();
    expect(await screen.findByRole("button", { name: /^workflow$/i })).toBeInTheDocument();
  });

  it("a run that failed an assertion aggregates to 0% (a failed run, not its proportion)", async () => {
    const failingResult = mk({
      RunID: "run-1", ScenarioID: "checkout", ProviderID: "claude",
      ConversationAssertions: { passed: false, failed: 1, total: 2, results: [] },
    });
    getResult.mockImplementationOnce((id: string) =>
      Promise.resolve(id === "run-1" ? failingResult : null),
    );

    render(<App />);
    // 0 of 1 runs passed → 0% (never the 50% assertion proportion).
    expect(await screen.findByText("0%")).toBeInTheDocument();
    expect(screen.queryByText("100%")).not.toBeInTheDocument();
  });

  it("clicking a transcript message opens the SessionReview Inspector", async () => {
    const runWithMessages = mk({
      RunID: "run-1", ScenarioID: "checkout", ProviderID: "claude",
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
    await screen.findByText("TRIAL MATRIX · SCENARIO × PROVIDER");
    fireEvent.click((await screen.findByText("Pass")).closest("button")!);
    await screen.findByRole("button", { name: /^transcript$/i });

    expect(screen.queryByText("Overview")).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("Hello!"));
    expect(await screen.findByText("Overview")).toBeInTheDocument();
  });

  it("renders Hero + CommandStrip above the instrument band and trial matrix", async () => {
    render(<App />);
    const commandStripLabel = await screen.findByText("CHART A RUN");
    const gaugeLabel = await screen.findByText("PASS RATE · ALL TRIALS");
    const matrixHeading = await screen.findByText("TRIAL MATRIX · SCENARIO × PROVIDER");
    // The dateline reflects the latest completed run, which loads async.
    expect(await screen.findByText(/^THE ARENA · CHARTED /)).toBeInTheDocument();
    expect(
      commandStripLabel.compareDocumentPosition(gaugeLabel) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      gaugeLabel.compareDocumentPosition(matrixHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("selecting a scenario chip updates the CommandStrip readout to that scenario against the whole field", async () => {
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
    // All scenarios are selected by default → the readout counts them.
    expect(await screen.findByText("2 scenarios · 2 contenders = 4 trials")).toBeInTheDocument();

    // Toggling a pill off drops it from the selection.
    fireEvent.click(screen.getByRole("button", { name: "refund" }));
    expect(await screen.findByText("1 scenario · 2 contenders = 2 trials")).toBeInTheDocument();
  });

  it("CommandStrip's Run the field runs every selected scenario across ALL providers", async () => {
    render(<App />);
    await screen.findByText("CHART A RUN");
    // Wait for the all-scenarios default to seed (a useEffect once options
    // load) — otherwise the button is disabled and the click is a no-op.
    await screen.findByText("1 scenario · 2 contenders = 2 trials");

    fireEvent.click(screen.getByText(/Run the field/));

    expect(startRun).toHaveBeenCalledWith({ providers: ["claude", "mock"], scenarios: ["checkout"], runs: 1 });
  });

  it("clicking an empty matrix cell starts a run for just that scenario+provider", async () => {
    render(<App />);
    // "mock" has no result for "checkout" in seededResults, so its cell is empty.
    const runCellButton = await screen.findByRole("button", { name: "Run checkout on mock" });
    fireEvent.click(runCellButton);

    expect(startRun).toHaveBeenCalledWith({ providers: ["mock"], scenarios: ["checkout"], runs: 1 });
  });
});
