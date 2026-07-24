import { describe, it, expect } from "vitest";
import {
  buildMatrix,
  buildStandings,
  buildOverallGauge,
  buildMetrics,
  buildSuiteTrend,
  estimateFieldRun,
  orderLedgerRows,
  overlayWorkflowRun,
  batchIdOf,
  groupRunsByBatch,
} from "./arenaView";
import type { RunResult, WorkflowGraph, ActiveRun } from "@/types";

const mk = (o: Partial<RunResult>): RunResult => ({
  RunID: o.RunID ?? "r", PromptPack: "", Region: "", ScenarioID: o.ScenarioID!, ProviderID: o.ProviderID!,
  Params: {}, Messages: [], Commit: {}, Cost: (o.Cost as any) ?? { total_cost_usd: 0, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 },
  Violations: [], StartTime: o.StartTime ?? "2026-07-07T00:00:00Z", EndTime: o.EndTime ?? "2026-07-07T00:00:01Z",
  Duration: o.Duration ?? 1000, Error: o.Error ?? "", SelfPlay: false, PersonaID: "", MediaOutputs: [], A2AAgents: [],
  ConversationAssertions: o.ConversationAssertions, ...o,
} as RunResult);

const providers = [{ id: "claude" }, { id: "gpt4o" }];
const scenarios = [{ id: "checkout" }];

const passed1 = (scenario: string, provider: string) =>
  mk({ ScenarioID: scenario, ProviderID: provider, ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] } });
const failed1 = (scenario: string, provider: string) =>
  mk({ ScenarioID: scenario, ProviderID: provider, ConversationAssertions: { passed: false, failed: 1, total: 1, results: [] } });

