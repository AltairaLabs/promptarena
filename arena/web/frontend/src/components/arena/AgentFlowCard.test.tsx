import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { AgentFlowCard } from "./AgentFlowCard";
import type { RunResult, WorkflowGraph } from "@/types";

const graph: WorkflowGraph = {
  nodes: [
    { id: "intake", label: "intake", kind: "entry", entry: true, terminal: false },
    { id: "resolve", label: "resolve", kind: "output", entry: false, terminal: true },
  ],
  edges: [{ from: "intake", to: "resolve" }],
};

function makeRun(overrides: Partial<RunResult> = {}): RunResult {
  return {
    RunID: "r1",
    PromptPack: "",
    Region: "us",
    ScenarioID: "checkout",
    ProviderID: "claude",
    Params: {},
    Messages: [],
    Commit: {},
    Cost: { input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0, total_cost_usd: 0 },
    Violations: [],
    StartTime: "2026-01-01T00:00:00Z",
    EndTime: "2026-01-01T00:00:01Z",
    Duration: 0,
    Error: "",
    SelfPlay: false,
    PersonaID: "",
    MediaOutputs: [],
    A2AAgents: [],
    ...overrides,
  };
}

describe("AgentFlowCard", () => {
  it("renders the WORKFLOW header and a placeholder body without crashing when the graph hasn't loaded", () => {
    render(<AgentFlowCard graph={null} run={undefined} />);
    expect(screen.getByText("WORKFLOW")).toBeInTheDocument();
    expect(screen.getByText("Loading workflow…")).toBeInTheDocument();
  });

  it("renders the React Flow workflow view once the graph has loaded", async () => {
    render(<AgentFlowCard graph={graph} run={makeRun()} />);
    expect(screen.getByText("WORKFLOW")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("intake")).toBeInTheDocument());
    expect(screen.getByText("resolve")).toBeInTheDocument();
  });
});
