import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
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

  it("renders no minimap chrome", () => {
    const { container } = render(<WorkflowGraphView elements={elements} />);
    expect(container.querySelector(".react-flow__minimap")).not.toBeInTheDocument();
  });

  it("calls onStateClick with the state id when a composition-owning node is clicked", () => {
    const onStateClick = vi.fn();
    render(<WorkflowGraphView elements={elements} onStateClick={onStateClick} />);

    fireEvent.click(screen.getByText("intake"));

    expect(onStateClick).toHaveBeenCalledWith("intake");
  });

  it("does not call onStateClick when a non-composition node is clicked", () => {
    const onStateClick = vi.fn();
    render(<WorkflowGraphView elements={elements} onStateClick={onStateClick} />);

    fireEvent.click(screen.getByText("resolve"));

    expect(onStateClick).not.toHaveBeenCalled();
  });

  it("maps a click on an expanded group node back to its owning state id via data.stateId", () => {
    const onStateClick = vi.fn();
    const expandedElements: FlowElements = {
      nodes: [
        {
          id: "grp:intake",
          type: "group",
          position: { x: 0, y: 0 },
          data: { label: "intake", kind: "entry", isGroup: true, stateId: "intake" },
          style: { width: 200, height: 120 },
        },
        {
          id: "intake/step1",
          type: "workflowNode",
          position: { x: 20, y: 40 },
          data: { label: "step1", kind: "prompt" },
          parentId: "grp:intake",
          extent: "parent",
        },
      ],
      edges: [],
    };
    render(<WorkflowGraphView elements={expandedElements} onStateClick={onStateClick} />);

    fireEvent.click(screen.getByText("intake"));

    expect(onStateClick).toHaveBeenCalledWith("intake");
  });
});
