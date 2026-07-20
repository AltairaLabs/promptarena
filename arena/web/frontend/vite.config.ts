/// <reference types="vitest" />
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

// ATLAS_LOCAL=1 resolves @altairalabs/atlas and atlas-tokens to the sibling
// atlas-components checkout instead of the published package, so a change
// there is visible here immediately — no version bump, no publish, no install.
//
// This exists because the alternative is using the registry as a feedback
// loop: publish, consume, find a problem, publish again. That produced six
// versions in two days, one of which existed only to fix the one before it.
// Publish when a change is finished, not to try it out.
//
// Opt-in and env-gated on purpose: the default resolution — for CI, for every
// other developer, and for any build that ships — stays the published package,
// so nothing can accidentally release against unpublished source.
const atlasLocal = process.env.ATLAS_LOCAL === "1";
const atlasRoot = path.resolve(__dirname, "../../../../atlas-components/packages");

// Order matters: alias entries match on prefix, so the stylesheet subpath has
// to be listed before the bare package or it would be rewritten into
// `.../react/src/index.ts/styles.css`.
//
// The stylesheet still comes from the local checkout's dist, because it is a
// build artefact (tsup concatenates the CSS each component imports). Component
// JS imports its own CSS from source, so a CSS change you make locally still
// shows up; a CSS-only change to a file no component imports needs
// `pnpm --filter @altairalabs/atlas build` in atlas-components.
const localAtlasAliases = atlasLocal
  ? {
      "@altairalabs/atlas/styles.css": path.resolve(atlasRoot, "react/dist/index.css"),
      "@altairalabs/atlas": path.resolve(atlasRoot, "react/src/index.ts"),
      "@altairalabs/atlas-tokens": path.resolve(atlasRoot, "tokens/src"),
    }
  : {};

if (atlasLocal) {
  // Loud on purpose — you are not looking at what CI will build.
  console.warn("\n  ⚠  ATLAS_LOCAL=1 — resolving @altairalabs/atlas from local source, not the published package.\n");
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      ...localAtlasAliases,
      "@": path.resolve(__dirname, "./src"),
      // Force the single (app) React everywhere — the link:'d @altairalabs/atlas
      // is symlinked outside this tree and would otherwise resolve its own React.
      react: path.resolve(__dirname, "node_modules/react"),
      "react-dom": path.resolve(__dirname, "node_modules/react-dom"),
      "react/jsx-runtime": path.resolve(__dirname, "node_modules/react/jsx-runtime"),
      "react/jsx-dev-runtime": path.resolve(__dirname, "node_modules/react/jsx-dev-runtime"),
      "react-dom/client": path.resolve(__dirname, "node_modules/react-dom/client"),
    },
    dedupe: ["react", "react-dom"],
  },
  server: {
    proxy: {
      "/api": { target: "http://localhost:8080", ws: true },
    },
  },
  build: {
    outDir: "dist",
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    css: false,
    // The linked @altairalabs/atlas package is symlinked outside this tree, so
    // its `react` peer would otherwise resolve to atlas-components' own React.
    // Inline it (with resolve.dedupe above) so tests use the app's single React.
    server: { deps: { inline: [/@altairalabs\/atlas/, "react-markdown", "remark-gfm", "lucide-react"] } },
    include: ["src/**/*.{test,spec}.{ts,tsx}"],
    coverage: {
      provider: "v8",
      reporter: ["text", "lcov"],
      reportsDirectory: "coverage",
      include: ["src/**/*.{ts,tsx}"],
      exclude: [
        "src/**/*.{test,spec}.{ts,tsx}",
        "src/main.tsx",
        "src/vite-env.d.ts",
        "src/**/*.d.ts",
      ],
    },
  },
});
