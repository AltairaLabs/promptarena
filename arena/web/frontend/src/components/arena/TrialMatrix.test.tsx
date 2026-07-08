import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { TrialMatrix } from "./TrialMatrix";
import type { TrialMatrix as TrialMatrixModel } from "@/types";

function makeMatrix(): TrialMatrixModel {
  return {
    providers: [
      { id: "claude", label: "claude" },
      { id: "gpt4o", label: "gpt4o" },
    ],
    rows: [
      {
        scenarioId: "checkout",
        label: "checkout",
        cells: [
          {
            scenarioId: "checkout",
            providerId: "claude",
            key: "checkout:claude",
            passRate: 100,
            passed: true,
            best: true,
            costUsd: 0.01,
            latencyMs: 633792,
            runId: "r1",
            hasData: true,
          },
          {
            scenarioId: "checkout",
            providerId: "gpt4o",
            key: "checkout:gpt4o",
            passRate: 50,
            passed: false,
            best: false,
            costUsd: 0,
            latencyMs: 0.634,
            runId: "r2",
            hasData: true,
          },
        ],
      },
      {
        scenarioId: "refund",
        label: "refund",
        cells: [
          {
            scenarioId: "refund",
            providerId: "claude",
            key: "refund:claude",
            passRate: 0,
            passed: false,
            best: false,
            costUsd: 0,
            latencyMs: 0,
            runId: "",
            hasData: false,
          },
          {
            scenarioId: "refund",
            providerId: "gpt4o",
            key: "refund:gpt4o",
            passRate: 0,
            passed: false,
            best: false,
            costUsd: 0,
            latencyMs: 0,
            runId: "",
            hasData: false,
          },
        ],
      },
    ],
  };
}

describe("TrialMatrix", () => {
  it("renders one header cell per provider and one row per scenario", () => {
    render(<TrialMatrix matrix={makeMatrix()} selectedKey={null} onSelect={() => {}} />);
    expect(screen.getByText("claude")).toBeInTheDocument();
    expect(screen.getByText("gpt4o")).toBeInTheDocument();
    expect(screen.getByText("checkout")).toBeInTheDocument();
    expect(screen.getByText("refund")).toBeInTheDocument();
  });

  it("calls onSelect with the scenario:provider key when a data cell is clicked", () => {
    const onSelect = vi.fn();
    render(<TrialMatrix matrix={makeMatrix()} selectedKey={null} onSelect={onSelect} />);
    fireEvent.click(screen.getByText("100%"));
    expect(onSelect).toHaveBeenCalledWith("checkout:claude");
  });

  it("renders latency as a human-readable duration, not a raw millisecond count", () => {
    render(<TrialMatrix matrix={makeMatrix()} selectedKey={null} onSelect={() => {}} />);
    expect(screen.getByText("10m 34s")).toBeInTheDocument();
    expect(screen.getByText("<1ms")).toBeInTheDocument();
  });

  it("shows the gold star for the best cell", () => {
    render(<TrialMatrix matrix={makeMatrix()} selectedKey={null} onSelect={() => {}} />);
    const bestCellButton = screen.getByText("100%").closest("button")!;
    expect(bestCellButton.querySelector("img")).toBeTruthy();
  });

  it("does not show a star for a non-best cell", () => {
    render(<TrialMatrix matrix={makeMatrix()} selectedKey={null} onSelect={() => {}} />);
    const cellButton = screen.getByText("50%").closest("button")!;
    expect(cellButton.querySelector("img")).toBeNull();
  });

  it("applies the inset cyan ring to the selected cell", () => {
    render(<TrialMatrix matrix={makeMatrix()} selectedKey="checkout:gpt4o" onSelect={() => {}} />);
    const selectedButton = screen.getByText("50%").closest("button")!;
    expect(selectedButton.style.boxShadow).toContain("var(--ion-cyan)");
  });

  it("renders empty cells as non-interactive dashes when onRunCell is not provided", () => {
    render(<TrialMatrix matrix={makeMatrix()} selectedKey={null} onSelect={() => {}} />);
    const dashes = screen.getAllByText("—");
    expect(dashes).toHaveLength(2);
    dashes.forEach((d) => expect(d.closest("button")).toBeNull());
  });

  it("renders a clickable run affordance for an empty cell when onRunCell is provided, and calls it with scenario+provider", () => {
    const onRunCell = vi.fn();
    render(<TrialMatrix matrix={makeMatrix()} selectedKey={null} onSelect={() => {}} onRunCell={onRunCell} />);
    expect(screen.queryByText("—")).not.toBeInTheDocument();
    const runButton = screen.getByRole("button", { name: "Run refund on claude" });
    fireEvent.click(runButton);
    expect(onRunCell).toHaveBeenCalledWith("refund", "claude");
  });

  it("still calls onSelect (not onRunCell) when a filled cell is clicked, even with onRunCell provided", () => {
    const onSelect = vi.fn();
    const onRunCell = vi.fn();
    render(<TrialMatrix matrix={makeMatrix()} selectedKey={null} onSelect={onSelect} onRunCell={onRunCell} />);
    fireEvent.click(screen.getByText("100%"));
    expect(onSelect).toHaveBeenCalledWith("checkout:claude");
    expect(onRunCell).not.toHaveBeenCalled();
  });
});
