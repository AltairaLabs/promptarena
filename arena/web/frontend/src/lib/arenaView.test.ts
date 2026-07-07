import { describe, it, expect } from "vitest";
import {
  buildMatrix,
  buildStandings,
  buildOverallGauge,
  buildMetrics,
  buildTranscript,
  buildTerminalLines,
  buildTrend,
  buildAgentFlow,
} from "./arenaView";
import type { RunResult, TrialCell } from "@/types";

const mk = (o: Partial<RunResult>): RunResult => ({
  RunID: o.RunID ?? "r", PromptPack: "", Region: "", ScenarioID: o.ScenarioID!, ProviderID: o.ProviderID!,
  Params: {}, Messages: [], Commit: {}, Cost: (o.Cost as any) ?? { total_cost_usd: 0, input_tokens: 0, output_tokens: 0, input_cost_usd: 0, output_cost_usd: 0 },
  Violations: [], StartTime: o.StartTime ?? "2026-07-07T00:00:00Z", EndTime: o.EndTime ?? "2026-07-07T00:00:01Z",
  Duration: o.Duration ?? 1000, Error: o.Error ?? "", SelfPlay: false, PersonaID: "", MediaOutputs: [], A2AAgents: [],
  ConversationAssertions: o.ConversationAssertions, ...o,
} as RunResult);

const providers = [{ id: "claude" }, { id: "gpt4o" }];
const scenarios = [{ id: "checkout" }];

describe("buildMatrix", () => {
  it("places each result in its scenario×provider cell", () => {
    const m = buildMatrix([
      mk({ ScenarioID: "checkout", ProviderID: "claude", ConversationAssertions: { passed: true, failed: 0, total: 4, results: [] } }),
      mk({ ScenarioID: "checkout", ProviderID: "gpt4o", ConversationAssertions: { passed: false, failed: 2, total: 4, results: [] } }),
    ], providers, scenarios);
    expect(m.rows).toHaveLength(1);
    expect(m.rows[0].cells).toHaveLength(2);
    const claude = m.rows[0].cells.find(c => c.providerId === "claude")!;
    expect(claude.passRate).toBe(100);
    expect(claude.best).toBe(true);
    expect(m.rows[0].cells.find(c => c.providerId === "gpt4o")!.passRate).toBe(50);
  });
  it("uses the most recent run when a cell has duplicates", () => {
    const m = buildMatrix([
      mk({ ScenarioID: "checkout", ProviderID: "claude", EndTime: "2026-07-07T00:00:01Z", ConversationAssertions: { passed: false, failed: 1, total: 1, results: [] } }),
      mk({ ScenarioID: "checkout", ProviderID: "claude", EndTime: "2026-07-07T00:00:09Z", ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] } }),
    ], providers, scenarios);
    expect(m.rows[0].cells.find(c => c.providerId === "claude")!.passRate).toBe(100);
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
    expect(g.caption).toBe("2 / 3 passed");
  });
});

