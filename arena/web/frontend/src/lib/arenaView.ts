// arenaView.ts — pure selector layer turning real RunResult[]/ActiveRun[]
// into the Atlas redesign's viewmodel. No React, no side effects, no
// Date.now() — every ordering/derivation comes from the data's own
// timestamps so the module stays deterministic and easy to unit test.

import type {
  RunResult,
  ActiveRun,
  Message,
  MessageCreatedData,
  MessageToolCall,
  MessageToolResult,
  TrialCell,
  TrialRow,
  TrialMatrix,
  Standing,
  OverallGauge,
  TranscriptMessage,
} from "@/types";
import type { MetricSpec, TerminalLine, GraphNode, GraphEdge } from "@/components/atlas/types";

function endTimeMs(r: RunResult): number {
  const t = Date.parse(r.EndTime);
  return Number.isNaN(t) ? 0 : t;
}

// latestByEndTime picks the most recently completed run from a bucket of
// RunResults that share the same scenario:provider key.
function latestByEndTime(bucket: RunResult[]): RunResult {
  return bucket.reduce((latest, r) => (endTimeMs(r) > endTimeMs(latest) ? r : latest));
}

// cellPassRate implements the aggregation rule: when ConversationAssertions
// exist, an overall pass is 100, otherwise it's the proportion of individual
// assertions that passed; when there are no assertions at all, an Error
// means a hard 0 and anything else counts as a full pass.
function cellPassRate(r: RunResult): number {
  const ca = r.ConversationAssertions;
  if (ca) {
    if (ca.passed) return 100;
    if (ca.total > 0) return Math.round((100 * (ca.total - ca.failed)) / ca.total);
    return 0;
  }
  return r.Error ? 0 : 100;
}

function emptyCell(scenarioId: string, providerId: string): TrialCell {
  return {
    scenarioId,
    providerId,
    key: `${scenarioId}:${providerId}`,
    passRate: 0,
    passed: false,
    best: false,
    costUsd: 0,
    latencyMs: 0,
    runId: "",
    hasData: false,
  };
}

export function buildMatrix(
  results: RunResult[],
  providers: { id: string; label?: string }[],
  scenarios: { id: string; label?: string }[],
): TrialMatrix {
  const byKey = new Map<string, RunResult[]>();
  for (const r of results) {
    const key = `${r.ScenarioID}:${r.ProviderID}`;
    const bucket = byKey.get(key);
    if (bucket) bucket.push(r);
    else byKey.set(key, [r]);
  }

  const rows: TrialRow[] = scenarios.map((scenario) => {
    const cells: TrialCell[] = providers.map((provider) => {
      const key = `${scenario.id}:${provider.id}`;
      const bucket = byKey.get(key);
      if (!bucket || bucket.length === 0) return emptyCell(scenario.id, provider.id);

      const latest = latestByEndTime(bucket);
      const passRate = cellPassRate(latest);
      return {
        scenarioId: scenario.id,
        providerId: provider.id,
        key,
        passRate,
        passed: passRate === 100,
        best: false,
        costUsd: latest.Cost?.total_cost_usd ?? 0,
        latencyMs: latest.Duration ?? 0,
        runId: latest.RunID,
        hasData: true,
      };
    });

    // best = highest passRate in the row, ties broken by lower cost then
    // lower latency; cells with no data are never eligible.
    const withData = cells.filter((c) => c.hasData);
    if (withData.length > 0) {
      const best = withData.reduce((a, b) => {
        if (b.passRate !== a.passRate) return b.passRate > a.passRate ? b : a;
        if (b.costUsd !== a.costUsd) return b.costUsd < a.costUsd ? b : a;
        return b.latencyMs < a.latencyMs ? b : a;
      });
      best.best = true;
    }

    return { scenarioId: scenario.id, label: scenario.label ?? scenario.id, cells };
  });

  return {
    providers: providers.map((p) => ({ id: p.id, label: p.label ?? p.id })),
    rows,
  };
}

export function buildStandings(matrix: TrialMatrix): Standing[] {
  const wins = new Map<string, number>();
  for (const p of matrix.providers) wins.set(p.id, 0);
  for (const row of matrix.rows) {
    for (const cell of row.cells) {
      if (cell.best) wins.set(cell.providerId, (wins.get(cell.providerId) ?? 0) + 1);
    }
  }

  const ranked = matrix.providers
    .map((p) => ({ providerId: p.id, label: p.label, wins: wins.get(p.id) ?? 0 }))
    .sort((a, b) => b.wins - a.wins);

  const topWins = ranked.length > 0 ? ranked[0].wins : 0;
  return ranked.map((s, i) => ({
    rank: i + 1,
    providerId: s.providerId,
    label: s.label,
    wins: s.wins,
    leader: topWins > 0 && s.wins === topWins,
  }));
}

