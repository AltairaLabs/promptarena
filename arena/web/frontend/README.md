# Arena web UI

React + Vite SPA, embedded into the Go binary via `go:embed frontend/dist`.

```
npm run dev      # dev server (proxies /api to :8080)
npm run build    # tsc -b && vite build  — what CI runs
npm run test     # vitest
```

## Working on @altairalabs/atlas at the same time

The UI consumes `@altairalabs/atlas` as a **published package** from GitHub
Packages. That is deliberate — it is what CI installs and what ships — but it
makes a change in `atlas-components` invisible here until it is released.

Do **not** publish to try something out. Set `ATLAS_LOCAL=1` and the build
resolves `@altairalabs/atlas` and `@altairalabs/atlas-tokens` from the sibling
`atlas-components` checkout instead:

```
npm run dev:atlas     # dev server against local atlas source
npm run test:atlas    # tests against local atlas source
npm run build:atlas   # bundle against local atlas source
```

It expects `atlas-components` beside `promptarena`:

```
repos/
  atlas-components/
  promptarena/
```

Vite prints a warning when the flag is on, because you are no longer looking at
what CI will build.

### What it does and does not cover

- **JS/TSX** comes from `atlas-components/packages/react/src` — edits show up on
  the next reload, including exports that do not exist in any release yet.
- **Tokens** come from `packages/tokens/src`.
- **The stylesheet** (`@altairalabs/atlas/styles.css`) still comes from that
  checkout's `dist/`, because it is a build artefact. Component CSS imported by
  a component still arrives from source; a CSS-only change to a file no
  component imports needs `pnpm --filter @altairalabs/atlas build` in
  atlas-components.
- **Types are not aliased.** `tsc -b` still checks against the installed
  package, so a new Atlas export will fail typecheck until it is published.
  `build:atlas` therefore skips `tsc -b` — use it to *see* the change, and the
  normal `npm run build` to verify before merging.

### Then release once

Land the atlas change, cut one release, and take it here with
`pnpm update @altairalabs/atlas`. The version history should read as a list of
finished changes — not as a trail of attempts.
