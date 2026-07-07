import { describe, it, expect } from "vitest";
import { buildMatrix, buildStandings, buildOverallGauge, buildMetrics } from "./arenaView";
import type { RunResult } from "@/types";

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
