import { describe, it, expect } from "vitest";
import { float32ToInt16, downsampleTo16k } from "./pcm";

describe("float32ToInt16", () => {
  it("maps [-1,1] to full int16 range", () => {
    const out = float32ToInt16(new Float32Array([0, 1, -1, 0.5]));
    expect(out[0]).toBe(0);
    expect(out[1]).toBe(32767);
    expect(out[2]).toBe(-32768);
    expect(out[3]).toBeCloseTo(16383, -1);
  });
});

describe("downsampleTo16k", () => {
  it("halves a 32k input length", () => {
    const input = new Float32Array(320); // 10ms @ 32k
    const out = downsampleTo16k(input, 32000);
    expect(out.length).toBe(160); // 10ms @ 16k
  });
  it("returns input unchanged at 16k", () => {
    const input = new Float32Array([0.1, 0.2, 0.3]);
    const out = downsampleTo16k(input, 16000);
    expect(Array.from(out)).toEqual([0.1, 0.2, 0.3].map((v) => expect.closeTo(v)));
  });
});
