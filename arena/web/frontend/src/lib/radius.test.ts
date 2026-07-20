import { readFileSync } from "node:fs";
import { describe, it, expect } from "vitest";

// index.css must declare --radius-* under @theme for Tailwind to emit the
// rounded-* utilities, and that is the same variable name Atlas defines — so
// it cannot be aliased without going circular. The values are therefore
// copies, and copies drift. This fails the build if they stop matching.
//
// Read from disk rather than importing: Vitest stubs CSS imports (css: false),
// so even `?raw` yields an empty string here. Paths are relative to the Vitest
// root, which is this package.
const LOCAL_CSS = "src/index.css";
const ATLAS_SPACING = "node_modules/@altairalabs/atlas-tokens/src/spacing.css";

function parseRadii(css: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const m of css.matchAll(/--radius-([a-z0-9]+):\s*([^;]+);/g)) {
    const [, name, value] = m;
    // Keep the first declaration of each name — Atlas declares its scale once.
    if (!(name in out)) out[name] = value.trim();
  }
  return out;
}

describe("radius tokens", () => {
  it("match the Atlas scale they mirror", () => {
    const local = parseRadii(readFileSync(LOCAL_CSS, "utf8"));
    const atlas = parseRadii(readFileSync(ATLAS_SPACING, "utf8"));

    const mirrored = Object.keys(local);
    expect(mirrored.length).toBeGreaterThan(0);

    for (const name of mirrored) {
      expect(atlas[name], `Atlas has no --radius-${name}`).toBeDefined();
      expect(local[name], `--radius-${name} drifted from Atlas`).toBe(atlas[name]);
    }
  });
});
