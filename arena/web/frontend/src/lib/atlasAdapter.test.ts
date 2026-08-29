import { describe, it, expect } from "vitest";
import { adaptMessage, adaptRun, adaptLiveMessages, conversationChecks, adaptWorkflow } from "./atlasAdapter";
import type { LiveMessage, Message, RunResult, WorkflowGraph } from "@/types";

const run = (over: Partial<RunResult> = {}): RunResult =>
  ({ RunID: "r", ScenarioID: "helpfulness", ProviderID: "mock", StartTime: "2026-07-03T12:52:15Z", Messages: [], Error: "", ...over } as unknown as RunResult);

const msg = (over: Partial<Message> = {}): Message => ({ role: "assistant", content: "hi", ...over } as Message);

// Every path holding an explicit `undefined`, so a failure names the field.
const undefinedKeysIn = (v: unknown, path = "$", out: string[] = []): string[] => {
  if (Array.isArray(v)) v.forEach((x, i) => undefinedKeysIn(x, `${path}[${i}]`, out));
  else if (v && typeof v === "object")
    for (const [k, val] of Object.entries(v)) {
      if (val === undefined) out.push(`${path}.${k}`);
      else undefinedKeysIn(val, `${path}.${k}`, out);
    }
  return out;
};

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

  // What the guardrail actually caught lives in details.value.violations. Arena
  // also flattens it onto run.Violations, but stringified through Go's map
  // formatter ("map[score:0 value:map[violations:[...]]]"), so the per-message
  // structure is the only usable source.
  it("carries what a guardrail caught, so the Inspector can show it", () => {
    const m = msg({
      validations: [
        {
          validator_type: "banned_words",
          passed: false,
          details: { score: 0, validator_type: "banned_words", value: { violations: ['contains "damn"', 'contains "hell"'] } },
        },
      ],
    });
    const check = adaptMessage(m, 3, run(), 0).checks![0];
    expect(check.violations).toEqual([
      { turnIndex: 3, description: 'contains "damn"' },
      { turnIndex: 3, description: 'contains "hell"' },
    ]);
    // Not duplicated into explanation — every surface renders both, adjacent.
    expect(check.explanation).toBeUndefined();
  });

  it("leaves a guardrail with no violation detail untouched", () => {
    const a = adaptMessage(msg({ validations: [{ validator_type: "length", passed: true }] }), 0, run(), 0);
    expect(a.checks![0].violations).toBeUndefined();
    expect(a.checks![0].explanation).toBeUndefined();
  });

  // Atlas's JsonView prints an undefined-valued key as a literal `undefined`
  // line rather than omitting it, so every optional field left unset becomes a
  // row of noise in the Inspector's Raw tab.
  it("emits no undefined-valued keys, at any depth", () => {
    const a = adaptMessage(msg({ cost_info: { input_tokens: 6, total_cost_usd: 0.1 } as never }), 0, run(), 0);
    expect(undefinedKeysIn(a)).toEqual([]);
  });

  it("keeps meta as-is, without deep-copying it", () => {
    const meta = { _llm_trace: { big: "payload" } };
    const a = adaptMessage(msg({ meta: meta as never }), 0, run(), 0);
    expect(a.meta).toBe(meta);
  });

  it("attaches a run error to the last message", () => {
    const r = run({ Messages: [msg(), msg()] as Message[], Error: "provider timeout" });
    expect(adaptMessage(msg(), 1, r, 0).error?.message).toBe("provider timeout");
    expect(adaptMessage(msg(), 0, r, 0).error).toBeUndefined();
  });

  it("surfaces a tool-result message's output when content is empty (no blank tool bubble)", () => {
    const toolMsg = msg({
      role: "tool",
      content: "",
      tool_result: { id: "t1", name: "workflow__transition", parts: [{ type: "text", text: '{"event":"More"}' }], latency_ms: 0 },
    });
    const a = adaptMessage(toolMsg, 3, run(), 0);
    expect(a.role).toBe("tool");
    expect(a.parts).toEqual([{ type: "text", text: '{"event":"More"}' }]);
  });
});