describe("buildMatrix (aggregate reliability across runs)", () => {
  it("aggregates a cell's runs into passed/total and a pass rate", () => {
    const m = buildMatrix([
      passed1("checkout", "claude"),
      passed1("checkout", "claude"),
      failed1("checkout", "claude"),
    ], providers, scenarios);
    const claude = m.rows[0].cells.find(c => c.providerId === "claude")!;
    expect(claude.totalRuns).toBe(3);
    expect(claude.passedCount).toBe(2);
    expect(claude.passRate).toBe(67);
    expect(claude.passed).toBe(false); // not fully reliable
  });

  it("a cell where every run passed is 100% and fully reliable", () => {
    const m = buildMatrix([passed1("checkout", "claude"), passed1("checkout", "claude")], providers, scenarios);
    const claude = m.rows[0].cells.find(c => c.providerId === "claude")!;
    expect(claude.passRate).toBe(100);
    expect(claude.passed).toBe(true);
  });

  it("a partially-passing single run is a FAILED run — the cell is 0/1, not its assertion proportion", () => {
    const m = buildMatrix([
      mk({ ScenarioID: "checkout", ProviderID: "gpt4o", ConversationAssertions: { passed: false, failed: 2, total: 4, results: [] } }),
    ], providers, scenarios);
    const gpt = m.rows[0].cells.find(c => c.providerId === "gpt4o")!;
    expect(gpt.passRate).toBe(0);
    expect(gpt.passedCount).toBe(0);
    expect(gpt.totalRuns).toBe(1);
  });

  it("counts a run passed via TURN-level assertions", () => {
    const m = buildMatrix([
      mk({ ScenarioID: "checkout", ProviderID: "claude",
        Messages: [{ role: "assistant", content: "x", meta: { assertions: { passed: true, failed: 0, total: 2 } } }] as never }),
    ], providers, scenarios);
    expect(m.rows[0].cells.find(c => c.providerId === "claude")!.passRate).toBe(100);
  });

  it("an errored run counts as a failed run, dragging the rate down", () => {
    const m = buildMatrix([
      passed1("checkout", "claude"),
      mk({ ScenarioID: "checkout", ProviderID: "claude", Error: "boom" }),
    ], providers, scenarios);
    const claude = m.rows[0].cells.find(c => c.providerId === "claude")!;
    expect(claude.totalRuns).toBe(2);
    expect(claude.passedCount).toBe(1);
    expect(claude.passRate).toBe(50);
  });

  it("excludes assertion-less clean runs from the rate (unscored, not a false 100%)", () => {
    const m = buildMatrix([mk({ ScenarioID: "checkout", ProviderID: "claude" })], providers, scenarios);
    const claude = m.rows[0].cells.find(c => c.providerId === "claude")!;
    expect(claude.hasData).toBe(true);
    expect(claude.scored).toBe(false);
    expect(claude.totalRuns).toBe(0);
  });

  it("records each scored trial's outcome in history, oldest→newest", () => {
    const m = buildMatrix([
      mk({ RunID: "a", ScenarioID: "checkout", ProviderID: "claude", EndTime: "2026-07-07T00:00:00Z", ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] } }),
      mk({ RunID: "b", ScenarioID: "checkout", ProviderID: "claude", EndTime: "2026-07-07T00:00:02Z", Error: "boom" }),
      mk({ RunID: "c", ScenarioID: "checkout", ProviderID: "claude", EndTime: "2026-07-07T00:00:01Z", ConversationAssertions: { passed: false, failed: 1, total: 1, results: [] } }),
    ], providers, scenarios);
    const claude = m.rows[0].cells.find(c => c.providerId === "claude")!;
    expect(claude.history).toEqual(["pass", "fail", "error"]); // by StartTime
  });

  it("averages cost and latency across the cell's runs", () => {
    const m = buildMatrix([
      mk({ ScenarioID: "checkout", ProviderID: "claude", Duration: 1_000_000, Cost: { total_cost_usd: 0.02, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 } }),
      mk({ ScenarioID: "checkout", ProviderID: "claude", Duration: 3_000_000, Cost: { total_cost_usd: 0.04, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 } }),
    ], providers, scenarios);
    const claude = m.rows[0].cells.find(c => c.providerId === "claude")!;
    expect(claude.latencyMs).toBeCloseTo(2);
    expect(claude.costUsd).toBeCloseTo(0.03);
  });

  it("marks the most reliable provider best; a row nobody passed has no winner", () => {
    const m = buildMatrix([passed1("checkout", "claude"), failed1("checkout", "gpt4o")], providers, scenarios);
    expect(m.rows[0].cells.find(c => c.providerId === "claude")!.best).toBe(true);
    expect(m.rows[0].cells.find(c => c.providerId === "gpt4o")!.best).toBe(false);
  });

  it("awards no best to a scenario no provider ever passed", () => {
    const m = buildMatrix([
      mk({ ScenarioID: "checkout", ProviderID: "claude", Error: "boom" }),
      mk({ ScenarioID: "checkout", ProviderID: "gpt4o", Error: "boom" }),
    ], providers, scenarios);
    expect(m.rows[0].cells.every(c => !c.best)).toBe(true);
    expect(buildStandings(m).every(s => s.wins === 0)).toBe(true);
  });
});

describe("batchIdOf", () => {
  it("returns the timestamp prefix shared by a batch's runs", () => {
    expect(batchIdOf("2026-07-23T14-50-02Z_claude_default_checkout_abc_0001")).toBe("2026-07-23T14-50-02Z");
  });
  it("returns the whole id when there is no underscore", () => {
    expect(batchIdOf("plain-id")).toBe("plain-id");
  });
});

