import { render } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { MetricChart } from "./MetricChart";

describe("MetricChart", () => {
  it("renders an SVG for a small numeric series", () => {
    const { container } = render(<MetricChart data={[1, 3, 2, 5]} />);
    expect(container.querySelector("svg")).toBeInTheDocument();
  });

  it("returns null for empty data", () => {
    const { container } = render(<MetricChart data={[]} />);
    expect(container.firstChild).toBeNull();
  });
});
