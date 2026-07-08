import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { TopBar } from "./TopBar";

describe("TopBar", () => {
  it("renders the wordmark and the promptpack context", () => {
    render(
      <TopBar
        connected={true}
        promptpack="support-suite · v2.4.0"
        runningLive={false}
        theme="dark"
        onToggleTheme={vi.fn()}
      />,
    );
    expect(screen.getByText("PromptArena")).toBeInTheDocument();
    expect(screen.getByText("studio")).toBeInTheDocument();
    expect(screen.getByText("support-suite · v2.4.0")).toBeInTheDocument();
  });

  it("omits the promptpack context when undefined", () => {
    render(
      <TopBar
        connected={true}
        runningLive={false}
        theme="dark"
        onToggleTheme={vi.fn()}
      />,
    );
    expect(screen.queryByText(/·/)).not.toBeInTheDocument();
  });

  it("shows a Live pill when a trial is running live, regardless of connection state", () => {
    render(
      <TopBar
        connected={true}
        runningLive={true}
        theme="dark"
        onToggleTheme={vi.fn()}
      />,
    );
    expect(screen.getByText("Live")).toBeInTheDocument();
  });

  it("shows an Online pill when connected and idle", () => {
    render(
      <TopBar
        connected={true}
        runningLive={false}
        theme="dark"
        onToggleTheme={vi.fn()}
      />,
    );
    expect(screen.getByText("Online")).toBeInTheDocument();
  });

  it("shows an Offline pill when disconnected and idle", () => {
    render(
      <TopBar
        connected={false}
        runningLive={false}
        theme="dark"
        onToggleTheme={vi.fn()}
      />,
    );
    expect(screen.getByText("Offline")).toBeInTheDocument();
  });

  it("calls onToggleTheme when the theme toggle button is clicked", () => {
    const onToggleTheme = vi.fn();
    render(
      <TopBar
        connected={true}
        runningLive={false}
        theme="dark"
        onToggleTheme={onToggleTheme}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /switch to light mode/i }));
    expect(onToggleTheme).toHaveBeenCalledTimes(1);
  });

  it("does not render a Run trial button (that action lives in CommandStrip)", () => {
    render(
      <TopBar
        connected={true}
        runningLive={false}
        theme="dark"
        onToggleTheme={vi.fn()}
      />,
    );
    expect(screen.queryByText(/Run trial/)).not.toBeInTheDocument();
  });

  it("follows the theme (a translucent surface bar, no forced dark-ramp pins)", () => {
    const { container } = render(
      <TopBar connected={true} runningLive={false} theme="light" onToggleTheme={vi.fn()} />,
    );
    const header = container.querySelector("header")!;
    // The bar no longer pins the dark ramp locally — it inherits the flipping
    // --c-*/--star-* tokens so it goes light in light mode, dark in dark mode.
    expect(header.style.getPropertyValue("--ink-canvas")).toBe("");
    expect(header.style.getPropertyValue("--star-100")).toBe("");
    // Background is the theme-following frosted surface.
    expect(header.style.background).toContain("var(--c-surface)");
  });
});
