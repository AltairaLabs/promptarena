import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Standings } from "./Standings";
import type { Standing } from "@/types";

const standings: Standing[] = [
  { rank: 1, providerId: "claude", label: "claude", wins: 2, leader: true },
  { rank: 2, providerId: "gpt4o", label: "gpt4o", wins: 1, leader: false },
];

function makeStandings(n: number): Standing[] {
  return Array.from({ length: n }, (_, i) => ({
    rank: i + 1,
    providerId: `p${i + 1}`,
    label: `p${i + 1}`,
    wins: i === 0 ? 3 : 0,
    leader: i === 0,
  }));
}

describe("Standings", () => {
  it("renders a row per standing with rank, label, and win count", () => {
    render(<Standings standings={standings} />);
    expect(screen.getByText("claude")).toBeInTheDocument();
    expect(screen.getByText("gpt4o")).toBeInTheDocument();
    expect(screen.getByText("2 wins")).toBeInTheDocument();
    expect(screen.getByText("1 win")).toBeInTheDocument();
  });

  it("shows the gold star only for the leader", () => {
    render(<Standings standings={standings} />);
    expect(screen.getAllByRole("img")).toHaveLength(1);
  });

  it("does not add an overflow line when the whole field fits", () => {
    render(<Standings standings={standings} />);
    expect(screen.queryByText(/more contender/)).not.toBeInTheDocument();
  });

  it("caps a wide field so the standings cannot stretch the instrument band", () => {
    render(<Standings standings={makeStandings(35)} />);
    // Only the top eight rank, so the card's height stays a band.
    expect(screen.getByText("p1")).toBeInTheDocument();
    expect(screen.getByText("p8")).toBeInTheDocument();
    expect(screen.queryByText("p9")).not.toBeInTheDocument();
    expect(screen.queryByText("p35")).not.toBeInTheDocument();
  });

  it("says how many contenders the cap left out", () => {
    render(<Standings standings={makeStandings(35)} />);
    expect(screen.getByText("and 27 more contenders")).toBeInTheDocument();
  });

  it("uses the singular when exactly one contender is left out", () => {
    render(<Standings standings={makeStandings(9)} />);
    expect(screen.getByText("and 1 more contender")).toBeInTheDocument();
  });
});
