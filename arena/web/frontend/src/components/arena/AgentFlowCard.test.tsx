import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { AgentFlowCard } from "./AgentFlowCard";
import type { GraphNode, GraphEdge } from "@/components/atlas/types";

describe("AgentFlowCard", () => {
  it("renders the AGENT FLOW header and the constellation graph as an svg", () => {
    const nodes: GraphNode[] = [
      { id: "entry", x: 10, y: 75, kind: "entry", label: "user" },
      { id: "output", x: 350, y: 75, kind: "output", label: "resolved" },
    ];
    const edges: GraphEdge[] = [{ from: "entry", to: "output", gold: true }];
    const { container } = render(<AgentFlowCard flow={{ nodes, edges }} />);
    expect(screen.getByText("AGENT FLOW")).toBeInTheDocument();
    expect(container.querySelector("svg")).toBeTruthy();
    expect(container.querySelectorAll("svg > g")).toHaveLength(2);
  });
});
