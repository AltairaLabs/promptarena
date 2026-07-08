import { render } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Gauge } from "./Gauge";

describe("Gauge", () => {
  it("renders an SVG and the rounded percentage readout by default", () => {
    const { container, getByText } = render(<Gauge value={30} max={100} />);
    expect(container.querySelector("svg")).toBeInTheDocument();
    expect(getByText("30")).toBeInTheDocument();
  });

  it("shows the provided display text instead of the percentage", () => {
    const { getByText } = render(<Gauge value={30} max={100} display="ready" />);
    expect(getByText("ready")).toBeInTheDocument();
  });
});
