/// <reference types="vitest" />
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import fs from "node:fs";
import path from "node:path";

// ATLAS_LOCAL=1 resolves @altairalabs/atlas and atlas-tokens to the sibling
// atlas-components checkout instead of the published package, so a change
// there is visible here without a version bump, a publish, or an install.
// It does need that checkout built — see the alias block below for why, and
// run tsup in watch mode to keep the loop fast:
//   pnpm --dir ../atlas-components --filter @altairalabs/atlas build -- --watch
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

// ATLAS_LOCAL consumes the local checkout's BUILD OUTPUT, never its source.
//
// Aliasing to react/src/index.ts used to seem friendlier — no build step — but
// it silently changed what you were testing. tsup marks react, react-dom and
// @xyflow/react external (packages/react/tsup.config.ts), so the built bundle
// takes those from THIS app. Source does not: each file resolves its own
// imports, and atlas-components carries its own React 18 while this app is on
// React 19. Two Reacts means a null dispatcher, and the symptom is a component
// blowing up with "Cannot read properties of null (reading 'useState')" —
// which is what the Workflow tab did, on unmodified code, while the published
// package was fine. The react aliases and resolve.dedupe below were already
// present and did not save it.
//
// dist is also the only honest thing to point at: it is exactly what gets
// published, so ATLAS_LOCAL now differs from a real install in version only.
//
// Order matters: alias entries match on prefix, so the stylesheet subpath has
// to be listed before the bare package.
//
// atlas-tokens is deliberately NOT built — that package ships src (see its
// "files" and "exports"), so src IS its published shape.
const localAtlasAliases = atlasLocal
  ? {
      "@altairalabs/atlas/styles.css": path.resolve(atlasRoot, "react/dist/index.css"),
      "@altairalabs/atlas": path.resolve(atlasRoot, "react/dist/index.js"),
      "@altairalabs/atlas-tokens": path.resolve(atlasRoot, "tokens/src"),
    }
  : {};

// Consuming a build means it can be stale, and a stale one is worse than no
// alias at all: you review a change that is not the change you made. Refuse to
// start rather than let that happen quietly.
function assertAtlasBuildIsFresh() {
  const dist = path.resolve(atlasRoot, "react/dist/index.js");
  if (!fs.existsSync(dist)) {
    throw new Error(
      `ATLAS_LOCAL=1 but ${dist} does not exist.\n` +
        `Build it first:  pnpm --dir ../atlas-components --filter @altairalabs/atlas build`,
    );
  }
  const builtAt = fs.statSync(dist).mtimeMs;
  const src = path.resolve(atlasRoot, "react/src");
  const newer: string[] = [];
  // Only files that can actually reach the bundle count. tsup's entry is
  // src/index.ts, so tests and stories are never in it — flagging those would
  // demand rebuilds that change nothing.
  const affectsBundle = (name: string) =>
    /\.(ts|tsx|css)$/.test(name) && !/\.(test|spec|stories)\./.test(name);
  const walk = (dir: string) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (affectsBundle(entry.name) && fs.statSync(full).mtimeMs > builtAt) {
        newer.push(path.relative(src, full));
      }
    }
  };
  walk(src);
  if (newer.length > 0) {
    throw new Error(
      `ATLAS_LOCAL=1 but the atlas build is stale — ${newer.length} source file(s) are newer ` +
        `than dist/index.js, e.g. ${newer.slice(0, 3).join(", ")}.\n` +
        `Rebuild:  pnpm --dir ../atlas-components --filter @altairalabs/atlas build\n` +
        `Or leave it running:  pnpm --dir ../atlas-components --filter @altairalabs/atlas build -- --watch`,
    );
  }
}

if (atlasLocal) {
  assertAtlasBuildIsFresh();
  // Loud on purpose — you are not looking at what CI will build.
  console.warn(
    "\n  ⚠  ATLAS_LOCAL=1 — resolving @altairalabs/atlas from the local checkout's build, not the published package.\n",
  );
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      ...localAtlasAliases,
      "@": path.resolve(__dirname, "./src"),
      // Every package tsup marks external in atlas-components must resolve to
      // THIS app's copy. The atlas bundle lives outside this tree, so a bare
      // external import resolves against atlas-components' node_modules
      // instead — and that tree is built against React 18 while this app is on
      // React 19. @xyflow/react was the one that bit: its own React 18 import
      // gave a null dispatcher inside ReactFlow, surfacing as
      // "Cannot read properties of null (reading 'useState')" on the Workflow
      // tab. react/react-dom were already pinned here; the rest were not.
      // Keep this list in step with packages/react/tsup.config.ts externals.
      "@xyflow/react": path.resolve(__dirname, "node_modules/@xyflow/react"),
      "@dagrejs/dagre": path.resolve(__dirname, "node_modules/@dagrejs/dagre"),
      "react-markdown": path.resolve(__dirname, "node_modules/react-markdown"),
      "remark-gfm": path.resolve(__dirname, "node_modules/remark-gfm"),
      "lucide-react": path.resolve(__dirname, "node_modules/lucide-react"),
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
      // Default to the serve command's default port; override with
      // VITE_API_TARGET when the backend landed elsewhere (e.g. 8080 taken).
      "/api": { target: process.env.VITE_API_TARGET || "http://localhost:8080", ws: true },
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
    //
    // Under ATLAS_LOCAL the alias has already rewritten the import to an
    // absolute path into atlas-components, which the "@altairalabs/atlas"
    // pattern no longer matches — so the bundle was left external, Node
    // resolved its imports from atlas-components' own node_modules, and the
    // app got a second React 18 alongside its React 19. That is the null
    // dispatcher behind "Cannot read properties of null (reading 'useState')".
    // Match the rewritten path too.
    server: {
      deps: {
        inline: [
          /@altairalabs\/atlas/,
          ...(atlasLocal ? [/atlas-components[\\/]packages[\\/](react|tokens)[\\/]/] : []),
          "react-markdown",
          "remark-gfm",
          "lucide-react",
        ],
      },
    },
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
