// arenaView.ts — pure selector layer turning real RunResult[]/ActiveRun[]
// into the Atlas redesign's viewmodel. No React, no side effects, no
// Date.now() — every ordering/derivation comes from the data's own
// timestamps so the module stays deterministic and easy to unit test.

import type {
  RunResult,
  TrialCell,
  TrialRow,
  TrialMatrix,
  Standing,
} from "@/types";

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
