import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { WorkflowGraphView } from "./WorkflowGraphView";
import type { FlowElements } from "@/lib/workflowFlow";

const elements: FlowElements = {
  nodes: [
    {
      id: "intake",
      type: "workflowNode",
      position: { x: 0, y: 0 },
      data: { label: "intake", kind: "entry", hasComposition: true },
    },
    {
      id: "resolve",
      type: "workflowNode",
      position: { x: 200, y: 0 },
      data: { label: "resolve", kind: "output" },
    },
  ],
  edges: [
    {
      id: "intake->resolve",
      source: "intake",
      target: "resolve",
      data: { label: "classified" },
    },
  ],
};

describe("WorkflowGraphView", () => {
  it("mounts without throwing and renders the node labels", async () => {
    const { container } = render(<WorkflowGraphView elements={elements} />);
    expect(screen.getByText("intake")).toBeInTheDocument();
    expect(screen.getByText("resolve")).toBeInTheDocument();
    await waitFor(() => expect(container.querySelector(".react-flow__edge")).toBeTruthy());
  });

  it("shows the composition badge on a node whose data.hasComposition is set", () => {
    render(<WorkflowGraphView elements={elements} />);
    expect(screen.getByLabelText("has composition")).toBeInTheDocument();
  });
});
