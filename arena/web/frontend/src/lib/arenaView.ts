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
  FieldEstimate,
  RunBatch,
  WorkflowGraph,
} from "@/types";
import type { MetricSpec } from "@altairalabs/atlas";
import { formatDuration } from "./utils";

function endTimeMs(r: RunResult): number {
  const t = Date.parse(r.EndTime);
  return Number.isNaN(t) ? 0 : t;
}

// latestByEndTime picks the most recently completed run from a bucket of
// RunResults that share the same scenario:provider key. Seeded with the first
// element: an unseeded reduce() throws on an empty bucket, and while callers
// only reach this for buckets they've already counted as non-empty, the throw
// would take the whole view down rather than degrade.
function latestByEndTime(bucket: RunResult[]): RunResult {
  return bucket.reduce((latest, r) => (endTimeMs(r) > endTimeMs(latest) ? r : latest), bucket[0]);
}

// assertionStats aggregates assertion outcomes across BOTH turn-level
// assertions (stamped in each message's meta — the common case) and
// conversation-level assertions. The web previously read only the
// conversation-level summary, so any run judged by turn-level assertions
// (most of them) scored as a bare 0%. This mirrors the Go AllAssertionsPassed,
// which counts both lanes.
function assertionStats(r: RunResult): { total: number; passed: number } {
  let total = 0;
  let passed = 0;
  const ca = r.ConversationAssertions;
  if (ca && ca.total > 0) {
    total += ca.total;
    passed += ca.total - ca.failed;
  }
  for (const m of r.Messages ?? []) {
    const a = m.meta?.["assertions"] as { total?: number; failed?: number } | undefined;
    if (a && typeof a.total === "number" && a.total > 0) {
      total += a.total;
      passed += a.total - (a.failed ?? 0);
    }
  }
  return { total, passed };
}

// cellPassRate: an Error is a hard fail and takes precedence over everything.
// Otherwise it's the proportion of assertions (turn- and conversation-level)
// that passed; a clean run with no assertions has nothing to fail, so it's a
// full pass (used by the historical trend line as "did it run / error").
function cellPassRate(r: RunResult): number {
  if (r.Error) return 0;
  const { total, passed } = assertionStats(r);
  if (total === 0) return 100;
  return Math.round((100 * passed) / total);
}

// runScored reports whether a trial was actually judged: it errored (a definite
// failed outcome) or at least one assertion (turn- or conversation-level)
// evaluated it. A clean run with no assertions was never scored — it reads as
// neutral (an unscored trial), not a win, on the matrix/gauge/standings.
export function runScored(r: RunResult): boolean {
  if (r.Error) return true;
  return assertionStats(r).total > 0;
}

// STRIP_MAX caps how many trial marks a cell's history strip shows.
const STRIP_MAX = 10;

function emptyCell(scenarioId: string, providerId: string): TrialCell {
  return {
    scenarioId,
    providerId,
    key: `${scenarioId}:${providerId}`,
    passRate: 0,
    passedCount: 0,
    totalRuns: 0,
    passed: false,
    scored: false,
    history: [],
    best: false,
    costUsd: 0,
    latencyMs: 0,
    runId: "",
    hasData: false,
  };
}

function runTimeMs(r: RunResult): number {
  return Date.parse(r.EndTime || r.StartTime || "") || 0;
}

// runPassed reports whether one complete run passed: no execution error AND
// every assertion (turn- and conversation-level) passed. A run with no
// assertions has nothing to fail, so it passes vacuously.
function runPassed(r: RunResult): boolean {
  if (r.Error) return false;
  const { total, passed } = assertionStats(r);
  return total === 0 ? true : passed === total;
}

