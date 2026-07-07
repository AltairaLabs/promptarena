import { render } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { ConstellationGraph } from "./ConstellationGraph";
import type { GraphNode, GraphEdge } from "./types";

describe("ConstellationGraph", () => {
  it("renders one line per resolvable edge and one group per node, skipping unknown endpoints", () => {
    const nodes: GraphNode[] = [
      { id: "a", x: 10, y: 10, kind: "entry" },
      { id: "b", x: 50, y: 50, kind: "agent" },
      { id: "c", x: 90, y: 90, kind: "output" },
    ];
    const edges: GraphEdge[] = [
      { from: "a", to: "b" },
      { from: "b", to: "c" },
      { from: "b", to: "unknown" },
    ];
    const { container } = render(<ConstellationGraph nodes={nodes} edges={edges} />);
    expect(container.querySelectorAll("svg > line").length).toBe(2);
    expect(container.querySelectorAll("svg > g").length).toBe(3);
  });
});
