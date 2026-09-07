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
            passedCount: 2,
            totalRuns: 2,
            passed: true,
            scored: true,
            history: [],
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
            passedCount: 1,
            totalRuns: 2,
            passed: false,
            scored: true,
            history: [],
            best: false,
            costUsd: 0.02,
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
            passedCount: 0,
            totalRuns: 0,
            passed: false,
            scored: false,
            history: [],
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
            passedCount: 0,
            totalRuns: 0,
            passed: false,
            scored: false,
            history: [],
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

// A wide field — the shape `examples/capability-matrix` demonstrates, and the
// one that used to collapse every column to an illegible sliver.
function makeWideMatrix(providerCount: number): TrialMatrixModel {
  const providers = Array.from({ length: providerCount }, (_, i) => ({
    id: `p${i + 1}`,
    label: `provider-${i + 1}`,
  }));
  return {
    providers,
    rows: [
      {
        scenarioId: "checkout",
        label: "checkout",
        cells: providers.map((p) => ({
          scenarioId: "checkout",
          providerId: p.id,
          key: `checkout:${p.id}`,
          passRate: 0,
          passedCount: 0,
          totalRuns: 0,
          passed: false,
          scored: false,
          history: [],
          best: false,
          costUsd: 0,
          latencyMs: 0,
          runId: "",
          hasData: false,
        })),
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

  it("renders a scored data cell as an em-dash, not a percentage, so an unjudged run never reads as a win", () => {
    const m = makeMatrix();
    const cell = m.rows[0].cells[1]; // checkout:gpt4o
    cell.scored = false;
    cell.passed = false;
    render(<TrialMatrix matrix={m} selectedKey={null} onSelect={() => {}} />);
    // Its cost ("$0.020") still renders, but the rate shows "—" and no star.
    const unscoredButton = screen.getByText("$0.020").closest("button")!;
    expect(unscoredButton.textContent).toContain("—");
    expect(unscoredButton.textContent).not.toContain("50%");
    expect(unscoredButton.querySelector("img")).toBeNull();
  });

  it("shows the passed/total run count alongside the aggregate rate", () => {
    render(<TrialMatrix matrix={makeMatrix()} selectedKey={null} onSelect={() => {}} />);
    // checkout:gpt4o aggregated 1 of 2 runs → "50%" and "1/2".
    const btn = screen.getByText("50%").closest("button")!;
    expect(btn.textContent).toContain("1/2");
  });

  it("renders a flakiness strip — one mark per recent trial — when a cell has history", () => {
    const m = makeMatrix();
    m.rows[0].cells[0].history = ["pass", "pass", "fail"];
    render(<TrialMatrix matrix={m} selectedKey={null} onSelect={() => {}} />);
    const button = screen.getByText("100%").closest("button")!;
    const strip = button.querySelector('[title^="Recent trials"]')!;
    expect(strip).toBeTruthy();
    expect(strip.children).toHaveLength(3);
  });

  it("hides the strip for a cell with fewer than two runs", () => {
    const m = makeMatrix();
    m.rows[0].cells[0].history = ["pass"];
    render(<TrialMatrix matrix={m} selectedKey={null} onSelect={() => {}} />);
    const button = screen.getByText("100%").closest("button")!;
    expect(button.querySelector('[title^="Recent trials"]')).toBeNull();
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

  it("gives every column a legible floor instead of an unbounded 1fr", () => {
    const { container } = render(
      <TrialMatrix matrix={makeWideMatrix(35)} selectedKey={null} onSelect={() => {}} />,
    );
    const grids = container.querySelectorAll<HTMLElement>('[data-matrix-grid="true"]');
    expect(grids.length).toBeGreaterThan(0);
    grids.forEach((g) => {
      expect(g.style.gridTemplateColumns).toContain("minmax(");
      // A bare `1fr` repeat has no floor, so the columns clip instead of
      // overflowing where a scrollbar can appear.
      expect(g.style.gridTemplateColumns).not.toMatch(/repeat\(\d+, 1fr\)/);
    });
  });

  it("scrolls the header and the body together, so they cannot disagree on column count", () => {
    const { container } = render(
      <TrialMatrix matrix={makeWideMatrix(35)} selectedKey={null} onSelect={() => {}} />,
    );
    const scroller = container.querySelector<HTMLElement>('[data-matrix-scroll="true"]')!;
    expect(scroller).toBeTruthy();
    expect(scroller.style.overflowX).toBe("auto");
    // Both grids live inside the one scroller and share one template.
    const grids = scroller.querySelectorAll<HTMLElement>('[data-matrix-grid="true"]');
    expect(grids).toHaveLength(2); // header + one scenario row
    expect(grids[0].style.gridTemplateColumns).toBe(grids[1].style.gridTemplateColumns);
  });

  it("pins the header row so a deep field never shows unlabelled columns", () => {
    const { container } = render(
      <TrialMatrix matrix={makeWideMatrix(35)} selectedKey={null} onSelect={() => {}} />,
    );
    const header = container.querySelector<HTMLElement>('[data-matrix-header="true"]')!;
    expect(header).toBeTruthy();
    expect(header.style.position).toBe("sticky");
    expect(header.style.top).toBe("0px");
    // Opaque, or the cells scrolling under it show through.
    expect(header.style.background).toBeTruthy();
  });

  it("pins the scenario column, and the corner cell outranks both", () => {
    const { container } = render(
      <TrialMatrix matrix={makeWideMatrix(35)} selectedKey={null} onSelect={() => {}} />,
    );
    const corner = container.querySelector<HTMLElement>('[data-matrix-corner="true"]')!;
    const rowLabel = container.querySelector<HTMLElement>('[data-matrix-rowlabel="true"]')!;
    expect(corner.style.position).toBe("sticky");
    expect(corner.style.left).toBe("0px");
    expect(rowLabel.style.position).toBe("sticky");
    expect(rowLabel.style.left).toBe("0px");
    // The corner sits where both sticky axes meet, so it must paint over them.
    expect(Number(corner.style.zIndex)).toBeGreaterThan(Number(rowLabel.style.zIndex));
  });

  it("bounds the pane's height so the horizontal scrollbar stays reachable", () => {
    const { container } = render(
      <TrialMatrix matrix={makeWideMatrix(35)} selectedKey={null} onSelect={() => {}} />,
    );
    const scroller = container.querySelector<HTMLElement>('[data-matrix-scroll="true"]')!;
    expect(scroller.style.maxHeight).toBe("70vh");
    expect(scroller.style.overflowY).toBe("auto");
  });

  it("keeps a header cell and a body cell for every provider in a wide field", () => {
    render(<TrialMatrix matrix={makeWideMatrix(35)} selectedKey={null} onSelect={() => {}} />);
    expect(screen.getByText("provider-35")).toBeInTheDocument();
    // One inert dash per provider on the single (empty) scenario row.
    expect(screen.getAllByText("—")).toHaveLength(35);
  });
});
