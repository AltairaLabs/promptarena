import { describe, it, expect, vi } from "vitest";
import { VoiceTransport } from "./voiceTransport";

class FakeWS {
  static OPEN = 1;
  readyState = 1;
  binaryType = "arraybuffer";
  sent: unknown[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(public url: string) {}
  send(d: unknown) {
    this.sent.push(d);
  }
  close() {
    this.onclose?.();
  }
}

function make() {
  const playback = { enqueue: vi.fn(), flush: vi.fn() };
  const states: string[] = [];
  let ws!: FakeWS;
  const t = new VoiceTransport({
    url: "ws://x/api/interactive/voice?session=s",
    playback,
    onState: (s) => states.push(s),
    onError: () => states.push("error"),
    wsFactory: (u) => {
      ws = new FakeWS(u);
      return ws as unknown as WebSocket;
    },
  });
  return { t, playback, states, getWs: () => ws };
}

describe("VoiceTransport", () => {
  it("routes state frames to onState", () => {
    const { t, states, getWs } = make();
    t.connect();
    getWs().onmessage?.({ data: JSON.stringify({ type: "state", state: "speaking" }) } as MessageEvent);
    expect(states).toContain("speaking");
  });

  it("routes flush frames to playback.flush", () => {
    const { t, playback, getWs } = make();
    t.connect();
    getWs().onmessage?.({ data: JSON.stringify({ type: "flush" }) } as MessageEvent);
    expect(playback.flush).toHaveBeenCalled();
  });

  it("enqueues binary playback frames", () => {
    const { t, playback, getWs } = make();
    t.connect();
    const buf = new Int16Array([1, 2]).buffer;
    getWs().onmessage?.({ data: buf } as MessageEvent);
    expect(playback.enqueue).toHaveBeenCalledWith(buf);
  });

  it("drops mic sends when muted", () => {
    const { t, getWs } = make();
    t.connect();
    getWs().onopen?.();
    t.setMuted(true);
    t.sendPcm(new Int16Array([1]).buffer);
    // only the mute control frame was sent, no binary
    const binarySends = getWs().sent.filter((s) => s instanceof ArrayBuffer);
    expect(binarySends.length).toBe(0);
  });
});
