import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Standings } from "./Standings";
import type { Standing } from "@/types";

const standings: Standing[] = [
  { rank: 1, providerId: "claude", label: "claude", wins: 2, leader: true },
  { rank: 2, providerId: "gpt4o", label: "gpt4o", wins: 1, leader: false },
];

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
});
