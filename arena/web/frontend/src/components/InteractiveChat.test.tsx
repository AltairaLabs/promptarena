import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { ArenaState } from "@/types";

const fetchOptions = vi.fn();
const createSession = vi.fn();
const sendMessage = vi.fn();

vi.mock("@/hooks/useInteractiveChat", () => ({
  useInteractiveChat: () => ({
    fetchOptions,
    createSession,
    sendMessage,
    busy: false,
    error: null,
  }),
}));

const { InteractiveChat } = await import("@/components/InteractiveChat");

function emptyState(overrides: Partial<ArenaState> = {}): ArenaState {
  return {
    connected: true,
    runs: {},
    completedRunIds: [],
    totalCost: 0,
    totalTokens: 0,
    logs: [],
    ...overrides,
  };
}

describe("InteractiveChat", () => {
  beforeEach(() => {
    fetchOptions.mockReset();
    createSession.mockReset();
    sendMessage.mockReset();
  });

  it("renders the setup card once options load, gating Start Chat until agent + provider are chosen", async () => {
    fetchOptions.mockResolvedValue({
      agents: [
        { taskType: "support", description: "" },
        { taskType: "sales", description: "" },
      ],
      providers: ["claude", "mock"],
      hasEvals: false,
    });

    render(
      <InteractiveChat state={emptyState()} registerInteractiveRun={vi.fn()} onBack={vi.fn()} />,
    );

    expect(await screen.findByText("Interactive Chat")).toBeInTheDocument();
    const startButton = screen.getByText("Start Chat").closest("button")!;
    expect(startButton).toBeDisabled();

    fireEvent.change(screen.getByDisplayValue("Select agent…"), { target: { value: "support" } });
    fireEvent.change(screen.getByDisplayValue("Select provider…"), { target: { value: "claude" } });
    expect(startButton).not.toBeDisabled();
  });

  it("shows an error state (with a Back to Runs link) when options fail to load", async () => {
    fetchOptions.mockRejectedValue(new Error("boom"));
    const onBack = vi.fn();

    render(<InteractiveChat state={emptyState()} registerInteractiveRun={vi.fn()} onBack={onBack} />);

    expect(await screen.findByText(/Failed to load interactive options: boom/)).toBeInTheDocument();
    fireEvent.click(screen.getByText(/Back to Runs/));
    expect(onBack).toHaveBeenCalledTimes(1);
  });

  it("advances to the chat phase and renders the session header once a session is created", async () => {
    fetchOptions.mockResolvedValue({
      agents: [{ taskType: "support", description: "" }],
      providers: ["claude"],
      hasEvals: false,
    });
    createSession.mockResolvedValue({ sessionId: "sess-1" });
    const registerInteractiveRun = vi.fn();

    render(
      <InteractiveChat state={emptyState()} registerInteractiveRun={registerInteractiveRun} onBack={vi.fn()} />,
    );

    fireEvent.click(await screen.findByText("Start Chat"));

    await waitFor(() => expect(registerInteractiveRun).toHaveBeenCalledWith("sess-1"));
    expect(await screen.findByText("support")).toBeInTheDocument();
    expect(screen.getByText("claude")).toBeInTheDocument();
    expect(screen.getByText("Send a message to start the conversation.")).toBeInTheDocument();
  });
});
