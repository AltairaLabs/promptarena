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

  it("passes a call to LiveConsole once a session is active and the selected provider supports voice", async () => {
    fetchOptions.mockResolvedValue({
      agents: [{ taskType: "support", description: "" }],
      providers: ["claude"],
      hasEvals: false,
      voice: true,
      voiceProviders: ["claude"],
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

  it("does not pass a call to LiveConsole when the config has no voice-capable providers", async () => {
    fetchOptions.mockResolvedValue({
      agents: [{ taskType: "support", description: "" }],
      providers: ["claude"],
      hasEvals: false,
      voice: false,
      voiceProviders: [],
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
    // No hint either — voice isn't possible at all in this config.
    expect(screen.queryByText(/voice needs a realtime model/i)).not.toBeInTheDocument();
  });

  it("gates voice off and explains when the selected provider can't do voice but another one could", async () => {
    fetchOptions.mockResolvedValue({
      agents: [{ taskType: "support", description: "" }],
      providers: ["claude", "rt-model"],
      hasEvals: false,
      voice: true,
      voiceProviders: ["rt-model"], // config can do voice, but not via "claude"
    });
    createSession.mockResolvedValue({ sessionId: "sess-hint" });

    render(
      <InteractiveChat state={emptyState()} registerInteractiveRun={vi.fn()} onBack={vi.fn()} />,
    );

    // Two providers → not auto-selected; pick the non-voice one explicitly.
    fireEvent.change(await screen.findByDisplayValue("Select provider…"), {
      target: { value: "claude" },
    });
    fireEvent.click(screen.getByText("Start Chat"));

    await waitFor(() => expect(screen.getByPlaceholderText(/Type a message/)).toBeInTheDocument());

    expect(screen.getByText(/voice needs a realtime model/i)).toBeInTheDocument();
    expect(screen.queryByLabelText("Call")).not.toBeInTheDocument();
    const calls = liveConsoleSpy.mock.calls;
    const lastProps = calls[calls.length - 1]?.[0] as { call?: unknown } | undefined;
    expect(lastProps?.call).toBeUndefined();
  });

  // The setup and vars phases moved onto Atlas Select/Checkbox/Input/Alert.
  // These cover the branches that swap in, which the existing tests reached
  // only incidentally on the way to the chat phase.
  describe("setup form", () => {
    const twoOfEach = {
      agents: [
        { taskType: "support", description: "" },
        { taskType: "sales", description: "" },
      ],
      providers: ["claude", "mock"],
      hasEvals: true,
    };

    it("offers agent and provider as labelled selects", async () => {
      fetchOptions.mockResolvedValue(twoOfEach);
      render(<InteractiveChat state={emptyState()} registerInteractiveRun={vi.fn()} onBack={vi.fn()} />);

      const agent = (await screen.findByLabelText("Agent")) as HTMLSelectElement;
      const provider = screen.getByLabelText("Provider") as HTMLSelectElement;
      expect(Array.from(agent.options).map((o) => o.value)).toEqual(["", "support", "sales"]);
      expect(Array.from(provider.options).map((o) => o.value)).toEqual(["", "claude", "mock"]);
    });

    it("passes the chosen agent, provider and evals flag through to the session", async () => {
      fetchOptions.mockResolvedValue(twoOfEach);
      createSession.mockResolvedValue({ sessionId: "s1" });
      render(<InteractiveChat state={emptyState()} registerInteractiveRun={vi.fn()} onBack={vi.fn()} />);

      fireEvent.change(await screen.findByLabelText("Agent"), { target: { value: "sales" } });
      fireEvent.change(screen.getByLabelText("Provider"), { target: { value: "mock" } });
      fireEvent.click(screen.getByLabelText("Run evals per turn"));
      fireEvent.click(screen.getByRole("button", { name: /start chat/i }));

      await waitFor(() =>
        expect(createSession).toHaveBeenCalledWith({
          agent: "sales",
          provider: "mock",
          variables: {},
          evals: true,
        }),
      );
    });

    it("surfaces a session failure as an alert", async () => {
      fetchOptions.mockResolvedValue({ ...twoOfEach, hasEvals: false });
      createSession.mockResolvedValue({ error: "provider unreachable" });
      render(<InteractiveChat state={emptyState()} registerInteractiveRun={vi.fn()} onBack={vi.fn()} />);

      fireEvent.change(await screen.findByLabelText("Agent"), { target: { value: "sales" } });
      fireEvent.change(screen.getByLabelText("Provider"), { target: { value: "mock" } });
      fireEvent.click(screen.getByRole("button", { name: /start chat/i }));

      const alert = await screen.findByRole("alert");
      expect(alert).toHaveTextContent("provider unreachable");
    });

    it("collects missing variables, then retries the session with them", async () => {
      fetchOptions.mockResolvedValue({ ...twoOfEach, hasEvals: false });
      createSession
        .mockResolvedValueOnce({ missingVars: ["customer_name"] })
        .mockResolvedValueOnce({ sessionId: "s2" });
      render(<InteractiveChat state={emptyState()} registerInteractiveRun={vi.fn()} onBack={vi.fn()} />);

      fireEvent.change(await screen.findByLabelText("Agent"), { target: { value: "sales" } });
      fireEvent.change(screen.getByLabelText("Provider"), { target: { value: "mock" } });
      fireEvent.click(screen.getByRole("button", { name: /start chat/i }));

      // Vars phase: one Input per missing variable.
      const field = await screen.findByLabelText("customer_name");
      fireEvent.change(field, { target: { value: "Ada" } });
      fireEvent.click(screen.getByRole("button", { name: /start chat/i }));

      await waitFor(() =>
        expect(createSession).toHaveBeenLastCalledWith({
          agent: "sales",
          provider: "mock",
          variables: { customer_name: "Ada" },
          evals: false,
        }),
      );
    });

    it("reports a failure to load options at all", async () => {
      fetchOptions.mockRejectedValue(new Error("options endpoint down"));
      render(<InteractiveChat state={emptyState()} registerInteractiveRun={vi.fn()} onBack={vi.fn()} />);
      expect(await screen.findByRole("alert")).toHaveTextContent("options endpoint down");
    });
  });
});