describe("groupRunsByBatch", () => {
  const b1a = mk({ RunID: "2026-07-23T10-00-00Z_claude_default_checkout_h_1", ScenarioID: "checkout", ProviderID: "claude", StartTime: "2026-07-23T10:00:01Z", Cost: { total_cost_usd: 0.01, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 }, ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] } });
  const b1b = mk({ RunID: "2026-07-23T10-00-00Z_gpt4o_default_refund_h_2", ScenarioID: "refund", ProviderID: "gpt4o", StartTime: "2026-07-23T10:00:00Z", Cost: { total_cost_usd: 0.02, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 }, ConversationAssertions: { passed: false, failed: 1, total: 1, results: [] } });
  const b2 = mk({ RunID: "2026-07-23T12-00-00Z_claude_default_checkout_h_1", ScenarioID: "checkout", ProviderID: "claude", StartTime: "2026-07-23T12:00:00Z", ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] } });

  it("groups runs by their RunID timestamp prefix, newest batch first", () => {
    const batches = groupRunsByBatch([b1a, b1b, b2]);
    expect(batches).toHaveLength(2);
    expect(batches[0].batchId).toBe("2026-07-23T12-00-00Z");
    expect(batches[1].batchId).toBe("2026-07-23T10-00-00Z");
  });

  it("summarizes scenarios, providers, pass rate and cost for a batch", () => {
    const [, older] = groupRunsByBatch([b1a, b1b, b2]);
    expect(older.results).toHaveLength(2);
    expect(older.scenarios.sort()).toEqual(["checkout", "refund"]);
    expect(older.providers.sort()).toEqual(["claude", "gpt4o"]);
    expect(older.passRate).toBe(50); // 1 of 2 scored runs passed
    expect(older.totalCostUsd).toBeCloseTo(0.03);
    expect(older.startedAt).toBe("2026-07-23T10:00:00Z"); // earliest StartTime
  });

  it("returns [] for no results", () => {
    expect(groupRunsByBatch([])).toEqual([]);
  });
});

describe("estimateFieldRun", () => {
  const twoProviders = [{ id: "claude" }, { id: "gpt4o" }];
  const twoScenarios = [{ id: "checkout" }, { id: "refund" }];
  // costed builds a run with a known cost and latency (Duration is nanoseconds
  // on the wire → durationMs * 1e6).
  const costed = (scenario: string, provider: string, costUsd: number, durationMs: number) =>
    mk({
      ScenarioID: scenario, ProviderID: provider, Duration: durationMs * 1e6,
      Cost: { total_cost_usd: costUsd, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 },
    });

  const m = buildMatrix(
    [costed("checkout", "claude", 0.02, 2), costed("checkout", "gpt4o", 0.04, 4)],
    twoProviders, twoScenarios,
  );

  it("sums the selected cells' average cost across the field for one sweep", () => {
    const est = estimateFieldRun(m, ["checkout"], 1, 1)!;
    expect(est.costUsd).toBeCloseTo(0.06); // 0.02 (claude) + 0.04 (gpt4o)
    expect(est.covered).toBe(2);
    expect(est.total).toBe(2);
  });

  it("models within-sweep concurrency in the time estimate", () => {
    // sum latency 6ms, slowest cell 4ms. concurrency 1 → serial 6ms;
    // concurrency 2 → max(4, 6/2=3) = 4ms (can't beat the slowest cell).
    expect(estimateFieldRun(m, ["checkout"], 1, 1)!.timeMs).toBeCloseTo(6);
    expect(estimateFieldRun(m, ["checkout"], 1, 2)!.timeMs).toBeCloseTo(4);
  });

  it("multiplies cost and time by the sweep count (sweeps run sequentially)", () => {
    const est = estimateFieldRun(m, ["checkout"], 3, 1)!;
    expect(est.costUsd).toBeCloseTo(0.18);
    expect(est.timeMs).toBeCloseTo(18);
  });

  it("counts only the selected scenarios' cells", () => {
    // refund has no runs → both its cells are empty → nothing to estimate.
    expect(estimateFieldRun(m, ["refund"], 1, 1)).toBeNull();
  });

  it("reports partial coverage and treats it as a lower bound", () => {
    // Only checkout/claude has history; checkout/gpt4o is empty.
    const partial = buildMatrix([costed("checkout", "claude", 0.02, 2)], twoProviders, twoScenarios);
    const est = estimateFieldRun(partial, ["checkout"], 1, 1)!;
    expect(est.covered).toBe(1);
    expect(est.total).toBe(2);
    expect(est.costUsd).toBeCloseTo(0.02); // gpt4o unknown, contributes nothing
  });

  it("returns null when no target cell has data", () => {
    const empty = buildMatrix([], twoProviders, twoScenarios);
    expect(estimateFieldRun(empty, ["checkout"], 1)).toBeNull();
  });
});

