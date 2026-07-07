import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { TrialInspector } from "./TrialInspector";
import type { RunResult, ActiveRun, TrialCell } from "@/types";

function makeCell(overrides: Partial<TrialCell> = {}): TrialCell {
  return {
    scenarioId: "checkout",
    providerId: "claude",
    key: "checkout:claude",
    passRate: 100,
    passed: true,
    best: false,
    costUsd: 0.0041,
    latencyMs: 820,
    runId: "r1",
    hasData: true,
    ...overrides,
  };
}

function makeRun(overrides: Partial<RunResult> = {}): RunResult {
  return {
    RunID: "r1",
    PromptPack: "",
    Region: "us",
    ScenarioID: "checkout",
    ProviderID: "claude",
    Params: {},
    Messages: [
      { role: "user", content: "Hi" },
      { role: "assistant", content: "Hello!" },
    ],
    Commit: {},
    Cost: {
      input_tokens: 10,
      output_tokens: 20,
      input_cost_usd: 0.001,
      output_cost_usd: 0.003,
      total_cost_usd: 0.0041,
    },
    Violations: [],
    StartTime: "2026-01-01T00:00:00Z",
    EndTime: "2026-01-01T00:00:01Z",
    Duration: 820,
    Error: "",
    SelfPlay: false,
    PersonaID: "",
    MediaOutputs: [],
    A2AAgents: [],
    ...overrides,
  };
}

describe("TrialInspector", () => {
  it("renders the transcript, agent-flow svg, terminal, and a Passed status pill", () => {
    const { container } = render(
      <TrialInspector
        run={makeRun()}
        cell={makeCell()}
        scenarioId="checkout"
        providerId="claude"
        providerLabel="Claude"
      />,
    );
    expect(screen.getByText("TRANSCRIPT")).toBeInTheDocument();
    expect(screen.getByText("checkout")).toBeInTheDocument();
    expect(screen.getByText("· Claude")).toBeInTheDocument();
    expect(screen.getByText("Hello!")).toBeInTheDocument();
    expect(container.querySelector("svg")).toBeTruthy();
    expect(screen.getByText(/promptarena run --scenario checkout --provider claude/)).toBeInTheDocument();
    expect(screen.getByText("Passed")).toBeInTheDocument();
  });

  it("renders a Failed status pill when the cell did not pass", () => {
    render(
      <TrialInspector
        run={makeRun({ Error: "boom" })}
        cell={makeCell({ passed: false, passRate: 0 })}
        scenarioId="checkout"
        providerId="claude"
        providerLabel="Claude"
      />,
    );
    expect(screen.getByText("Failed")).toBeInTheDocument();
  });

  it("renders a Running status pill for an in-flight ActiveRun", () => {
    const run: ActiveRun = {
      runId: "r2",
      scenario: "checkout",
      provider: "claude",
      region: "us",
      startTime: "2026-01-01T00:00:00Z",
      turnIndex: 1,
      messages: [{ role: "user", content: "Hi", index: 0 }],
      costs: { inputTokens: 5, outputTokens: 0, totalCost: 0 },
      status: "running",
    };
    render(
      <TrialInspector
        run={run}
        cell={makeCell({ hasData: false, runId: "r2" })}
        scenarioId="checkout"
        providerId="claude"
        providerLabel="Claude"
      />,
    );
    expect(screen.getByText("Running")).toBeInTheDocument();
  });

  it("calls onSelectMessage with the index and Message for a clicked transcript message on a RunResult run", () => {
    const onSelectMessage = vi.fn();
    const run = makeRun();
    render(
      <TrialInspector
        run={run}
        cell={makeCell()}
        scenarioId="checkout"
        providerId="claude"
        providerLabel="Claude"
        onSelectMessage={onSelectMessage}
      />,
    );
    fireEvent.click(screen.getByText("Hello!"));
    expect(onSelectMessage).toHaveBeenCalledWith(1, run.Messages[1], run.Messages);
  });

  it("calls onSelectMessage with just the index for a clicked transcript message on an ActiveRun (no Message[] to offer)", () => {
    const onSelectMessage = vi.fn();
    const run: ActiveRun = {
      runId: "r2",
      scenario: "checkout",
      provider: "claude",
      region: "us",
      startTime: "2026-01-01T00:00:00Z",
      turnIndex: 1,
      messages: [{ role: "user", content: "Hi", index: 0 }],
      costs: { inputTokens: 5, outputTokens: 0, totalCost: 0 },
      status: "running",
    };
    render(
      <TrialInspector
        run={run}
        cell={makeCell({ hasData: false, runId: "r2" })}
        scenarioId="checkout"
        providerId="claude"
        providerLabel="Claude"
        onSelectMessage={onSelectMessage}
      />,
    );
    fireEvent.click(screen.getByText("Hi"));
    expect(onSelectMessage).toHaveBeenCalledWith(0);
  });
});
