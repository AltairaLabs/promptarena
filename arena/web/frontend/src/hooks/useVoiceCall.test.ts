import { describe, it, expect, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useVoiceCall, voiceFactories } from "./useVoiceCall";

describe("useVoiceCall", () => {
  it("starts idle and transitions to connecting on onCall", () => {
    const transport = {
      connect: vi.fn(),
      sendPcm: vi.fn(),
      setMuted: vi.fn(),
      close: vi.fn(),
    };
    const mic = { start: vi.fn().mockResolvedValue(undefined), stop: vi.fn() };
    vi.spyOn(voiceFactories, "makeTransport").mockImplementation((o) => {
      // drive to "connecting" as the real one does
      o.onState("connecting");
      return transport as never;
    });
    vi.spyOn(voiceFactories, "makeMic").mockReturnValue(mic as never);
    vi.spyOn(voiceFactories, "makePlayback").mockReturnValue({ enqueue: vi.fn(), flush: vi.fn(), close: vi.fn() } as never);

    const { result } = renderHook(() => useVoiceCall({ sessionId: "s1", enabled: true }));
    expect(result.current.state).toBe("idle");

    act(() => result.current.onCall());
    expect(mic.start).toHaveBeenCalled();
    expect(transport.connect).toHaveBeenCalled();
    expect(result.current.state).toBe("connecting");
  });

  it("toggles mute through the transport", () => {
    const transport = { connect: vi.fn(), sendPcm: vi.fn(), setMuted: vi.fn(), close: vi.fn() };
    vi.spyOn(voiceFactories, "makeTransport").mockReturnValue(transport as never);
    vi.spyOn(voiceFactories, "makeMic").mockReturnValue({ start: vi.fn().mockResolvedValue(undefined), stop: vi.fn() } as never);
    vi.spyOn(voiceFactories, "makePlayback").mockReturnValue({ enqueue: vi.fn(), flush: vi.fn(), close: vi.fn() } as never);

    const { result } = renderHook(() => useVoiceCall({ sessionId: "s1", enabled: true }));
    act(() => result.current.onCall());
    act(() => result.current.onToggleMute());
    expect(transport.setMuted).toHaveBeenCalledWith(true);
    expect(result.current.muted).toBe(true);
  });

  it("does not downgrade an error state to idle from a trailing callback", () => {
    let callbacks: { onState: (s: string) => void; onError: (m: string) => void } | undefined;
    const transport = { connect: vi.fn(), sendPcm: vi.fn(), setMuted: vi.fn(), close: vi.fn() };
    vi.spyOn(voiceFactories, "makeTransport").mockImplementation((o) => {
      callbacks = { onState: o.onState as never, onError: o.onError as never };
      return transport as never;
    });
    vi.spyOn(voiceFactories, "makeMic").mockReturnValue({ start: vi.fn().mockResolvedValue(undefined), stop: vi.fn() } as never);
    vi.spyOn(voiceFactories, "makePlayback").mockReturnValue({ enqueue: vi.fn(), flush: vi.fn(), close: vi.fn() } as never);

    const { result } = renderHook(() => useVoiceCall({ sessionId: "s1", enabled: true }));
    act(() => result.current.onCall());

    act(() => {
      callbacks!.onError("boom");
      callbacks!.onState("idle");
    });

    expect(result.current.state).toBe("error");
    expect(result.current.errorMessage).toBe("boom");
  });

  it("tears down the first instances when onCall is invoked twice (no leak)", () => {
    const mics: Array<{ start: ReturnType<typeof vi.fn>; stop: ReturnType<typeof vi.fn> }> = [];
    const transports: Array<{ connect: ReturnType<typeof vi.fn>; sendPcm: ReturnType<typeof vi.fn>; setMuted: ReturnType<typeof vi.fn>; close: ReturnType<typeof vi.fn> }> = [];
    const playbacks: Array<{ enqueue: ReturnType<typeof vi.fn>; flush: ReturnType<typeof vi.fn>; close: ReturnType<typeof vi.fn> }> = [];

    vi.spyOn(voiceFactories, "makeTransport").mockImplementation(() => {
      const t = { connect: vi.fn(), sendPcm: vi.fn(), setMuted: vi.fn(), close: vi.fn() };
      transports.push(t);
      return t as never;
    });
    vi.spyOn(voiceFactories, "makeMic").mockImplementation(() => {
      const m = { start: vi.fn().mockResolvedValue(undefined), stop: vi.fn() };
      mics.push(m);
      return m as never;
    });
    vi.spyOn(voiceFactories, "makePlayback").mockImplementation(() => {
      const p = { enqueue: vi.fn(), flush: vi.fn(), close: vi.fn() };
      playbacks.push(p);
      return p as never;
    });

    const { result } = renderHook(() => useVoiceCall({ sessionId: "s1", enabled: true }));
    act(() => {
      result.current.onCall();
      result.current.onCall();
    });

    expect(mics).toHaveLength(2);
    expect(transports).toHaveLength(2);
    expect(mics[0].stop).toHaveBeenCalled();
    expect(transports[0].close).toHaveBeenCalled();
    expect(mics[1].stop).not.toHaveBeenCalled();
    expect(transports[1].close).not.toHaveBeenCalled();
  });

  it("allows retry after error", () => {
    let callbacks: { onState: (s: string) => void; onError: (m: string) => void } | undefined;
    const mics: Array<{ start: ReturnType<typeof vi.fn>; stop: ReturnType<typeof vi.fn> }> = [];

    vi.spyOn(voiceFactories, "makeTransport").mockImplementation((o) => {
      callbacks = { onState: o.onState as never, onError: o.onError as never };
      return { connect: vi.fn(), sendPcm: vi.fn(), setMuted: vi.fn(), close: vi.fn() } as never;
    });
    vi.spyOn(voiceFactories, "makeMic").mockImplementation(() => {
      const m = { start: vi.fn().mockResolvedValue(undefined), stop: vi.fn() };
      mics.push(m);
      return m as never;
    });
    vi.spyOn(voiceFactories, "makePlayback").mockReturnValue({ enqueue: vi.fn(), flush: vi.fn(), close: vi.fn() } as never);

    const { result } = renderHook(() => useVoiceCall({ sessionId: "s1", enabled: true }));
    act(() => result.current.onCall());
    act(() => callbacks!.onError("boom"));
    expect(result.current.state).toBe("error");

    act(() => result.current.onCall());
    act(() => callbacks!.onState("connecting"));

    expect(mics).toHaveLength(2);
    expect(mics[1].start).toHaveBeenCalled();
    expect(result.current.state).not.toBe("error");
    expect(result.current.state).toBe("connecting");
  });
});