// runOutcome classifies a single run for the ledger: "error" for a genuine
// execution failure (an Error with no assertion data — e.g. a provider 403),
// "pass"/"fail" otherwise. A failed-assertion abort carries assertion data, so
// it reads as "fail", not "error".
export function runOutcome(r: RunResult): "pass" | "fail" | "error" {
  if (r.Error && assertionStats(r).total === 0) return "error";
  return runPassed(r) ? "pass" : "fail";
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

      // Aggregate this cell's runs — the matrix is a reliability view across
      // the model's non-determinism, not a single run. Only SCORED runs (ones
      // with assertions, or that errored) count toward the rate; a clean run
      // with no assertions was never judged and is excluded, so an assertion-
      // less cell reads as unscored (—) rather than a false 100%.
      const scoredRuns = bucket.filter(runScored);
      const totalRuns = scoredRuns.length;
      const passedCount = scoredRuns.filter(runPassed).length;
      const passRate = totalRuns > 0 ? Math.round((100 * passedCount) / totalRuns) : 0;
      // history: each scored trial's outcome oldest→newest, capped, for the
      // per-cell flakiness strip.
      const history = [...scoredRuns]
        .sort((a, b) => runTimeMs(a) - runTimeMs(b))
        .slice(-STRIP_MAX)
        .map(runOutcome);
      // Cost/latency are per-run averages over all runs. Duration is a Go
      // time.Duration in nanoseconds → ms.
      const avgLatency = bucket.reduce((s, r) => s + (r.Duration ?? 0) / 1e6, 0) / bucket.length;
      const avgCost = bucket.reduce((s, r) => s + (r.Cost?.total_cost_usd ?? 0), 0) / bucket.length;
      return {
        scenarioId: scenario.id,
        providerId: provider.id,
        key,
        passRate,
        passedCount,
        totalRuns,
        passed: totalRuns > 0 && passRate === 100,
        scored: totalRuns > 0,
        history,
        best: false,
        costUsd: avgCost,
        latencyMs: avgLatency,
        runId: latestByEndTime(bucket).RunID,
        hasData: true,
      };
    });

    // best = the most reliable provider for the scenario (highest pass rate),
    // ties broken by lower cost then latency. A row where nothing ever passed
    // (all 0%) has no winner.
    const winners = cells.filter((c) => c.scored && c.passRate > 0);
    if (winners.length > 0) {
      const best = winners.reduce((a, b) => {
        if (b.passRate !== a.passRate) return b.passRate > a.passRate ? b : a;
        if (b.costUsd !== a.costUsd) return b.costUsd < a.costUsd ? b : a;
        return b.latencyMs < a.latencyMs ? b : a;
      }, winners[0]);
      best.best = true;
    }

    return { scenarioId: scenario.id, label: scenario.label ?? scenario.id, cells };
  });

  return {
    providers: providers.map((p) => ({ id: p.id, label: p.label ?? p.id })),
    rows,
  };
}

// batchIdOf extracts the batch identifier shared by every run from one "Run
// the field": the leading timestamp prefix of the RunID (all runs dispatched
// together share it, e.g. "2026-07-23T14-50-02Z"). The prefix has no
// underscore, so everything up to the first "_" is the batch id.
export function batchIdOf(runId: string): string {
  const i = runId.indexOf("_");
  return i === -1 ? runId : runId.slice(0, i);
}

// groupRunsByBatch groups results into run-batches keyed by batchIdOf(RunID),
// newest batch first (the timestamp prefix sorts lexicographically). Each
// batch summarizes its scenarios, providers, pass rate and cost for the
// ledger. Pass rate counts only scored runs (see cellScored) so an unjudged
// batch doesn't read as a false 100%.
export function groupRunsByBatch(results: RunResult[]): RunBatch[] {
  const byBatch = new Map<string, RunResult[]>();
  for (const r of results) {
    const id = batchIdOf(r.RunID);
    const bucket = byBatch.get(id);
    if (bucket) bucket.push(r);
    else byBatch.set(id, [r]);
  }

  const batches: RunBatch[] = [];
  for (const [batchId, runs] of byBatch) {
    const scored = runs.filter(runScored);
    const passed = scored.filter((r) => cellPassRate(r) === 100).length;
    const startedAt = runs.reduce(
      (min, r) => (r.StartTime && (min === "" || r.StartTime < min) ? r.StartTime : min),
      "",
    );
    batches.push({
      batchId,
      startedAt,
      results: runs,
      scenarios: [...new Set(runs.map((r) => r.ScenarioID))],
      providers: [...new Set(runs.map((r) => r.ProviderID))],
      passRate: scored.length > 0 ? Math.round((100 * passed) / scored.length) : 0,
      totalCostUsd: runs.reduce((sum, r) => sum + (r.Cost?.total_cost_usd ?? 0), 0),
    });
  }

  batches.sort((a, b) => (a.batchId < b.batchId ? 1 : a.batchId > b.batchId ? -1 : 0));
  return batches;
}

// orderLedgerRows produces the ledger's visible run order: optionally scoped to
// one scenario×provider (a matrix-cell drill), then sorted newest-first by end
// time. Shared by the ledger list and the run-details back/next stepper so both
// walk the runs in exactly the same order. Pure — never mutates its input.
export function orderLedgerRows(
  results: RunResult[],
  filter?: { scenarioId: string; providerId: string } | null,
): RunResult[] {
  const scoped = filter
    ? results.filter((r) => r.ScenarioID === filter.scenarioId && r.ProviderID === filter.providerId)
    : results;
  return [...scoped].sort((a, b) => runTimeMs(b) - runTimeMs(a));
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
  // Aggregate: the overall run pass rate across every run in the grid.
  const cells = matrix.rows.flatMap((row) => row.cells).filter((c) => c.hasData);
  const passed = cells.reduce((s, c) => s + c.passedCount, 0);
  const total = cells.reduce((s, c) => s + c.totalRuns, 0);
  const passRate = total > 0 ? Math.round((100 * passed) / total) : 0;
  return { passRate, passed, total, caption: `${passed} / ${total} trials passed` };
}

