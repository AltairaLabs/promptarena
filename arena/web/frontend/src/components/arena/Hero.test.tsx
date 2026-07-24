import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Hero } from "./Hero";

describe("Hero", () => {
  it("appends the CHARTED dateline from the latest run date", () => {
    render(<Hero scenarioCount={4} providerCount={3} chartedAt="2026-07-07T12:00:00Z" />);
    expect(screen.getByText(/^THE ARENA · CHARTED jul \d+$/)).toBeInTheDocument();
  });

  it("omits the dateline entirely when nothing has been charted yet", () => {
    render(<Hero scenarioCount={4} providerCount={3} />);
    expect(screen.getByText("THE ARENA")).toBeInTheDocument();
    expect(screen.queryByText(/CHARTED/)).not.toBeInTheDocument();
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
