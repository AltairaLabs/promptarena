import { describe, it, expect } from "vitest";
import { adaptMessage, adaptRun, conversationChecks } from "./atlasAdapter";
import type { Message, RunResult } from "@/types";

const run = (over: Partial<RunResult> = {}): RunResult =>
  ({ RunID: "r", ScenarioID: "helpfulness", ProviderID: "mock", StartTime: "2026-07-03T12:52:15Z", Messages: [], Error: "", ...over } as unknown as RunResult);

const msg = (over: Partial<Message> = {}): Message => ({ role: "assistant", content: "hi", ...over } as Message);

describe("adaptMessage", () => {
  it("maps role, content fallback, and sequence", () => {
    const a = adaptMessage(msg({ role: "weird", content: "yo" }), 2, run(), 0);
    expect(a.role).toBe("assistant"); // unknown role → assistant
    expect(a.sequenceNum).toBe(2);
    expect(a.parts).toEqual([{ type: "text", text: "yo" }]);
  });

  it("derives latency from cost_info.latency_ns (nanoseconds)", () => {
    const a = adaptMessage(msg({ cost_info: { input_tokens: 6, output_tokens: 33, input_cost_usd: 0, output_cost_usd: 0, total_cost_usd: 0.0004, latency_ns: 25_000_000 } as never }), 0, run(), 0);
    expect(a.metrics?.latencyMs).toBe(25);
    expect(a.metrics?.outputTokens).toBe(33);
    expect(a.metrics?.costUsd).toBeCloseTo(0.0004);
  });

  it("reads per-message checks from meta.assertions.results", () => {
    const m = msg({
      meta: {
        assertions: {
          results: [
            { type: "assertion", passed: true, message: "should be helpful", config: { params: { eval_type: "llm_judge", min_score: 0.8 } }, details: { score: 0.85, value: true } },
          ],
        },
      } as never,
    });
    const a = adaptMessage(m, 0, run(), 0);
    expect(a.checks).toHaveLength(1);
    expect(a.checks![0]).toMatchObject({ type: "llm_judge", kind: "assertion", passed: true, score: 0.85, explanation: "should be helpful" });
  });

  it("maps a message validator to a guardrail check", () => {
    const a = adaptMessage(msg({ validations: [{ validator_type: "pii", passed: false }] }), 0, run(), 0);
    expect(a.checks![0]).toMatchObject({ type: "pii", kind: "guardrail", passed: false, action: "block" });
  });

  it("attaches a run error to the last message", () => {
    const r = run({ Messages: [msg(), msg()] as Message[], Error: "provider timeout" });
    expect(adaptMessage(msg(), 1, r, 0).error?.message).toBe("provider timeout");
    expect(adaptMessage(msg(), 0, r, 0).error).toBeUndefined();
  });
});

describe("conversationChecks", () => {
  it("reads conversation_assertions (snake wire)", () => {
    const r = run({ conversation_assertions: { results: [{ type: "assertion", passed: true, message: "helpful overall", details: { score: 0.9, value: true } }] } } as never);
    const cs = conversationChecks(r);
    expect(cs).toHaveLength(1);
    expect(cs[0]).toMatchObject({ passed: true, score: 0.9, explanation: "helpful overall" });
  });

  it("falls back to the PascalCase ConversationAssertions field", () => {
    const r = run({ ConversationAssertions: { failed: 0, passed: true, total: 1, results: [{ type: "assertion", passed: false, message: "off topic" }] } });
    expect(conversationChecks(r)[0]).toMatchObject({ passed: false, explanation: "off topic" });
  });

  it("is empty when neither is present", () => {
    expect(conversationChecks(run())).toEqual([]);
  });
});

describe("adaptRun", () => {
  it("titles by scenario·provider and adapts every message", () => {
    const r = run({ Messages: [msg({ role: "user", content: "q" }), msg({ role: "assistant", content: "a" })] as Message[] });
    const out = adaptRun(r);
    expect(out.title).toBe("helpfulness · mock");
    expect(out.messages).toHaveLength(2);
  });

  it("exposes a recording url when RecordingPath is set", () => {
    const out = adaptRun(run({ RecordingPath: "sessions/abc.wav" } as never));
    expect(out.recording?.src).toBe("/api/media/sessions/abc.wav");
  });
});
