import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { ArenaState } from "@/types";
import type { LiveConsoleProps } from "@altairalabs/atlas";

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

// useVoiceCall is mocked so voice-wiring tests don't need real Web Audio/WS —
// it always returns a stable LiveConsoleCall-shaped object; the tests only
// assert on whether that object reaches LiveConsole, not on call behavior.
const useVoiceCallSpy = vi.fn((_opts: unknown) => ({
  state: "idle" as const,
  muted: false,
  level: 0,
  onCall: vi.fn(),
  onHangup: vi.fn(),
  onToggleMute: vi.fn(),
}));

vi.mock("@/hooks/useVoiceCall", () => ({
  useVoiceCall: (opts: unknown) => useVoiceCallSpy(opts),
}));

// LiveConsole is replaced with a minimal stand-in (rather than rendering the
// real Atlas component) so we can assert on the `call` prop it receives
// without driving the full setup -> chat phase transition for every voice
// assertion, and without pulling the real component's React tree — and its
// own copy of `react` resolved from the atlas package — into this render.
// It renders just enough surface for existing/adjacent tests to still find
// the composer placeholder, plus a "Call" button gated on `call` being set,
// mirroring Atlas's own LiveCallBar (`aria-label="Call"` when idle).
const liveConsoleSpy = vi.fn();

vi.mock("@altairalabs/atlas", async () => {
  const actual = await vi.importActual<typeof import("@altairalabs/atlas")>("@altairalabs/atlas");
  return {
    ...actual,
    LiveConsole: (props: LiveConsoleProps) => {
      liveConsoleSpy(props);
      return (
        <div>
          <div>{props.title}</div>
          <input placeholder={props.composerPlaceholder} disabled={props.composerDisabled} readOnly />
          {props.call && <button aria-label="Call" onClick={props.call.onCall} />}
        </div>
      );
    },
  };
});

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
    useVoiceCallSpy.mockClear();
    liveConsoleSpy.mockClear();
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
    // The Atlas LiveConsole composer renders in the chat phase.
    expect(screen.getByPlaceholderText(/Type a message/)).toBeInTheDocument();
  });

  it("passes a call to LiveConsole once a session is active and voice is enabled", async () => {
    fetchOptions.mockResolvedValue({
      agents: [{ taskType: "support", description: "" }],
      providers: ["claude"],
      hasEvals: false,
      voice: true,
    });
    createSession.mockResolvedValue({ sessionId: "sess-voice" });

    render(
      <InteractiveChat state={emptyState()} registerInteractiveRun={vi.fn()} onBack={vi.fn()} />,
    );

    fireEvent.click(await screen.findByText("Start Chat"));

    await waitFor(() => expect(screen.getByPlaceholderText(/Type a message/)).toBeInTheDocument());

    const calls = liveConsoleSpy.mock.calls;
    const lastProps = calls[calls.length - 1]?.[0] as { call?: unknown } | undefined;
    expect(lastProps?.call).toBeDefined();
    expect(screen.getByLabelText("Call")).toBeInTheDocument();
  });

  it("does not pass a call to LiveConsole when voice is disabled", async () => {
    fetchOptions.mockResolvedValue({
      agents: [{ taskType: "support", description: "" }],
      providers: ["claude"],
      hasEvals: false,
      voice: false,
    });
    createSession.mockResolvedValue({ sessionId: "sess-no-voice" });

    render(
      <InteractiveChat state={emptyState()} registerInteractiveRun={vi.fn()} onBack={vi.fn()} />,
    );

    fireEvent.click(await screen.findByText("Start Chat"));

    await waitFor(() => expect(screen.getByPlaceholderText(/Type a message/)).toBeInTheDocument());

    const calls = liveConsoleSpy.mock.calls;
    const lastProps = calls[calls.length - 1]?.[0] as { call?: unknown } | undefined;
    expect(lastProps?.call).toBeUndefined();
    expect(screen.queryByLabelText("Call")).not.toBeInTheDocument();
  });
});
