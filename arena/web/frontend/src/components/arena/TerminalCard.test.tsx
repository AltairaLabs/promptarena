import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { TerminalCard } from "./TerminalCard";
import type { TerminalLine } from "@altairalabs/atlas";

describe("TerminalCard", () => {
  it("renders the terminal command line and its prompt", () => {
    const lines: TerminalLine[] = [
      { type: "command", text: "promptarena run --scenario checkout --provider claude" },
      { type: "success", text: "✓ assertions passed · $0.0041 · 820ms" },
    ];
    render(<TerminalCard lines={lines} />);
    expect(screen.getByText("promptarena run --scenario checkout --provider claude")).toBeInTheDocument();
    expect(screen.getAllByText("arena ❯").length).toBeGreaterThan(0);
  });
});