describe("adaptLiveMessages", () => {
  it("populates metrics/meta on a live message once the SSE message.full upsert supplies cost_info/meta (proves the live Inspector renders them)", () => {
    const thin: LiveMessage = { index: 0, role: "user", content: "hi" };
    const full: LiveMessage = {
      index: 1,
      role: "assistant",
      content: "hello",
      latency_ms: 812,
      cost_info: { input_tokens: 10, output_tokens: 5, input_cost_usd: 0, output_cost_usd: 0, total_cost_usd: 0.001 },
      meta: { persona: "support" },
    };
    const [a0, a1] = adaptLiveMessages([thin, full]);
    expect(a0.metrics).toBeUndefined();
    expect(a0.meta).toBeUndefined();
    expect(a1.metrics).toMatchObject({ latencyMs: 812, inputTokens: 10, outputTokens: 5, costUsd: 0.001 });
    expect(a1.meta).toEqual({ persona: "support" });
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

  it("rewrites zero-time and duplicate timestamps into a strictly increasing sequence (scrubber range)", () => {
    const r = run({
      StartTime: "2026-07-03T12:52:15Z",
      Messages: [
        msg({ role: "user", content: "q", timestamp: "2026-07-03T12:52:15.100Z" }),
        msg({ role: "assistant", content: "a", timestamp: "2026-07-03T12:52:15.100Z" }), // duplicate ts
        msg({ role: "tool", content: "", timestamp: "0001-01-01T00:00:00Z", tool_result: { id: "t", name: "x", parts: [{ type: "text", text: "{}" }], latency_ms: 0 } }), // Go zero time
        msg({ role: "assistant", content: "b", timestamp: "2026-07-03T12:52:15.200Z" }),
      ] as Message[],
    });
    const ts = adaptRun(r).messages.map((m) => Date.parse(m.timestamp));
    // Strictly increasing — no collapsed/duplicate points.
    for (let i = 1; i < ts.length; i++) expect(ts[i]).toBeGreaterThan(ts[i - 1]);
    // The year-0001 sentinel never survives to poison the range.
    expect(Math.min(...ts)).toBeGreaterThan(Date.parse("2000-01-01T00:00:00Z"));
  });
});

describe("adaptWorkflow", () => {
  const graph: WorkflowGraph = {
    nodes: [
      { id: "s1", label: "start", kind: "entry", entry: true, terminal: false },
      { id: "a1", label: "agent", kind: "agent", entry: false, terminal: false },
      { id: "step1", label: "prompt", kind: "prompt", entry: false, terminal: false, parent: "a1" },
    ],
    edges: [{ from: "s1", to: "a1", gold: true }],
  };
  it("maps kinds 1:1 and renames from/to → source/target", () => {
    const { nodes, edges } = adaptWorkflow(graph);
    expect(nodes.find((n) => n.id === "s1")).toMatchObject({ kind: "entry", label: "start" });
    expect(edges.find((e) => e.source === "s1" && e.target === "a1")).toMatchObject({ gold: true });
  });
  it("marks a parent-referenced node as a group container", () => {
    const { nodes } = adaptWorkflow(graph);
    expect(nodes.find((n) => n.id === "a1")?.group).toBe(true);
    expect(nodes.find((n) => n.id === "step1")?.parent).toBe("a1");
  });
  // Not `terminator` for either: Atlas deprecated that kind because it was
  // doing double duty, and it now renders as `output` — which would draw our
  // START as a final state. They were always an entry and an output.
  it("bookends the graph with an entry and an output wired to entry/terminal nodes", () => {
    const { nodes, edges } = adaptWorkflow(graph);
    expect(nodes.find((n) => n.id === "__start")).toMatchObject({ kind: "entry" });
    expect(nodes.find((n) => n.id === "__end")).toMatchObject({ kind: "output" });
    expect(edges.some((e) => e.source === "__start" && e.target === "s1")).toBe(true);
  });
});

describe("reasoning", () => {
  it("maps a turn's thinking onto the Atlas message so the disclosure renders", () => {
    const a = adaptMessage(msg({ reasoning: { text: "D=5, C=15, B=11, A=22" } }), 0, run(), 0);
    expect(a.reasoning).toEqual({ text: "D=5, C=15, B=11, A=22", redacted: undefined });
  });

  it("omits reasoning when a turn produced none, rather than an empty disclosure", () => {
    expect(adaptMessage(msg(), 0, run(), 0).reasoning).toBeUndefined();
    expect(adaptMessage(msg({ reasoning: { text: "" } }), 0, run(), 0).reasoning).toBeUndefined();
  });

  it("carries reasoning on the live path too, not just historical results", () => {
    const [a] = adaptLiveMessages([msg({ reasoning: { text: "live thinking" } })]);
    expect(a.reasoning?.text).toBe("live thinking");
  });

  it("keeps a redacted trace, which has no text but must still surface", () => {
    const a = adaptMessage(msg({ reasoning: { redacted: true } }), 0, run(), 0);
    expect(a.reasoning?.redacted).toBe(true);
  });
});
