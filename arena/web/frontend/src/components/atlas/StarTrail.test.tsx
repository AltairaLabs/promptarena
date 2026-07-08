import { render } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { StarTrail } from "./StarTrail";

describe("StarTrail", () => {
  it("renders a polyline for a series of points", () => {
    const { container } = render(<StarTrail points={[1, 4, 2, 8, 5]} />);
    expect(container.querySelector("polyline")).toBeInTheDocument();
  });

  it("returns null for empty points", () => {
    const { container } = render(<StarTrail points={[]} />);
    expect(container.firstChild).toBeNull();
  });
});