// FIELD_CONCURRENCY mirrors the backend's defaultConcurrency (arena/web/
// server.go): within one sweep, cells run across this many workers; sweeps
// themselves run sequentially. estimateFieldRun models both.
export const FIELD_CONCURRENCY = 4;

// estimateFieldRun projects the spend and wall-clock time of running the
// selected scenarios across every provider `runCount` times, from each target
// cell's historical per-run averages. Cells with no history contribute nothing
// (their cost/time is unknown), so covered < total means the totals are a lower
// bound. Within a sweep, cells run across `concurrency` workers, so the time is
// at least the slowest single cell and roughly the summed latency spread over
// the workers; sweeps run sequentially, so both totals scale with runCount.
// Returns null when no target cell has data — there is nothing to estimate from.
export function estimateFieldRun(
  matrix: TrialMatrix,
  selectedScenarioIds: string[],
  runCount: number,
  concurrency = FIELD_CONCURRENCY,
): FieldEstimate | null {
  const selected = new Set(selectedScenarioIds);
  const cells = matrix.rows.filter((row) => selected.has(row.scenarioId)).flatMap((row) => row.cells);
  const withData = cells.filter((c) => c.hasData);
  if (withData.length === 0) return null;

  const costPerSweep = withData.reduce((s, c) => s + c.costUsd, 0);
  const sumLatency = withData.reduce((s, c) => s + c.latencyMs, 0);
  const maxLatency = withData.reduce((m, c) => Math.max(m, c.latencyMs), 0);
  const timePerSweep = Math.max(maxLatency, sumLatency / Math.max(1, concurrency));

  return {
    costUsd: costPerSweep * runCount,
    timeMs: timePerSweep * runCount,
    covered: withData.length,
    total: cells.length,
  };
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


// buildSuiteTrend plots each full-field SWEEP's overall pass rate, oldest→
// newest — every sweep, uncapped, so the whole history is visible however many
// times the field has been run. A sweep is a batch (runs sharing a RunID
// timestamp prefix) that ran EVERY populated cell — a real "Run the field", not
// an ad-hoc single-cell run. Only full sweeps are comparable, so only they make
// the line mean "the suite trending greener over time"; fewer than two sweeps
// isn't a trend, so the UI hides it.
export function buildSuiteTrend(results: RunResult[]): number[] {
  // Populated cells = every scenario×provider that has ever run.
  const cellCount = new Set(results.map((r) => `${r.ScenarioID}:${r.ProviderID}`)).size;
  if (cellCount === 0) return [];

  const byBatch = new Map<string, RunResult[]>();
  for (const r of results) {
    const id = batchIdOf(r.RunID);
    const bucket = byBatch.get(id);
    if (bucket) bucket.push(r);
    else byBatch.set(id, [r]);
  }

  const sweeps = [...byBatch.entries()]
    .filter(([, runs]) => new Set(runs.map((r) => `${r.ScenarioID}:${r.ProviderID}`)).size >= cellCount)
    .sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0)); // chronological by batch id
  if (sweeps.length < 2) return [];

  return sweeps.map(([, runs]) => {
    const passed = runs.filter(runPassed).length;
    return Math.round((100 * passed) / runs.length);
  });
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

// currentWorkflowStateAt returns the state the conversation is in at message
// `index`: the most recent current_state stamped at or before that message
// (with the previous_state of that step). Empty when the run carries no
// workflow-state meta up to that point. Drives the live, scrubber-synced
// highlight — as the playhead moves, the current state moves with it.
export function currentWorkflowStateAt(
  run: RunResult | ActiveRun | undefined,
  index: number,
): { current?: string; previous?: string } {
  if (!run || !isRunResult(run)) return {};
  let result: { current?: string; previous?: string } = {};
  const upto = Math.min(index, run.Messages.length - 1);
  for (let i = 0; i <= upto; i++) {
    const state = workflowStateFromMeta(run.Messages[i]?.meta);
    if (!state) continue;
    const current = typeof state.current_state === "string" ? state.current_state : undefined;
    const previous = typeof state.previous_state === "string" ? state.previous_state : undefined;
    if (current) result = { current, previous };
  }
  return result;
}

// overlayWorkflowCurrentState spotlights ONE state on the graph — the current
// one — dimming the rest, with the edge just taken (previous → current) gold.
// Unlike overlayWorkflowRun (the whole taken path), this tracks a single moving
// point, so the highlight follows the scrubber/turn selection. No current state
// (e.g. a live run with no stamped meta) returns the graph unchanged.
export function overlayWorkflowCurrentState(
  graph: WorkflowGraph,
  current: string | undefined,
  previous?: string,
): WorkflowGraph {
  if (!current) return graph;
  return {
    nodes: graph.nodes.map((n) => (n.id === current ? { ...n } : { ...n, dim: true })),
    edges: graph.edges.map((e) =>
      previous && e.from === previous && e.to === current ? { ...e, gold: true } : { ...e, dim: true },
    ),
  };
}