describe("buildMetrics", () => {
  it("produces trials/spend/latency/tokens metrics with a gold spend tone", () => {
    const results = [
      mk({
        ScenarioID: "checkout", ProviderID: "claude", Duration: 800,
        Cost: { total_cost_usd: 0.01, input_tokens: 1000, output_tokens: 500, input_cost_usd: 0.006, output_cost_usd: 0.004 },
        ConversationAssertions: { passed: true, failed: 0, total: 1, results: [] },
      }),
      mk({
        ScenarioID: "checkout", ProviderID: "gpt4o", Duration: 1200,
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
    const latency = metrics.find((x) => x.label.toLowerCase().includes("latency"))!;
    expect(latency.unit).toBe("ms");
    const tokens = metrics.find((x) => x.label.toLowerCase().includes("token"))!;
    expect(tokens.unit).toBe("k");
    expect(tokens.dot).toBe("healthy");
  });
});

describe("buildTranscript", () => {
  it("gives assistant messages the ion-cyan accent", () => {
    const run = mk({ ScenarioID: "checkout", ProviderID: "claude", Messages: [
      { role: "assistant", content: "hi there" },
    ] });
    const t = buildTranscript(run);
    expect(t).toHaveLength(1);
    expect(t[0].role).toBe("assistant");
    expect(t[0].accent).toBe("var(--ion-cyan)");
    expect(t[0].bg).toContain("var(--ion-cyan)");
  });

  it("gives system/user/tool messages their accents, defaulting unknown roles", () => {
    const run = mk({ ScenarioID: "checkout", ProviderID: "claude", Messages: [
      { role: "system", content: "sys" },
      { role: "user", content: "hello" },
      { role: "tool", content: "result" },
      { role: "narrator", content: "???" },
    ] });
    const t = buildTranscript(run);
    expect(t.map((m) => m.accent)).toEqual([
      "var(--nebula-violet)",
      "var(--starlight-300)",
      "var(--amber-500)",
      "var(--starlight-300)",
    ]);
  });

  it("surfaces a tool call as a tool entry", () => {
    const run = mk({ ScenarioID: "checkout", ProviderID: "claude", Messages: [
      { role: "assistant", content: "", tool_calls: [{ id: "1", name: "search_docs", args: { q: "refund policy" } }] },
    ] });
    const t = buildTranscript(run);
    expect(t[0].tool?.name).toBe("search_docs");
    expect(t[0].tool?.body).toContain("refund policy");
  });

  it("builds meta from cost + latency and asserts from validations", () => {
    const run = mk({ ScenarioID: "checkout", ProviderID: "claude", Messages: [
      {
        role: "assistant", content: "done", latency_ms: 820,
        cost_info: { total_cost_usd: 0.0069, input_tokens: 10, output_tokens: 5, input_cost_usd: 0.004, output_cost_usd: 0.0029 },
        validations: [{ validator_type: "no_pii", passed: true }],
      },
    ] });
    const t = buildTranscript(run);
    expect(t[0].meta).toBe("$0.0069 · 820ms");
    expect(t[0].asserts).toEqual([{ name: "no_pii", ok: true }]);
  });

  it("returns [] for an undefined run", () => {
    expect(buildTranscript(undefined)).toEqual([]);
  });

  it("reads an ActiveRun's live messages", () => {
    const active = {
      runId: "r1", scenario: "checkout", provider: "claude", region: "us", startTime: "2026-07-07T00:00:00Z",
      turnIndex: 1, status: "running" as const, costs: { inputTokens: 0, outputTokens: 0, totalCost: 0 },
      messages: [{ role: "user", content: "hi", index: 0 }],
    };
    const t = buildTranscript(active);
    expect(t).toHaveLength(1);
    expect(t[0].role).toBe("user");
    expect(t[0].content).toBe("hi");
  });
});

describe("buildTerminalLines", () => {
  const baseCell: TrialCell = {
    scenarioId: "checkout", providerId: "claude", key: "checkout:claude",
    passRate: 100, passed: true, best: true, costUsd: 0.0069, latencyMs: 820, runId: "r1", hasData: true,
  };

  it("synthesizes the command line and a success line when the cell passed", () => {
    const lines = buildTerminalLines(baseCell, "checkout", "claude");
    expect(lines[0].text).toBe("promptarena run --scenario checkout --provider claude");
    const success = lines.find((l) => l.type === "success")!;
    expect(success.text).toContain("✓");
    expect(success.text).toContain("$0.0069");
    expect(success.text).toContain("820ms");
  });

  it("synthesizes an error line when the cell failed", () => {
    const failed: TrialCell = { ...baseCell, passed: false, passRate: 0 };
    const lines = buildTerminalLines(failed, "checkout", "claude");
    const error = lines.find((l) => l.type === "error")!;
    expect(error.text).toContain("✗");
  });

  it("handles an empty cell", () => {
    const lines = buildTerminalLines(undefined, "checkout", "claude");
    expect(lines[0].text).toBe("promptarena run --scenario checkout --provider claude");
    expect(lines.some((l) => l.type === "success" || l.type === "error")).toBe(false);
  });
});

describe("buildTrend", () => {
  it("returns [] when there are fewer than 3 results", () => {
    expect(buildTrend([])).toEqual([]);
    expect(buildTrend([mk({ ScenarioID: "checkout", ProviderID: "claude" })])).toEqual([]);
  });

  it("derives a chronological pass-rate series from historical runs", () => {
    const results = [
      mk({ ScenarioID: "checkout", ProviderID: "claude", StartTime: "2026-07-07T00:00:02Z", Error: "boom" }),
      mk({ ScenarioID: "checkout", ProviderID: "claude", StartTime: "2026-07-07T00:00:00Z", Error: "" }),
      mk({ ScenarioID: "checkout", ProviderID: "claude", StartTime: "2026-07-07T00:00:01Z", Error: "boom" }),
    ];
    const trend = buildTrend(results);
    // sorted by StartTime ascending: pass(100), fail(0), fail(0)
    expect(trend).toEqual([100, 0, 0]);
  });
});

describe("buildAgentFlow", () => {
  it("returns entry/agent/tool/output nodes and connecting edges for one tool call", () => {
    const run = mk({
      ScenarioID: "checkout", ProviderID: "claude",
      Messages: [
        { role: "user", content: "refund my order" },
        { role: "assistant", content: "checking", tool_calls: [{ id: "1", name: "lookup_order", args: {} }] },
        { role: "tool", content: "order found" },
        { role: "assistant", content: "here's your refund status" },
      ],
    });
    const { nodes, edges } = buildAgentFlow(run);

    expect(nodes.map((n) => n.kind).sort()).toEqual(["agent", "agent", "entry", "output", "tool"].sort());
    const entry = nodes.find((n) => n.kind === "entry")!;
    const tool = nodes.find((n) => n.kind === "tool")!;
    const output = nodes.find((n) => n.kind === "output")!;
    expect(tool.label).toBe("lookup_order");
    expect(output.label).toBe("resolved");

    // every node fits the fixed 360x150 viewBox
    for (const n of nodes) {
      expect(n.x).toBeGreaterThanOrEqual(0);
      expect(n.x).toBeLessThanOrEqual(360);
      expect(n.y).toBeGreaterThanOrEqual(0);
      expect(n.y).toBeLessThanOrEqual(150);
    }

    // edges connect entry all the way through to output
    const byFrom = new Map(edges.map((e) => [e.from, e.to]));
    let cur = entry.id;
    const visited = [cur];
    while (byFrom.has(cur) && visited.length < nodes.length + 1) {
      cur = byFrom.get(cur)!;
      visited.push(cur);
    }
    expect(visited[visited.length - 1]).toBe(output.id);
  });

  it("marks the output node failed when the run errored", () => {
    const run = mk({ ScenarioID: "checkout", ProviderID: "claude", Error: "boom", Messages: [{ role: "user", content: "hi" }] });
    const { nodes } = buildAgentFlow(run);
    const output = nodes.find((n) => n.kind === "output")!;
    expect(output.label).toBe("failed");
  });

  it("returns empty nodes/edges for an undefined run", () => {
    expect(buildAgentFlow(undefined)).toEqual({ nodes: [], edges: [] });
  });

  it("marks the terminal node running (not resolved) for a live in-progress ActiveRun", () => {
    const active = {
      runId: "r1", scenario: "checkout", provider: "claude", region: "us", startTime: "2026-07-07T00:00:00Z",
      turnIndex: 1, status: "running" as const, costs: { inputTokens: 0, outputTokens: 0, totalCost: 0 },
      messages: [{ role: "user", content: "hi", index: 0 }],
    };
    const { nodes, edges } = buildAgentFlow(active);
    const terminal = nodes.find((n) => n.id === "output")!;
    expect(terminal.label).toBe("running");
    expect(terminal.kind).not.toBe("output");

    const incoming = edges.find((e) => e.to === terminal.id)!;
    expect(incoming.gold).not.toBe(true);
  });
});
