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

  it("renders an edge's label just above the line midpoint when present", () => {
    const nodes: GraphNode[] = [
      { id: "a", x: 10, y: 10, kind: "entry" },
      { id: "b", x: 50, y: 90, kind: "agent" },
    ];
    const edges: GraphEdge[] = [{ from: "a", to: "b", label: "classified" }];
    const { container } = render(<ConstellationGraph nodes={nodes} edges={edges} />);
    const label = container.querySelector("text");
    expect(label).not.toBeNull();
    expect(label!.textContent).toBe("classified");
    expect(label!.getAttribute("x")).toBe("30");
    // midpoint y (50) lifted 5px off the line so the label clears the stroke
    expect(label!.getAttribute("y")).toBe("45");
  });

  it("renders no edge label text when label is absent", () => {
    const nodes: GraphNode[] = [
      { id: "a", x: 10, y: 10, kind: "entry" },
      { id: "b", x: 50, y: 90, kind: "agent" },
    ];
    const edges: GraphEdge[] = [{ from: "a", to: "b" }];
    const { container } = render(<ConstellationGraph nodes={nodes} edges={edges} />);
    expect(container.querySelector("text")).toBeNull();
  });

  it("renders a dimmed node's group at reduced opacity, and a non-dimmed node at full opacity", () => {
    const nodes: GraphNode[] = [
      { id: "a", x: 10, y: 10, kind: "entry", dim: true },
      { id: "b", x: 50, y: 90, kind: "agent" },
    ];
    const { container } = render(<ConstellationGraph nodes={nodes} edges={[]} />);
    const groups = container.querySelectorAll("svg > g");
    expect((groups[0] as SVGElement).style.opacity).toBe("0.3");
    expect((groups[1] as SVGElement).style.opacity === "" || (groups[1] as SVGElement).style.opacity === "1").toBe(
      true,
    );
  });

  it("renders a dimmed edge's line at reduced opacity", () => {
    const nodes: GraphNode[] = [
      { id: "a", x: 10, y: 10, kind: "entry" },
      { id: "b", x: 50, y: 90, kind: "agent" },
    ];
    const edges: GraphEdge[] = [{ from: "a", to: "b", dim: true }];
    const { container } = render(<ConstellationGraph nodes={nodes} edges={edges} />);
    const line = container.querySelector("svg line")!;
    expect(line.getAttribute("opacity")).toBe("0.3");
  });

  it("draws a curved path (not a line) with its label on the arc when the edge has a curve", () => {
    const nodes: GraphNode[] = [
      { id: "a", x: 10, y: 50, kind: "entry" },
      { id: "c", x: 210, y: 50, kind: "output" },
    ];
    const edges: GraphEdge[] = [{ from: "a", to: "c", label: "Resolve", curve: { cx: 110, cy: 10 } }];
    const { container } = render(<ConstellationGraph nodes={nodes} edges={edges} />);
    expect(container.querySelector("svg line")).toBeNull();
    const path = container.querySelector("svg path")!;
    expect(path.getAttribute("d")).toBe("M 10 50 Q 110 10 210 50");
    const label = container.querySelector("text")!;
    expect(label.textContent).toBe("Resolve");
    expect(label.getAttribute("x")).toBe("110");
    expect(label.getAttribute("y")).toBe("8"); // curve apex cy (10) lifted 2
  });
});
