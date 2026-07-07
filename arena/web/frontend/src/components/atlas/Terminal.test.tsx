import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Terminal } from "./Terminal";
import type { TerminalLine } from "./types";

describe("Terminal", () => {
  it("renders a command line with its text and the gold prompt, and a success line's glyph", () => {
    const lines: TerminalLine[] = [
      { type: "command", text: "bundle install" },
      { type: "success", text: "Bundle complete" },
    ];
    render(<Terminal lines={lines} cursor={false} />);
    expect(screen.getByText("bundle install")).toBeInTheDocument();
    expect(screen.getByText("omnia ❯")).toBeInTheDocument();
    expect(screen.getByText("✓", { exact: false })).toBeInTheDocument();
    expect(screen.getByText("Bundle complete")).toBeInTheDocument();
  });
});
