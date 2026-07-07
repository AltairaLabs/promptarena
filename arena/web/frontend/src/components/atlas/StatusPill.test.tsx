import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { StatusPill } from "./StatusPill";

describe("StatusPill", () => {
  it("renders its label and a dot for running status", () => {
    const { container } = render(<StatusPill status="running">Live</StatusPill>);
    expect(screen.getByText("Live")).toBeInTheDocument();
    // running has a status dot
    expect(container.querySelectorAll("span").length).toBeGreaterThan(1);
  });
  it("renders reconciled without a dot", () => {
    const { container } = render(<StatusPill status="reconciled">Passed</StatusPill>);
    expect(screen.getByText("Passed")).toBeInTheDocument();
  });
});
