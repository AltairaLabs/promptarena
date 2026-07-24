import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { CommandStrip } from "./CommandStrip";

const scenarios = [
  { id: "checkout", label: "Checkout" },
  { id: "refund" },
];

function setup(overrides: Partial<React.ComponentProps<typeof CommandStrip>> = {}) {
  const props = {
    scenarios,
    selected: ["checkout", "refund"],
    onToggle: vi.fn(),
    onSelectAll: vi.fn(),
    onSelectNone: vi.fn(),
    providerCount: 2,
    runCount: 1,
    onRunCountChange: vi.fn(),
    onRunTrial: vi.fn(),
    ...overrides,
  };
  render(<CommandStrip {...props} />);
  return props;
}

describe("CommandStrip", () => {
  it("renders a pill per scenario, using label when present and id as fallback", () => {
    setup();
    expect(screen.getByText("Checkout")).toBeInTheDocument();
    expect(screen.getByText("refund")).toBeInTheDocument();
  });

  it("marks selected scenarios active and unselected inactive", () => {
    setup({ selected: ["checkout"] });
    expect(screen.getByText("Checkout").closest("button")).toHaveStyle({ background: "var(--starlight-tint)" });
    expect(screen.getByText("refund").closest("button")).not.toHaveStyle({ background: "var(--starlight-tint)" });
  });

  it("toggles a scenario when its pill is clicked", () => {
    const p = setup();
    fireEvent.click(screen.getByText("refund"));
    expect(p.onToggle).toHaveBeenCalledWith("refund");
  });

  it("selects all or none via the quick toggles", () => {
    const p = setup();
    fireEvent.click(screen.getByText("All"));
    expect(p.onSelectAll).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByText("None"));
    expect(p.onSelectNone).toHaveBeenCalledTimes(1);
  });

  it("reads out the blast radius: scenarios × contenders × sweeps = trials", () => {
    setup({ selected: ["checkout", "refund"], providerCount: 3 });
    // 1 sweep · 2 scenarios × 3 contenders = 6 trials
    expect(screen.getByText("2 scenarios · 3 contenders = 6 trials")).toBeInTheDocument();
  });

  it("uses singular nouns and counts sweeps into the trial total", () => {
    setup({ selected: ["checkout"], providerCount: 1, runCount: 1 });
    expect(screen.getByText("1 scenario · 1 contender = 1 trial")).toBeInTheDocument();
  });

  it("multiplies the trial total by the sweep count", () => {
    setup({ selected: ["checkout", "refund"], providerCount: 3, runCount: 4 });
    expect(screen.getByText("4 sweeps · 2 scenarios · 3 contenders = 24 trials")).toBeInTheDocument();
  });

  it("steps the sweep count up and down within bounds", () => {
    const p = setup({ runCount: 2 });
    fireEvent.click(screen.getByLabelText("more sweeps"));
    expect(p.onRunCountChange).toHaveBeenCalledWith(3);
    fireEvent.click(screen.getByLabelText("fewer sweeps"));
    expect(p.onRunCountChange).toHaveBeenCalledWith(1);
  });

  it("disables the − step at 1 and the + step at the cap", () => {
    setup({ runCount: 1 });
    expect(screen.getByLabelText("fewer sweeps")).toBeDisabled();
    expect(screen.getByLabelText("more sweeps")).not.toBeDisabled();
  });

  it("shows an estimated cost and time when an estimate is provided", () => {
    setup({ estimate: { costUsd: 0.06, timeMs: 6, covered: 2, total: 2 } });
    expect(screen.getByText(/≈ \$0\.06 · ~6ms/)).toBeInTheDocument();
  });

  it("renders no estimate when none is provided", () => {
    setup();
    expect(screen.queryByText(/≈/)).not.toBeInTheDocument();
  });

  it("flags a partial-coverage estimate as a lower bound", () => {
    setup({ estimate: { costUsd: 0.02, timeMs: 2, covered: 1, total: 2 } });
    expect(screen.getByText(/≥ \$0\.02 · ~2ms/)).toBeInTheDocument();
  });

  it("runs the field when the action is clicked", () => {
    const p = setup();
    fireEvent.click(screen.getByText(/Run the field/));
    expect(p.onRunTrial).toHaveBeenCalledTimes(1);
  });

  it("disables Run the field when runDisabled is true", () => {
    setup({ runDisabled: true });
    expect(screen.getByText(/Run the field/).closest("button")).toBeDisabled();
  });
});
