// Adapter: Arena RunResult/Message → Atlas model. The one place that knows
// Arena's wire shapes — mixed casing (RunResult PascalCase, Message snake_case),
// nanosecond latencies, and checks that live in meta.assertions / conversation_
// assertions rather than a single field. Covers the historical path; the live
// SSE camelCase bridge and derived trace events land in later WUs.
import type { AtlasMessage, AtlasCheck, AtlasContentPart, AtlasToolCall, AtlasCheckViolation, ConstellationNode, ConstellationEdge } from "@altairalabs/atlas";
import type { Message, RunResult, ContentPart, EvalResult, WorkflowGraph } from "@/types";

const ROLES = new Set(["user", "assistant", "system", "tool"]);
const toRole = (r: string): AtlasMessage["role"] => (ROLES.has(r) ? r : "assistant") as AtlasMessage["role"];
const MEDIA = new Set(["image", "audio", "video", "document"]);

// ---- content parts ----

function partOf(p: ContentPart): AtlasContentPart | null {
  if (MEDIA.has(p.type) && p.media) {
    return {
      type: p.type as "image" | "audio" | "video" | "document",
      media: {
        url: p.media.url,
        storageRef: p.media.storage_reference,
        mimeType: p.media.mime_type ?? "application/octet-stream",
        fileName: p.media.file_path,
        sizeBytes: p.media.size_bytes,
        width: p.media.width,
        height: p.media.height,
        durationMs: p.media.duration != null ? p.media.duration * 1000 : undefined,
      },
    };
  }
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

// ---- checks ----
// Arena serialises assertion/eval/guardrail results in several shapes. The
// message-level `meta.assertions.results` and run-level `conversation_assertions.
// results` share this loose shape; EvalResult (`eval_results`) is separate.

interface RawAssertion {
  type?: string;
  name?: string;
  passed?: boolean;
  score?: number;
  message?: string;
  action?: AtlasCheck["action"];
  details?: { score?: number; value?: unknown; explanation?: string; metric_value?: number; passed?: boolean };
  config?: { params?: { eval_type?: string; action?: AtlasCheck["action"] } };
  violations?: { turn_index?: number; description?: string; evidence?: Record<string, unknown> }[];
}

function violationsOf(vs?: RawAssertion["violations"]): AtlasCheckViolation[] | undefined {
  return vs?.map((v) => ({ turnIndex: v.turn_index, description: v.description ?? "", evidence: v.evidence }));
}

function assertionToCheck(r: RawAssertion): AtlasCheck {
  const evalType = r.config?.params?.eval_type;
  const action = r.action ?? r.config?.params?.action;
  const kind: AtlasCheck["kind"] = evalType === "guardrail" || action ? "guardrail" : "assertion";
  const passed = r.passed ?? (typeof r.details?.value === "boolean" ? (r.details.value as boolean) : undefined);
  return {
    type: evalType || r.type || r.name || "assertion",
    kind,
    passed,
    score: r.score ?? r.details?.score ?? r.details?.metric_value,
    action: kind === "guardrail" ? action ?? (passed === false ? "block" : "allow") : undefined,
    explanation: r.message ?? r.details?.explanation,
    violations: violationsOf(r.violations),
  };
}

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

// Message assertions are persisted under meta.assertions.results.
function metaAssertions(m: Message): RawAssertion[] {
  const a = (m.meta as { assertions?: { results?: RawAssertion[] } } | undefined)?.assertions;
  return Array.isArray(a?.results) ? a!.results! : [];
}

function messageChecks(m: Message, i: number, run: RunResult): AtlasCheck[] {
  const out: AtlasCheck[] = [];
  for (const r of metaAssertions(m)) out.push(assertionToCheck(r));
  for (const v of m.validations ?? []) {
    out.push({ type: v.validator_type, kind: "guardrail", passed: v.passed, action: v.passed ? "allow" : "block" });
  }
  for (const e of run.eval_results ?? []) if (e.turn_index === i) out.push(evalToCheck(e));
  return out;
}

// Conversation checks are under `conversation_assertions` (snake, wire) — read
// the PascalCase field too in case an endpoint re-marshals.
function convResults(run: RunResult): RawAssertion[] {
  const ca = (run as { conversation_assertions?: { results?: RawAssertion[] } }).conversation_assertions ?? run.ConversationAssertions;
  return Array.isArray(ca?.results) ? (ca!.results as RawAssertion[]) : [];
}

export function conversationChecks(run: RunResult): AtlasCheck[] {
  const out: AtlasCheck[] = [];
  for (const r of convResults(run)) out.push(assertionToCheck(r));
  for (const e of run.eval_results ?? []) if (e.turn_index == null) out.push(evalToCheck(e));
  return out;
}

// ---- metrics ----

function metricsOf(m: Message): AtlasMessage["metrics"] {
  const latencyNs = (m.cost_info as { latency_ns?: number } | undefined)?.latency_ns;
  const latencyMs = m.latency_ms ?? (latencyNs != null ? latencyNs / 1e6 : undefined);
  if (latencyMs == null && !m.cost_info) return undefined;
  return {
    latencyMs: latencyMs != null ? Math.round(latencyMs) : undefined,
    inputTokens: m.cost_info?.input_tokens,
    outputTokens: m.cost_info?.output_tokens,
    costUsd: m.cost_info?.total_cost_usd,
  };
}

function tsOf(m: Message, i: number, baseMs: number): string {
  if (m.timestamp) return m.timestamp;
  return new Date(baseMs + i * 1000).toISOString();
}

// ---- top-level ----

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
    error: run.Error && i === run.Messages.length - 1 ? { message: run.Error } : undefined,
  };
}

export interface AdaptedRun {
  title: string;
  messages: AtlasMessage[];
  checks?: AtlasCheck[];
  recording?: { src: string };
}

// Arena's WorkflowGraph maps almost 1:1 onto ConstellationGraph: node.kind is
// the same vocabulary; edges rename from/to → source/target; the dim/gold
// overlay fields (set by overlayWorkflowRun for the taken path) pass straight
// through. A node referenced as someone's parent becomes a group container.
export function adaptWorkflow(graph: WorkflowGraph): { nodes: ConstellationNode[]; edges: ConstellationEdge[] } {
  const parents = new Set(graph.nodes.map((n) => n.parent).filter(Boolean) as string[]);
  return {
    nodes: graph.nodes.map((n) => ({
      id: n.id,
      kind: n.kind,
      label: n.label,
      dim: n.dim,
      parent: n.parent,
      group: parents.has(n.id) || undefined,
    })),
    edges: graph.edges.map((e, i) => ({
      id: `${e.from}->${e.to}-${i}`,
      source: e.from,
      target: e.to,
      label: e.label,
      dashed: e.dashed,
      gold: e.gold,
      dim: e.dim,
    })),
  };
}

export function adaptRun(run: RunResult): AdaptedRun {
  const baseMs = Date.parse(run.StartTime) || Date.now();
  const messages = (run.Messages ?? []).map((m, i) => adaptMessage(m, i, run, baseMs));
  const checks = conversationChecks(run);
  return {
    title: `${run.ScenarioID} · ${run.ProviderID}`,
    messages,
    checks: checks.length ? checks : undefined,
    recording: run.RecordingPath ? { src: `/api/media/${encodeURI(run.RecordingPath)}` } : undefined,
  };
}
