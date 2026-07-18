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
});