describe("orderLedgerRows", () => {
  const a = mk({ RunID: "a", ScenarioID: "checkout", ProviderID: "claude", EndTime: "2026-07-07T00:00:01Z" });
  const b = mk({ RunID: "b", ScenarioID: "checkout", ProviderID: "claude", EndTime: "2026-07-07T00:00:03Z" });
  const c = mk({ RunID: "c", ScenarioID: "refund", ProviderID: "gpt4o", EndTime: "2026-07-07T00:00:02Z" });

  it("orders rows newest-first by end time", () => {
    expect(orderLedgerRows([a, b, c]).map((r) => r.RunID)).toEqual(["b", "c", "a"]);
  });

  it("scopes to the filtered scenario×provider", () => {
    expect(
      orderLedgerRows([a, b, c], { scenarioId: "refund", providerId: "gpt4o" }).map((r) => r.RunID),
    ).toEqual(["c"]);
  });

  it("returns a new array without mutating the input order", () => {
    const input = [a, b, c];
    orderLedgerRows(input);
    expect(input.map((r) => r.RunID)).toEqual(["a", "b", "c"]);
  });
});

describe("buildStandings", () => {
  it("ranks providers by wins", () => {
    const m = buildMatrix([
      mk({ ScenarioID: "checkout", ProviderID: "claude", ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] } }),
      mk({ ScenarioID: "checkout", ProviderID: "gpt4o", ConversationAssertions: { passed: false, failed: 1, total: 1, results: [] } }),
    ], providers, scenarios);
    const s = buildStandings(m);
    expect(s[0].providerId).toBe("claude");
    expect(s[0].leader).toBe(true);
    expect(s[0].wins).toBe(1);
  });
  it("awards no win for an unscored trial", () => {
    const m = buildMatrix([
      mk({ ScenarioID: "checkout", ProviderID: "claude" }),
      mk({ ScenarioID: "checkout", ProviderID: "gpt4o" }),
    ], providers, scenarios);
    const s = buildStandings(m);
    expect(s.every(x => x.wins === 0)).toBe(true);
    expect(s.every(x => x.leader === false)).toBe(true);
  });
});

describe("buildOverallGauge", () => {
  it("counts passed cells across rows, ignoring cells with no data", () => {
    const twoScenarios = [{ id: "checkout" }, { id: "refund" }];
    const m = buildMatrix([
      mk({ ScenarioID: "checkout", ProviderID: "claude", ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] } }),
      mk({ ScenarioID: "checkout", ProviderID: "gpt4o", ConversationAssertions: { passed: false, failed: 1, total: 1, results: [] } }),
      mk({ ScenarioID: "refund", ProviderID: "claude", ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] } }),
      // refund:gpt4o has no run -> empty cell, excluded from the denominator
    ], providers, twoScenarios);
    const g = buildOverallGauge(m);
    expect(g.total).toBe(3);
    expect(g.passed).toBe(2);
    expect(g.passRate).toBe(67);
    expect(g.caption).toBe("2 / 3 trials passed");
  });
  it("excludes unscored (no-assertion) cells from the gauge", () => {
    const m = buildMatrix([
      mk({ ScenarioID: "checkout", ProviderID: "claude", ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] } }),
      mk({ ScenarioID: "checkout", ProviderID: "gpt4o" }),
    ], providers, scenarios);
    const g = buildOverallGauge(m);
    expect(g.total).toBe(1);
    expect(g.passed).toBe(1);
    expect(g.caption).toBe("1 / 1 trials passed");
  });
});

