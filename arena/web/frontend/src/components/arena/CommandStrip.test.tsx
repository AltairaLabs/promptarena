import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { CommandStrip } from "./CommandStrip";

const scenarios = [
  { id: "checkout", label: "Checkout" },
  { id: "refund" },
];

describe("CommandStrip", () => {
  it("renders a chip per scenario, using label when present and id as fallback", () => {
    render(
      <CommandStrip
        scenarios={scenarios}
        selectedScenario="checkout"
        selectedProviderLabel="Claude"
        onSelectScenario={vi.fn()}
        onRunTrial={vi.fn()}
      />,
    );
    expect(screen.getByText("Checkout")).toBeInTheDocument();
    expect(screen.getByText("refund")).toBeInTheDocument();
  });

  it("gold-tints the active scenario chip", () => {
    render(
      <CommandStrip
        scenarios={scenarios}
        selectedScenario="checkout"
        selectedProviderLabel="Claude"
        onSelectScenario={vi.fn()}
        onRunTrial={vi.fn()}
      />,
    );
    const active = screen.getByText("Checkout").closest("button");
    const inactive = screen.getByText("refund").closest("button");
    expect(active).toHaveStyle({ background: "var(--gold-tint)" });
    expect(inactive).not.toHaveStyle({ background: "var(--gold-tint)" });
  });

  it("calls onSelectScenario with the scenario id when a chip is clicked", () => {
    const onSelectScenario = vi.fn();
    render(
      <CommandStrip
        scenarios={scenarios}
        selectedScenario="checkout"
        selectedProviderLabel="Claude"
        onSelectScenario={onSelectScenario}
        onRunTrial={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByText("refund"));
    expect(onSelectScenario).toHaveBeenCalledWith("refund");
  });

  it("renders the provider · scenario readout, falling back to em-dash", () => {
    render(
      <CommandStrip
        scenarios={scenarios}
        selectedScenario={null}
        selectedProviderLabel={null}
        onSelectScenario={vi.fn()}
        onRunTrial={vi.fn()}
      />,
    );
    expect(screen.getByText("— · —")).toBeInTheDocument();
  });

  it("calls onRunTrial when Run trial is clicked", () => {
    const onRunTrial = vi.fn();
    render(
      <CommandStrip
        scenarios={scenarios}
        selectedScenario="checkout"
        selectedProviderLabel="Claude"
        onSelectScenario={vi.fn()}
        onRunTrial={onRunTrial}
      />,
    );
    fireEvent.click(screen.getByText(/Run trial/));
    expect(onRunTrial).toHaveBeenCalledTimes(1);
  });

  it("disables Run trial when runDisabled is true", () => {
    render(
      <CommandStrip
        scenarios={scenarios}
        selectedScenario="checkout"
        selectedProviderLabel="Claude"
        onSelectScenario={vi.fn()}
        onRunTrial={vi.fn()}
        runDisabled={true}
      />,
    );
    expect(screen.getByText(/Run trial/).closest("button")).toBeDisabled();
  });
});
