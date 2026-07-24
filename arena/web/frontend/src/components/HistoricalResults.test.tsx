import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { HistoricalResults } from "./HistoricalResults";
import type { RunResult } from "@/types";

const mk = (runId: string, scenario: string, provider: string, o: Partial<RunResult> = {}): RunResult =>
  ({
    RunID: runId, PromptPack: "", Region: "", ScenarioID: scenario, ProviderID: provider,
    Params: {}, Messages: [], Commit: {},
    Cost: { total_cost_usd: 0.01, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 },
    Violations: [], StartTime: "2026-07-23T10:00:00Z", EndTime: "2026-07-23T10:00:01Z",
    Duration: 1000, Error: "", SelfPlay: false, PersonaID: "", MediaOutputs: [], A2AAgents: [],
    ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] },
    ...o,
  }) as RunResult;

const results = [
  mk("run-pass", "checkout", "claude"),
  mk("run-fail", "refund", "gpt4o", { ConversationAssertions: { passed: false, failed: 1, total: 1, results: [] } }),
  mk("run-err", "checkout", "gpt4o", { Error: "403", ConversationAssertions: undefined as never }),
];

function setup(overrides = {}) {
  const props = { results, onSelectRun: vi.fn(), onClear: vi.fn(), ...overrides };
  render(<HistoricalResults {...props} />);
  return props;
}

describe("HistoricalResults (ledger of individual runs)", () => {
  it("lists one row per run with its outcome", () => {
    setup();
    expect(screen.getByText("Pass")).toBeInTheDocument();
    expect(screen.getByText("Fail")).toBeInTheDocument();
    expect(screen.getByText("Error")).toBeInTheDocument();
  });

  it("opens a run's transcript when its row is clicked", () => {
    const p = setup();
    fireEvent.click(screen.getByText("Fail").closest("button")!);
    expect(p.onSelectRun).toHaveBeenCalledWith("run-fail");
  });

  it("collapses the run list without removing the ledger", () => {
    setup();
    fireEvent.click(screen.getByText("Ledger"));
    expect(screen.queryByText("Pass")).not.toBeInTheDocument();
    expect(screen.getByText("Ledger")).toBeInTheDocument();
  });

  it("clears all runs", () => {
    const p = setup();
    fireEvent.click(screen.getByText("Clear all"));
    expect(p.onClear).toHaveBeenCalledTimes(1);
  });

  it("shows an empty state when there are no runs", () => {
    setup({ results: [] });
    expect(screen.getByText("No runs yet.")).toBeInTheDocument();
  });

  it("filters rows by outcome", () => {
    setup();
    fireEvent.change(screen.getByLabelText("filter by outcome"), { target: { value: "fail" } });
    expect(screen.getByText("Fail")).toBeInTheDocument();
    expect(screen.queryByText("Pass")).not.toBeInTheDocument();
    expect(screen.queryByText("Error")).not.toBeInTheDocument();
  });

  it("filters rows by scenario", () => {
    setup();
    fireEvent.change(screen.getByLabelText("filter by scenario"), { target: { value: "refund" } });
    // Only the refund run (run-fail) survives.
    expect(screen.getByText("Fail")).toBeInTheDocument();
    expect(screen.queryByText("Pass")).not.toBeInTheDocument();
    expect(screen.queryByText("Error")).not.toBeInTheDocument();
  });

  it("paginates when there are more than a page of rows", () => {
    const many = Array.from({ length: 30 }, (_, i) => mk(`run-${i}`, "checkout", "claude"));
    setup({ results: many });
    // First page caps at PAGE_SIZE (25) rows.
    expect(screen.getAllByText("Pass")).toHaveLength(25);
    expect(screen.getByText(/1.25 of 30/)).toBeInTheDocument();
    // Next reveals the remainder.
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getByText(/26.30 of 30/)).toBeInTheDocument();
    expect(screen.getAllByText("Pass")).toHaveLength(5);
  });

  it("highlights the currently-open run's row via aria-current", () => {
    setup({ selectedRunId: "run-fail" });
    expect(screen.getByText("Fail").closest("button")).toHaveAttribute("aria-current", "true");
    expect(screen.getByText("Pass").closest("button")).not.toHaveAttribute("aria-current");
  });

  it("scopes to a scenario×provider when filtered, and offers to clear the filter", () => {
    const onClearFilter = vi.fn();
    setup({ filter: { scenarioId: "checkout", providerId: "gpt4o" }, onClearFilter });
    // Only the checkout×gpt4o run (the errored one) is shown.
    expect(screen.getByText("Error")).toBeInTheDocument();
    expect(screen.queryByText("Pass")).not.toBeInTheDocument();
    fireEvent.click(screen.getByText(/checkout × gpt4o/));
    expect(onClearFilter).toHaveBeenCalledTimes(1);
  });
});
