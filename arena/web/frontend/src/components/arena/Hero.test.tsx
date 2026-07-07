import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Hero } from "./Hero";

describe("Hero", () => {
  it("renders the eyebrow with the stable 'THE ARENA · CHARTED ' prefix", () => {
    render(<Hero scenarioCount={4} providerCount={3} />);
    expect(screen.getByText(/^THE ARENA · CHARTED /)).toBeInTheDocument();
  });

  it("renders the H1 with real counts and pluralizes scenarios/contenders", () => {
    render(<Hero scenarioCount={4} providerCount={3} />);
    expect(screen.getByText("4 scenarios.")).toBeInTheDocument();
    expect(screen.getByText("3 contenders.")).toBeInTheDocument();
  });

  it("uses singular forms for a count of 1", () => {
    render(<Hero scenarioCount={1} providerCount={1} />);
    expect(screen.getByText("1 scenario.")).toBeInTheDocument();
    expect(screen.getByText("1 contender.")).toBeInTheDocument();
  });

  it("renders the gold contenders span distinctly from the scenarios clause", () => {
    render(<Hero scenarioCount={4} providerCount={3} />);
    const gold = screen.getByText("3 contenders.");
    expect(gold.tagName).toBe("SPAN");
    expect(gold).toHaveStyle({ color: "var(--gold-500)" });
  });
});
