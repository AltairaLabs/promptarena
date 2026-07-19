// arenaView.ts — pure selector layer turning real RunResult[]/ActiveRun[]
// into the Atlas redesign's viewmodel. No React, no side effects, no
// Date.now() — every ordering/derivation comes from the data's own
// timestamps so the module stays deterministic and easy to unit test.

import type {
  RunResult,
  ActiveRun,
  TrialCell,
  TrialRow,
  TrialMatrix,
  Standing,
  OverallGauge,
  WorkflowGraph,
} from "@/types";
import type { MetricSpec } from "@altairalabs/atlas";
import { formatDuration } from "./utils";

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
        // RunResult.Duration is a Go time.Duration, serialized in
        // nanoseconds — convert to milliseconds so TrialCell.latencyMs
        // is real milliseconds throughout the viewmodel.
        latencyMs: (latest.Duration ?? 0) / 1e6,
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
  // RunResult.Duration is nanoseconds on the wire; convert to milliseconds
  // before taking the median so p50 lines up with TrialCell.latencyMs.
  const latencyP50 = p50(results.map((r) => (r.Duration ?? 0) / 1e6));

  return [
    { label: "TRIALS", value: String(results.length), tone: "default" },
    { label: "SPEND", value: `$${totalCost.toFixed(4)}`, tone: "gold" },
    { label: "P50", value: formatDuration(latencyP50), tone: "default" },
    { label: "TOKENS", value: (totalTokens / 1000).toFixed(1), unit: "k", tone: "healthy", dot: "healthy" },
  ];
}

function isRunResult(run: RunResult | ActiveRun): run is RunResult {
  return Array.isArray((run as RunResult).Messages);
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

// DEFAULT_WORKFLOW_NODE_ID is the single-node id the backend hands back for
// a config with no declared workflow. It always counts as visited so it
// never reads as dimmed/unreached.
const DEFAULT_WORKFLOW_NODE_ID = "default";

interface WorkflowStateMeta {
  current_state?: unknown;
  previous_state?: unknown;
}

function workflowStateFromMeta(meta: Record<string, unknown> | undefined): WorkflowStateMeta | undefined {
  const raw = meta?.["_workflow_state"];
  if (!raw || typeof raw !== "object") return undefined;
  return raw as WorkflowStateMeta;
}

// visitedWorkflowPath walks a completed RunResult's messages, collecting the
// visited node ids and traversed from->to edges from each message's
// Meta["_workflow_state"] (current_state / previous_state). ActiveRun
// messages carry no such meta (it's only stamped post-hoc on persisted
// RunResults), so live runs always come back empty here.
function visitedWorkflowPath(run: RunResult | ActiveRun | undefined): { nodes: Set<string>; edges: Set<string> } {
  const nodes = new Set<string>([DEFAULT_WORKFLOW_NODE_ID]);
  const edges = new Set<string>();
  if (!run || !isRunResult(run)) return { nodes, edges };

  for (const msg of run.Messages) {
    const state = workflowStateFromMeta(msg.meta);
    if (!state) continue;
    const current = typeof state.current_state === "string" ? state.current_state : undefined;
    const previous = typeof state.previous_state === "string" ? state.previous_state : undefined;
    if (current) nodes.add(current);
    if (previous) nodes.add(previous);
    if (previous && current) edges.add(`${previous}->${current}`);
  }
  return { nodes, edges };
}

// overlayWorkflowRun (Task 5) highlights the given run's path over a static
// WorkflowGraph: visited nodes/edges are left alone, unvisited ones get
// `dim: true`, and edges actually traversed also get `gold: true`. When the
// run carries no workflow-state meta at all (an in-progress ActiveRun, an
// undefined run, or a completed run whose scenario has no workflow), the
// graph is returned unchanged rather than dimming everything.
export function overlayWorkflowRun(graph: WorkflowGraph, run: RunResult | ActiveRun | undefined): WorkflowGraph {
  const { nodes: visited, edges: visitedEdges } = visitedWorkflowPath(run);
  if (visited.size <= 1 && visitedEdges.size === 0) return graph;

  return {
    nodes: graph.nodes.map((n) => (visited.has(n.id) ? { ...n } : { ...n, dim: true })),
    edges: graph.edges.map((e) => {
      const key = `${e.from}->${e.to}`;
      if (visitedEdges.has(key)) return { ...e, gold: true };
      return { ...e, dim: true };
    }),
  };
}