describe("buildMetrics", () => {
  it("produces trials/spend/latency/tokens metrics with a gold spend tone", () => {
    // Duration is nanoseconds on the wire (Go time.Duration): 800_000_000ns
    // and 1_200_000_000ns are 800ms and 1200ms respectively, so the p50
    // (lower-middle of the sorted pair) lands on 800ms -> "800ms".
    const results = [
      mk({
        ScenarioID: "checkout", ProviderID: "claude", Duration: 800_000_000,
        Cost: { total_cost_usd: 0.01, input_tokens: 1000, output_tokens: 500, input_cost_usd: 0.006, output_cost_usd: 0.004 },
        ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] },
      }),
      mk({
        ScenarioID: "checkout", ProviderID: "gpt4o", Duration: 1_200_000_000,
        Cost: { total_cost_usd: 0.02, input_tokens: 2000, output_tokens: 1000, input_cost_usd: 0.012, output_cost_usd: 0.008 },
        ConversationAssertions: { passed: false, failed: 1, total: 1, results: [] },
      }),
    ];
    const m = buildMatrix(results, providers, scenarios);
    const metrics = buildMetrics(results, m);
    expect(metrics).toHaveLength(4);
    const spend = metrics.find((x) => x.label.toLowerCase().includes("spend"))!;
    expect(spend.tone).toBe("gold");
    expect(spend.value).toBe("$0.0300");
    const trials = metrics.find((x) => x.label.toLowerCase().includes("trial"))!;
    expect(trials.value).toBe("2");
    const latency = metrics.find((x) => x.label === "P50")!;
    expect(latency.value).toBe("800ms");
    expect(latency.unit).toBeUndefined();
    const tokens = metrics.find((x) => x.label.toLowerCase().includes("token"))!;
    expect(tokens.unit).toBe("k");
    expect(tokens.dot).toBe("healthy");
  });
});

describe("buildSuiteTrend", () => {
  const run = (batch: string, provider: string, pass: boolean): RunResult =>
    mk({
      RunID: `${batch}_${provider}_default_checkout_h_1`,
      ScenarioID: "checkout", ProviderID: provider,
      ConversationAssertions: { passed: pass, failed: pass ? 0 : 1, total: 1, results: [] },
    });

  it("returns [] with fewer than two field runs (no trend)", () => {
    expect(buildSuiteTrend([])).toEqual([]);
    expect(buildSuiteTrend([run("2026-07-07T10-00-00Z", "claude", true)])).toEqual([]);
  });

  it("plots each full sweep's overall pass rate, oldest→newest", () => {
    const results = [
      // sweep A (older) covers both cells: 1 of 2 passed → 50%
      run("2026-07-07T10-00-00Z", "claude", true),
      run("2026-07-07T10-00-00Z", "gpt4o", false),
      // sweep B (newer) covers both cells: 2 of 2 → 100%
      run("2026-07-07T12-00-00Z", "claude", true),
      run("2026-07-07T12-00-00Z", "gpt4o", true),
    ];
    expect(buildSuiteTrend(results)).toEqual([50, 100]);
  });

  it("ignores partial batches — only full-field sweeps are points", () => {
    const results = [
      // sweep A (both cells)
      run("2026-07-07T10-00-00Z", "claude", true),
      run("2026-07-07T10-00-00Z", "gpt4o", true),
      // sweep B (both cells)
      run("2026-07-07T12-00-00Z", "claude", true),
      run("2026-07-07T12-00-00Z", "gpt4o", false),
      // partial batch C — only claude ran; NOT a sweep, excluded
      run("2026-07-07T13-00-00Z", "claude", false),
    ];
    expect(buildSuiteTrend(results)).toEqual([100, 50]);
  });

  it("hides the trend when only one full sweep exists (the rest are partial)", () => {
    const results = [
      run("2026-07-07T10-00-00Z", "claude", true),
      run("2026-07-07T10-00-00Z", "gpt4o", true), // one full sweep
      run("2026-07-07T12-00-00Z", "claude", true), // partial — ignored
    ];
    expect(buildSuiteTrend(results)).toEqual([]);
  });

  // sweeps builds N full-field sweeps (both cells) with distinct, ordered batch
  // ids so the trend keeps them chronological.
  const sweeps = (n: number): RunResult[] =>
    Array.from({ length: n }, (_, i) => {
      const batch = `2026-07-07T${String(i).padStart(2, "0")}-00-00Z`;
      return [run(batch, "claude", true), run(batch, "gpt4o", true)];
    }).flat();

  it("plots every sweep, uncapped, however many the field has been run", () => {
    expect(buildSuiteTrend(sweeps(20))).toHaveLength(20);
    expect(buildSuiteTrend(sweeps(40))).toHaveLength(40);
  });
});

