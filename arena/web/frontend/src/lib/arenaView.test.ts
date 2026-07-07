import { describe, it, expect } from "vitest";
import { buildMatrix, buildStandings } from "./arenaView";
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
