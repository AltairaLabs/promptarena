import { describe, it, expect } from "vitest";
import {
  MAX_SLICE_PROVIDERS,
  MAX_SLICE_SCENARIOS,
  refsInSlice,
  seedSlice,
} from "./fieldSlice";
import type { RunRef } from "@/types";

const ids = (n: number, prefix = "s") =>
  Array.from({ length: n }, (_, i) => ({ id: `${prefix}${i + 1}` }));

describe("seedSlice", () => {
  it("selects everything when the field fits under the cap", () => {
    expect(seedSlice(ids(11), MAX_SLICE_SCENARIOS)).toHaveLength(11);
  });

  it("selects exactly the cap, in config order, when the field is bigger", () => {
    const seeded = seedSlice(ids(1204), MAX_SLICE_SCENARIOS);
    expect(seeded).toHaveLength(MAX_SLICE_SCENARIOS);
    expect(seeded[0]).toBe("s1");
    expect(seeded[MAX_SLICE_SCENARIOS - 1]).toBe(`s${MAX_SLICE_SCENARIOS}`);
    expect(seeded).not.toContain(`s${MAX_SLICE_SCENARIOS + 1}`);
  });

  it("caps providers independently of scenarios", () => {
    expect(seedSlice(ids(300, "p"), MAX_SLICE_PROVIDERS)).toHaveLength(MAX_SLICE_PROVIDERS);
  });

  it("handles an empty field", () => {
    expect(seedSlice([], MAX_SLICE_SCENARIOS)).toEqual([]);
  });
});

function ref(scenario: string, provider: string, n: number): RunRef {
  return { run_id: `r${n}`, scenario_id: scenario, provider_id: provider };
}

describe("refsInSlice", () => {
  const refs = [
    ref("checkout", "claude", 1),
    ref("checkout", "gpt4o", 2),
    ref("refund", "claude", 3),
    ref("refund", "gpt4o", 4),
  ];

  it("keeps only runs whose cell is inside the picked slice", () => {
    const kept = refsInSlice(refs, ["checkout"], ["claude"]);
    expect(kept.map((r) => r.run_id)).toEqual(["r1"]);
  });

  it("requires both axes to match, not either", () => {
    // "refund × claude" matches the scenario but not the provider.
    const kept = refsInSlice(refs, ["checkout", "refund"], ["gpt4o"]);
    expect(kept.map((r) => r.run_id)).toEqual(["r2", "r4"]);
  });

  it("returns nothing when either axis is empty", () => {
    expect(refsInSlice(refs, [], ["claude"])).toEqual([]);
    expect(refsInSlice(refs, ["checkout"], [])).toEqual([]);
  });

  it("ignores runs for scenarios or providers no longer in the config", () => {
    const stale = [...refs, ref("deleted-scenario", "claude", 5)];
    const kept = refsInSlice(stale, ["checkout", "refund"], ["claude", "gpt4o"]);
    expect(kept.map((r) => r.run_id)).toEqual(["r1", "r2", "r3", "r4"]);
  });
});
