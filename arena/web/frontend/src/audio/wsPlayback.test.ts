import { describe, it, expect, beforeEach } from "vitest";
import { WsPlayback } from "./wsPlayback";

// Minimal AudioContext fake capturing scheduled sources.
class FakeSource {
  buffer: unknown = null;
  started = false;
  stopped = false;
  onended: (() => void) | null = null;
  connect() {}
  start() {
    this.started = true;
  }
  stop() {
    this.stopped = true;
  }
}
function fakeCtx() {
  const sources: FakeSource[] = [];
  return {
    sources,
    currentTime: 0,
    destination: {},
    createBuffer: (_ch: number, len: number, _rate: number) => ({
      length: len,
      duration: len / 24000,
      copyToChannel: () => {},
    }),
    createBufferSource: () => {
      const s = new FakeSource();
      sources.push(s);
      return s;
    },
  } as unknown as AudioContext & { sources: FakeSource[] };
}

describe("WsPlayback", () => {
  let ctx: AudioContext & { sources: FakeSource[] };
  beforeEach(() => {
    ctx = fakeCtx() as never;
  });

  it("schedules a source per enqueued frame", () => {
    const p = new WsPlayback(ctx);
    p.enqueue(new Int16Array([1, 2, 3, 4]).buffer);
    expect(ctx.sources.length).toBe(1);
    expect(ctx.sources[0].started).toBe(true);
  });

  it("flush stops queued sources", () => {
    const p = new WsPlayback(ctx);
    p.enqueue(new Int16Array([1, 2]).buffer);
    p.flush();
    expect(ctx.sources[0].stopped).toBe(true);
  });
});
