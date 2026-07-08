import { render, screen, waitFor, fireEvent } from "@testing-library/react";
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

// document-analysis-shaped graph: "analyzing" owns a composition step
// ("classify") that only renders once the panel's Expanded toggle is on, and
// "resolve" is left unvisited by the run below so This-run-only can drop it.
const compositionGraph: WorkflowGraph = {
  nodes: [
    { id: "intake", label: "intake", kind: "entry", entry: true, terminal: false },
    { id: "analyzing", label: "analyzing", kind: "agent", entry: false, terminal: false },
    { id: "classify", label: "classify", kind: "prompt", entry: false, terminal: false, parent: "analyzing" },
    { id: "resolve", label: "resolve", kind: "output", entry: false, terminal: true },
  ],
  edges: [
    { from: "intake", to: "analyzing" },
    { from: "analyzing", to: "classify" },
    { from: "analyzing", to: "resolve" },
  ],
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
    render(<AgentFlowCard graph={null} run={undefined} theme="dark" />);
    expect(screen.getByText("WORKFLOW")).toBeInTheDocument();
    expect(screen.getByText("Loading workflow…")).toBeInTheDocument();
  });

  it("renders the React Flow workflow view once the graph has loaded", async () => {
    render(<AgentFlowCard graph={graph} run={makeRun()} theme="dark" />);
    expect(screen.getByText("WORKFLOW")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("intake")).toBeInTheDocument());
    expect(screen.getByText("resolve")).toBeInTheDocument();
  });

  it("defaults to all compositions collapsed and This-run-only off, with no minimap", async () => {
    const { container } = render(<AgentFlowCard graph={compositionGraph} run={makeRun()} theme="dark" />);
    await waitFor(() => expect(screen.getByText("analyzing")).toBeInTheDocument());

    // Collapsed: the composition-owning state renders as a single node, not
    // its step ("classify") — and every top-level state is present since
    // this-run-only defaults to off. Start/end terminators always show.
    expect(screen.queryByText("classify")).not.toBeInTheDocument();
    expect(screen.getByText("intake")).toBeInTheDocument();
    expect(screen.getByText("resolve")).toBeInTheDocument();
    expect(screen.getByText("start")).toBeInTheDocument();
    expect(screen.getByText("end")).toBeInTheDocument();

    expect(screen.getByRole("button", { name: "Expand all" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "This run only" })).toHaveAttribute("aria-pressed", "false");

    // No minimap chrome anywhere in the panel.
    expect(container.querySelector(".react-flow__minimap")).not.toBeInTheDocument();
  });

  it("clicking Expand all reveals the composition's step nodes, then Collapse all hides them again", async () => {
    render(<AgentFlowCard graph={compositionGraph} run={makeRun()} theme="dark" />);
    await waitFor(() => expect(screen.getByText("analyzing")).toBeInTheDocument());
    expect(screen.queryByText("classify")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));

    await waitFor(() => expect(screen.getByText("classify")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Collapse all" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Collapse all" }));

    await waitFor(() => expect(screen.queryByText("classify")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Expand all" })).toBeInTheDocument();
  });

  it("clicking the composition-owning state node itself expands it", async () => {
    render(<AgentFlowCard graph={compositionGraph} run={makeRun()} theme="dark" />);
    await waitFor(() => expect(screen.getByText("analyzing")).toBeInTheDocument());
    expect(screen.queryByText("classify")).not.toBeInTheDocument();

    fireEvent.click(screen.getByText("analyzing"));

    await waitFor(() => expect(screen.getByText("classify")).toBeInTheDocument());
  });

  it("toggling This run only drops a state the run never visited", async () => {
    const run = makeRun({
      Messages: [
        { role: "system", content: "", meta: { _workflow_state: { current_state: "intake" } } },
        {
          role: "tool",
          content: "",
          meta: { _workflow_state: { current_state: "analyzing", previous_state: "intake", transition: "start" } },
        },
      ],
    });
    render(<AgentFlowCard graph={compositionGraph} run={run} theme="dark" />);
    await waitFor(() => expect(screen.getByText("analyzing")).toBeInTheDocument());
    expect(screen.getByText("resolve")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "This run only" }));

    await waitFor(() => expect(screen.queryByText("resolve")).not.toBeInTheDocument());
    expect(screen.getByText("intake")).toBeInTheDocument();
    expect(screen.getByText("analyzing")).toBeInTheDocument();
  });
});
