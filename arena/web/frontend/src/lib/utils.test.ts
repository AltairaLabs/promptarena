import { describe, it, expect } from "vitest";
import { cn, formatDuration, toDisplayString } from "./utils";

describe("cn", () => {
  it("concatenates space-separated class names", () => {
    expect(cn("a", "b", "c")).toBe("a b c");
  });

  it("drops falsy values", () => {
    expect(cn("a", false, null, undefined, "b")).toBe("a b");
  });

  it("resolves conflicting tailwind classes by keeping the last", () => {
    expect(cn("p-2", "p-4")).toBe("p-4");
  });

  it("returns empty string when given no arguments", () => {
    expect(cn()).toBe("");
  });
});

describe("formatDuration", () => {
  it("shows <1ms for sub-millisecond durations", () => {
    expect(formatDuration(0.634)).toBe("<1ms");
    expect(formatDuration(0)).toBe("<1ms");
  });

  it("shows whole milliseconds under 1s", () => {
    expect(formatDuration(820)).toBe("820ms");
    expect(formatDuration(1)).toBe("1ms");
    expect(formatDuration(999)).toBe("999ms");
  });

  it("shows one decimal of seconds under 10s", () => {
    expect(formatDuration(5000)).toBe("5.0s");
    expect(formatDuration(1234)).toBe("1.2s");
  });

  it("shows whole seconds from 10s up to a minute", () => {
    expect(formatDuration(45000)).toBe("45s");
    expect(formatDuration(59999)).toBe("60s");
  });

  it("rolls over into minutes and seconds at 60s and above", () => {
    expect(formatDuration(633792)).toBe("10m 34s");
    expect(formatDuration(60000)).toBe("1m 0s");
  });
});

describe("toDisplayString", () => {
  it("returns string values unchanged", () => {
    expect(toDisplayString("openai", "unknown")).toBe("openai");
  });

  it("returns empty strings unchanged (does not treat them as missing)", () => {
    expect(toDisplayString("", "fallback")).toBe("");
  });

  it("coerces numbers via String()", () => {
    expect(toDisplayString(42, "fallback")).toBe("42");
    expect(toDisplayString(0, "fallback")).toBe("0");
    expect(toDisplayString(-1.5, "fallback")).toBe("-1.5");
  });

  it("coerces booleans via String()", () => {
    expect(toDisplayString(true, "fallback")).toBe("true");
    expect(toDisplayString(false, "fallback")).toBe("false");
  });

  it("coerces bigints via String()", () => {
    expect(toDisplayString(9007199254740993n, "fallback")).toBe("9007199254740993");
  });

  it("uses the fallback for null", () => {
    expect(toDisplayString(null, "fallback")).toBe("fallback");
  });

  it("uses the fallback for undefined", () => {
    expect(toDisplayString(undefined, "fallback")).toBe("fallback");
  });

  it("reads .message from Error instances", () => {
    expect(toDisplayString(new Error("boom"), "fallback")).toBe("boom");
  });

  it("reads .message from Error-like plain objects", () => {
    expect(toDisplayString({ message: "bad things" }, "fallback")).toBe("bad things");
  });

  it("falls through to JSON.stringify when .message is an empty string", () => {
    // Empty .message is useless — prefer the full object shape.
    expect(toDisplayString({ message: "", code: 500 }, "fallback")).toBe(
      '{"message":"","code":500}',
    );
  });

  it("falls through to JSON.stringify when .message is not a string", () => {
    expect(toDisplayString({ message: 123 }, "fallback")).toBe('{"message":123}');
  });

  it("JSON-stringifies plain objects", () => {
    expect(toDisplayString({ provider: "openai", model: "gpt-4" }, "fallback")).toBe(
      '{"provider":"openai","model":"gpt-4"}',
    );
  });

  it("JSON-stringifies arrays", () => {
    expect(toDisplayString(["a", "b", 3], "fallback")).toBe('["a","b",3]');
  });

  it("returns the fallback when JSON.stringify throws (e.g. circular refs)", () => {
    const circular: Record<string, unknown> = { name: "loop" };
    circular.self = circular;
    expect(toDisplayString(circular, "fallback")).toBe("fallback");
  });

  it("returns the fallback for symbols (unsupported primitive)", () => {
    expect(toDisplayString(Symbol("x"), "fallback")).toBe("fallback");
  });

  it("returns the fallback for functions", () => {
    // typeof function === "function" — not string/number/boolean/bigint/object.
    expect(toDisplayString(() => 1, "fallback")).toBe("fallback");
  });
});
