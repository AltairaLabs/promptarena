import type { RunRef } from "@/types";

// The matrix draws a *slice* of the field, not the whole thing. At thousands of
// scenarios against hundreds of providers nobody reads a hundred-thousand-cell
// grid, so the picker becomes the primary control and these caps bound what a
// first load renders — and, because selection is double-duty, what a field run
// costs. 50 × 25 is 1,250 cells: dense but legible, and a sane blast radius.
export const MAX_SLICE_SCENARIOS = 50;
export const MAX_SLICE_PROVIDERS = 25;

// seedSlice picks the initial selection: everything up to `cap`, in the order
// the config declares it. Authored order leads with the representative cases,
// and a field under the cap seeds fully selected — so every existing kit
// behaves exactly as it did before the cap existed.
export function seedSlice<T extends { id: string }>(items: T[], cap: number): string[] {
  return items.slice(0, cap).map((item) => item.id);
}

// refsInSlice narrows run locators to those whose scenario × provider cell is
// inside the picked slice. This is what keeps the fetch bounded: only these
// runs' full results (transcripts and all) are ever pulled from the server.
export function refsInSlice(
  refs: RunRef[],
  scenarioIds: string[],
  providerIds: string[],
): RunRef[] {
  if (scenarioIds.length === 0 || providerIds.length === 0) return [];
  const scenarios = new Set(scenarioIds);
  const providers = new Set(providerIds);
  return refs.filter((r) => scenarios.has(r.scenario_id) && providers.has(r.provider_id));
}
