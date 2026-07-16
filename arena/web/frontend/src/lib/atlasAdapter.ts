// PREVIEW-GRADE adapter: Arena RunResult/Message → Atlas model.
// WU-2 hardens this (TDD, live-SSE camelCase bridge, media refs). For now it
// covers the historical path well enough to render SessionReview on a real run.
import type { AtlasMessage, AtlasCheck, AtlasContentPart, AtlasToolCall } from "@altairalabs/atlas";
import type { Message, RunResult, ContentPart, EvalResult } from "@/types";

const ROLES = new Set(["user", "assistant", "system", "tool"]);
const toRole = (r: string): AtlasMessage["role"] =>
  (ROLES.has(r) ? r : "assistant") as AtlasMessage["role"];

// Preview keeps parts text-only (llm-judge is text); media lands in WU-2.
function partOf(p: ContentPart): AtlasContentPart | null {
  return p.text ? { type: "text", text: p.text } : null;
}
function partsOf(m: Message): AtlasContentPart[] {
  if (m.parts?.length) return m.parts.map(partOf).filter(Boolean) as AtlasContentPart[];
  return m.content ? [{ type: "text", text: m.content }] : [];
}

function toolCallsOf(m: Message): AtlasToolCall[] | undefined {
  if (!m.tool_calls?.length) return undefined;
  return m.tool_calls.map((c) => {
    const res = m.tool_result && m.tool_result.id === c.id ? m.tool_result : undefined;
    return {
      id: c.id,
      callId: c.id,
      name: c.name,
      args: c.args,
      status: res ? (res.error ? "error" : "success") : "success",
      error: res?.error,
      result: res?.parts?.map(partOf).filter(Boolean) as AtlasContentPart[] | undefined,
      durationMs: res?.latency_ms,
    };
  });
}

function metricsOf(m: Message): AtlasMessage["metrics"] {
  if (m.latency_ms == null && !m.cost_info) return undefined;
  return {
    latencyMs: m.latency_ms,
    inputTokens: m.cost_info?.input_tokens,
    outputTokens: m.cost_info?.output_tokens,
    costUsd: m.cost_info?.total_cost_usd,
  };
}

// An eval result → a scored Atlas check (assertion when value is a bool).
function evalToCheck(e: EvalResult): AtlasCheck {
  const isBool = typeof e.value === "boolean";
  return {
    type: e.type || e.eval_id,
    kind: isBool ? "assertion" : "eval",
    passed: isBool ? (e.value as boolean) : undefined,
    score: e.score,
    explanation: e.explanation || e.message,
    durationMs: e.duration_ms,
    error: e.error,
    skipped: e.skipped,
    skipReason: e.skip_reason,
    violations: e.violations?.map((v) => ({ turnIndex: v.turn_index, description: v.description ?? "", evidence: v.evidence })),
    turnIndex: e.turn_index,
  };
}

// Turn-level checks: per-message validators (as guardrails) + evals scoped to this turn.
function messageChecks(m: Message, i: number, run: RunResult): AtlasCheck[] {
  const out: AtlasCheck[] = [];
  for (const v of m.validations ?? []) {
    out.push({ type: v.validator_type, kind: "guardrail", passed: v.passed, action: v.passed ? "allow" : "block" });
  }
  for (const e of run.eval_results ?? []) {
    if (e.turn_index === i) out.push(evalToCheck(e));
  }
  return out;
}

// Conversation-level checks: gating assertions + session-scoped evals.
function conversationChecks(run: RunResult): AtlasCheck[] {
  const out: AtlasCheck[] = [];
  for (const r of run.ConversationAssertions?.results ?? []) {
    out.push({
      type: r.name || r.type,
      kind: "assertion",
      passed: r.passed,
      score: r.score,
      explanation: r.message,
      violations: r.violations?.map((v) => ({ turnIndex: v.turn_index, description: v.description, evidence: v.evidence })),
    });
  }
  for (const e of run.eval_results ?? []) {
    if (e.turn_index == null) out.push(evalToCheck(e));
  }
  return out;
}

function tsOf(m: Message, i: number, baseMs: number): string {
  if (m.timestamp) return m.timestamp;
  return new Date(baseMs + i * 1000).toISOString();
}

export function adaptMessage(m: Message, i: number, run: RunResult, baseMs: number): AtlasMessage {
  const checks = messageChecks(m, i, run);
  return {
    id: `m${i}`,
    role: toRole(m.role),
    sequenceNum: i,
    timestamp: tsOf(m, i, baseMs),
    parts: partsOf(m),
    toolCalls: toolCallsOf(m),
    metrics: metricsOf(m),
    checks: checks.length ? checks : undefined,
  };
}

export function adaptRun(run: RunResult): { title: string; messages: AtlasMessage[]; checks?: AtlasCheck[] } {
  const baseMs = Date.parse(run.StartTime) || Date.now();
  const messages = (run.Messages ?? []).map((m, i) => adaptMessage(m, i, run, baseMs));
  const checks = conversationChecks(run);
  return {
    title: `${run.ScenarioID} · ${run.ProviderID}`,
    messages,
    checks: checks.length ? checks : undefined,
  };
}
