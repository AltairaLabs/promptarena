import { describe, it, expect } from "vitest";
import { rms, frameToPcm16le } from "./micCapture";

describe("rms", () => {
  it("is 0 for silence", () => {
    expect(rms(new Float32Array(128))).toBe(0);
  });
  it("is ~1 for full-scale", () => {
    expect(rms(new Float32Array([1, -1, 1, -1]))).toBeCloseTo(1, 5);
  });
});

describe("frameToPcm16le", () => {
  it("produces 2 bytes per output sample at 16k", () => {
    const frame = new Float32Array(160); // already 16k
    const buf = frameToPcm16le(frame, 16000);
    expect(buf.byteLength).toBe(320);
  });
  it("halves length from 32k input", () => {
    const frame = new Float32Array(320);
    const buf = frameToPcm16le(frame, 32000);
    expect(buf.byteLength).toBe(320); // 160 samples * 2 bytes
  });
});
