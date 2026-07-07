import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { InstrumentReadout } from "./InstrumentReadout";
import type { MetricSpec } from "./types";

describe("InstrumentReadout", () => {
  it("renders each metric's label and value, including a gold-toned metric", () => {
    const metrics: MetricSpec[] = [
      { label: "Replicas", value: "3/3", tone: "healthy" },
      { label: "Budget", value: "$42.10", unit: "usd", tone: "gold" },
    ];
    render(<InstrumentReadout metrics={metrics} />);
    expect(screen.getByText("Replicas")).toBeInTheDocument();
    expect(screen.getByText("3/3")).toBeInTheDocument();
    expect(screen.getByText("Budget")).toBeInTheDocument();
    expect(screen.getByText("$42.10")).toBeInTheDocument();
  });
});