// buildOverallGauge rolls the matrix up into a single pass-rate readout.
// Cells with no run yet (hasData === false) are excluded from both the
// numerator and the denominator so an empty grid doesn't read as 0%.
export function buildOverallGauge(matrix: TrialMatrix): OverallGauge {
  const cellsWithData = matrix.rows.flatMap((row) => row.cells).filter((c) => c.hasData);
  const passed = cellsWithData.filter((c) => c.passed).length;
  const total = cellsWithData.length;
  const passRate = total > 0 ? Math.round((100 * passed) / total) : 0;
  return { passRate, passed, total, caption: `${passed} / ${total} passed` };
}

// p50 uses the lower-middle element of the sorted durations — a
// deterministic nearest-rank median that needs no interpolation.
function p50(values: number[]): number {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.floor((sorted.length - 1) / 2)];
}

// buildMetrics summarizes the run set into the instrument-readout row:
// trial count, total spend, p50 latency, and total tokens (in thousands).
// `matrix` is accepted for symmetry with the rest of the selector API even
// though today's readout is derived entirely from the raw results.
export function buildMetrics(results: RunResult[], _matrix: TrialMatrix): MetricSpec[] {
  const totalCost = results.reduce((sum, r) => sum + (r.Cost?.total_cost_usd ?? 0), 0);
  const totalTokens = results.reduce(
    (sum, r) => sum + (r.Cost?.input_tokens ?? 0) + (r.Cost?.output_tokens ?? 0),
    0,
  );
  const latencyP50 = p50(results.map((r) => r.Duration ?? 0));

  return [
    { label: "TRIALS", value: String(results.length), tone: "default" },
    { label: "SPEND", value: `$${totalCost.toFixed(4)}`, tone: "gold" },
    { label: "P50 LATENCY", value: String(latencyP50), unit: "ms", tone: "default" },
    { label: "TOKENS", value: (totalTokens / 1000).toFixed(1), unit: "k", tone: "healthy", dot: "healthy" },
  ];
}

// roleAccent maps a transcript role to its Atlas accent color token.
export function roleAccent(role: string): string {
  switch (role) {
    case "system":
      return "var(--nebula-violet)";
    case "user":
      return "var(--starlight-300)";
    case "assistant":
      return "var(--ion-cyan)";
    case "tool":
      return "var(--amber-500)";
    default:
      return "var(--starlight-300)";
  }
}

function accentBg(accent: string): string {
  return `color-mix(in srgb, ${accent} 11%, transparent)`;
}

function firstTextPart(parts: { type: string; text?: string }[] | undefined): string | undefined {
  return parts?.find((p) => p.type === "text")?.text;
}

function toolFromCallAndResult(
  toolCalls: MessageToolCall[] | undefined,
  toolResult: MessageToolResult | null | undefined,
): { name: string; body: string } | undefined {
  if (toolResult) {
    const text = firstTextPart(toolResult.parts as { type: string; text?: string }[] | undefined);
    return { name: toolResult.name, body: text ?? JSON.stringify(toolResult.parts ?? []) };
  }
  if (toolCalls && toolCalls.length > 0) {
    const call = toolCalls[0];
    return { name: call.name, body: JSON.stringify(call.args ?? {}) };
  }
  return undefined;
}

function metaFromMessage(msg: Message): string | undefined {
  const parts: string[] = [];
  if (msg.cost_info) parts.push(`$${msg.cost_info.total_cost_usd.toFixed(4)}`);
  if (typeof msg.latency_ms === "number") parts.push(`${msg.latency_ms}ms`);
  return parts.length > 0 ? parts.join(" · ") : undefined;
}

function assertsFromMessage(msg: Message): { name: string; ok: boolean }[] | undefined {
  if (!msg.validations || msg.validations.length === 0) return undefined;
  return msg.validations.map((v) => ({ name: v.validator_type, ok: v.passed }));
}

function toTranscriptMessage(
  role: string,
  idx: number,
  content: string | undefined,
  meta: string | undefined,
  tool: { name: string; body: string } | undefined,
  asserts: { name: string; ok: boolean }[] | undefined,
): TranscriptMessage {
  const accent = roleAccent(role);
  return { role, idx, accent, bg: accentBg(accent), content, meta, tool, asserts };
}

function isRunResult(run: RunResult | ActiveRun): run is RunResult {
  return Array.isArray((run as RunResult).Messages);
}

// buildTranscript maps a RunResult's (or a still-running ActiveRun's) message
// sequence into the transcript viewmodel. ActiveRun messages carry no
// per-message cost/latency/validation data (that only exists on completed
// RunResults), so `meta` and `asserts` are omitted for live runs.
export function buildTranscript(run: RunResult | ActiveRun | undefined): TranscriptMessage[] {
  if (!run) return [];
  if (isRunResult(run)) {
    return run.Messages.map((msg, idx) =>
      toTranscriptMessage(
        msg.role,
        idx,
        msg.content,
        metaFromMessage(msg),
        toolFromCallAndResult(msg.tool_calls, msg.tool_result),
        assertsFromMessage(msg),
      ),
    );
  }
  return (run.messages ?? []).map((msg: MessageCreatedData, idx) =>
    toTranscriptMessage(
      msg.role,
      idx,
      msg.content,
      undefined,
      toolFromCallAndResult(msg.toolCalls, msg.toolResult),
      undefined,
    ),
  );
}

