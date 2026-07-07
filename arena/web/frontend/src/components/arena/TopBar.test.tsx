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
        onRunTrial={vi.fn()}
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
        onRunTrial={vi.fn()}
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
        onRunTrial={vi.fn()}
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
        onRunTrial={vi.fn()}
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
        onRunTrial={vi.fn()}
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
        onRunTrial={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /switch to light mode/i }));
    expect(onToggleTheme).toHaveBeenCalledTimes(1);
  });

  it("calls onRunTrial when the gold Run trial button is clicked", () => {
    const onRunTrial = vi.fn();
    render(
      <TopBar
        connected={true}
        runningLive={false}
        theme="dark"
        onToggleTheme={vi.fn()}
        onRunTrial={onRunTrial}
      />,
    );
    fireEvent.click(screen.getByText(/Run trial/));
    expect(onRunTrial).toHaveBeenCalledTimes(1);
  });

  it("disables the Run trial button when runDisabled is true", () => {
    render(
      <TopBar
        connected={true}
        runningLive={false}
        theme="dark"
        onToggleTheme={vi.fn()}
        onRunTrial={vi.fn()}
        runDisabled={true}
      />,
    );
    expect(screen.getByText(/Run trial/).closest("button")).toBeDisabled();
  });
});