describe("overlayWorkflowRun", () => {
  const wfGraph: WorkflowGraph = {
    nodes: [
      { id: "default", label: "default", kind: "entry", entry: true, terminal: false },
      { id: "intake", label: "intake", kind: "entry", entry: false, terminal: false },
      { id: "resolve", label: "resolve", kind: "output", entry: false, terminal: true },
      { id: "escalate", label: "escalate", kind: "agent", entry: false, terminal: true },
    ],
    edges: [
      { from: "intake", to: "resolve", label: "classified" },
      { from: "intake", to: "escalate", label: "unclear" },
    ],
  };

  it("marks the visited path gold and dims the unvisited sibling node/edge", () => {
    const run = mk({
      ScenarioID: "checkout", ProviderID: "claude",
      Messages: [
        { role: "system", content: "", meta: { _workflow_state: { current_state: "intake" } } },
        { role: "tool", content: "", meta: { _workflow_state: { current_state: "resolve", previous_state: "intake", transition: "classified" } } },
      ],
    });

    const out = overlayWorkflowRun(wfGraph, run);

    const intake = out.nodes.find((n) => n.id === "intake")!;
    const resolve = out.nodes.find((n) => n.id === "resolve")!;
    const escalate = out.nodes.find((n) => n.id === "escalate")!;
    expect(intake.dim).not.toBe(true);
    expect(resolve.dim).not.toBe(true);
    expect(escalate.dim).toBe(true);

    const visitedEdge = out.edges.find((e) => e.from === "intake" && e.to === "resolve")!;
    const unvisitedEdge = out.edges.find((e) => e.from === "intake" && e.to === "escalate")!;
    expect(visitedEdge.gold).toBe(true);
    expect(visitedEdge.dim).not.toBe(true);
    expect(unvisitedEdge.gold).not.toBe(true);
    expect(unvisitedEdge.dim).toBe(true);
  });

  it("never dims the default node even when unvisited by the run's path", () => {
    const run = mk({
      ScenarioID: "checkout", ProviderID: "claude",
      Messages: [
        { role: "system", content: "", meta: { _workflow_state: { current_state: "intake" } } },
      ],
    });
    const out = overlayWorkflowRun(wfGraph, run);
    const defaultNode = out.nodes.find((n) => n.id === "default")!;
    expect(defaultNode.dim).not.toBe(true);
  });

  it("leaves everything undimmed when no message carries workflow-state meta", () => {
    const run = mk({
      ScenarioID: "checkout", ProviderID: "claude",
      Messages: [{ role: "user", content: "hi" }, { role: "assistant", content: "hello" }],
    });
    const out = overlayWorkflowRun(wfGraph, run);
    expect(out.nodes.every((n) => n.dim !== true)).toBe(true);
    expect(out.edges.every((e) => e.dim !== true)).toBe(true);
  });

  it("returns the graph unchanged for an ActiveRun (no per-message workflow state exists yet)", () => {
    const active: ActiveRun = {
      runId: "r1", scenario: "checkout", provider: "claude", region: "us", startTime: "2026-07-07T00:00:00Z",
      turnIndex: 1, status: "running", costs: { inputTokens: 0, outputTokens: 0, totalCost: 0 },
      messages: [{ role: "user", content: "hi", index: 0 }],
    };
    const out = overlayWorkflowRun(wfGraph, active);
    expect(out.nodes.every((n) => n.dim !== true)).toBe(true);
    expect(out.edges.every((e) => e.dim !== true)).toBe(true);
  });

  it("returns the graph unchanged for an undefined run", () => {
    const out = overlayWorkflowRun(wfGraph, undefined);
    expect(out.nodes.every((n) => n.dim !== true)).toBe(true);
  });
});