// buildTerminalLines (D3) synthesizes a cosmetic CLI transcript for a trial
// cell — the command that would reproduce it, plus a pass/fail summary line.
// There's no real terminal session behind this; it's a readable stand-in.
export function buildTerminalLines(
  cell: TrialCell | undefined,
  scenarioId: string,
  providerId: string,
): TerminalLine[] {
  const lines: TerminalLine[] = [
    { type: "command", text: `promptarena run --scenario ${scenarioId} --provider ${providerId}`, prompt: "$" },
  ];
  if (!cell || !cell.hasData) {
    lines.push({ type: "muted", text: "no run yet" });
    return lines;
  }
  const cost = cell.costUsd > 0 ? `$${cell.costUsd.toFixed(4)}` : "free";
  if (cell.passed) {
    lines.push({ type: "success", text: `✓ assertions passed · ${cost} · ${cell.latencyMs}ms` });
  } else {
    lines.push({ type: "error", text: `✗ assertions failed · ${cost} · ${cell.latencyMs}ms` });
  }
  return lines;
}

// buildTrend (D1) derives a real pass-rate series by bucketing historical
// RunResults chronologically by StartTime, keeping only the last `buckets`
// runs. Fewer than 3 results means there isn't enough history to plot a
// meaningful trail, so the UI hides it entirely.
export function buildTrend(results: RunResult[], buckets = 8): number[] {
  if (results.length < 3) return [];
  const sorted = [...results].sort((a, b) => (Date.parse(a.StartTime) || 0) - (Date.parse(b.StartTime) || 0));
  return sorted.slice(-buckets).map((r) => cellPassRate(r));
}

interface FlowMessage {
  role: string;
  toolCallNames: string[];
}

function extractFlowMessages(run: RunResult | ActiveRun): FlowMessage[] {
  if (isRunResult(run)) {
    return run.Messages.map((m) => ({ role: m.role, toolCallNames: (m.tool_calls ?? []).map((tc) => tc.name) }));
  }
  return (run.messages ?? []).map((m: MessageCreatedData) => ({
    role: m.role,
    toolCallNames: (m.toolCalls ?? []).map((tc) => tc.name),
  }));
}

const FLOW_VIEWBOX = { width: 360, height: 150 };

// buildAgentFlow (D2, option b) derives an approximate constellation from a
// run's message/tool sequence: entry (user) -> agent (each assistant turn)
// -> tool (each distinct tool call, revisited rather than duplicated) ->
// output (resolved/failed). There's no backend endpoint behind this — it's
// a deterministic reading of the transcript already on hand, laid out in a
// fixed viewBox with entry pinned left and output pinned right.
export function buildAgentFlow(run: RunResult | ActiveRun | undefined): { nodes: GraphNode[]; edges: GraphEdge[] } {
  if (!run) return { nodes: [], edges: [] };

  const hasError = isRunResult(run) ? Boolean(run.Error) : run.status === "failed";
  const edgeMargin = 15; // keeps the entry/output node halos inside the viewBox
  const entry: GraphNode = { id: "entry", x: edgeMargin, y: FLOW_VIEWBOX.height / 2, kind: "entry", label: "user" };
  const output: GraphNode = {
    id: "output",
    x: FLOW_VIEWBOX.width - edgeMargin,
    y: FLOW_VIEWBOX.height / 2,
    kind: "output",
    label: hasError ? "failed" : "resolved",
  };

  const middle: GraphNode[] = [];
  const edges: GraphEdge[] = [];
  const toolNodeIds = new Map<string, string>();
  let agentCount = 0;
  let cursor: string = entry.id;

  for (const msg of extractFlowMessages(run)) {
    if (msg.role !== "assistant") continue;

    agentCount += 1;
    const agentId = `agent-${agentCount}`;
    middle.push({ id: agentId, x: 0, y: 0, kind: "agent", label: `turn ${agentCount}` });
    edges.push({ from: cursor, to: agentId });
    cursor = agentId;

    for (const name of msg.toolCallNames) {
      let toolId = toolNodeIds.get(name);
      if (!toolId) {
        toolId = `tool-${name}`;
        toolNodeIds.set(name, toolId);
        middle.push({ id: toolId, x: 0, y: 0, kind: "tool", label: name });
      }
      edges.push({ from: cursor, to: toolId });
      cursor = toolId;
    }
  }

  edges.push({ from: cursor, to: output.id, gold: !hasError });

  // Deterministic layout: spread the intermediate nodes evenly between
  // entry and output, offsetting tool nodes onto a second row so the
  // constellation doesn't collapse onto a single line.
  const margin = FLOW_VIEWBOX.width * 0.2;
  const spanStart = margin;
  const spanEnd = FLOW_VIEWBOX.width - margin;
  const n = middle.length;
  middle.forEach((node, i) => {
    node.x = n === 1 ? (spanStart + spanEnd) / 2 : spanStart + ((spanEnd - spanStart) * i) / (n - 1);
    node.y = node.kind === "tool" ? FLOW_VIEWBOX.height * 0.7 : FLOW_VIEWBOX.height * 0.3;
  });

  return { nodes: [entry, ...middle, output], edges };
}
