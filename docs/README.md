# PromptArena Documentation

The [Starlight](https://starlight.astro.build/) documentation site for PromptArena
and PackC, published to <https://promptarena.altairalabs.ai>.

## Develop

```bash
npm install
npm run dev        # local dev server with hot reload
npm run build      # production build into dist/
npm run preview    # serve the production build
npm run check-links  # crawl the built site for broken internal links
```

## Structure

```
src/content/docs/
├── arena/      # PromptArena docs (tutorials, how-to, reference, explanation)
└── packc/      # PackC docs (tutorials, how-to, reference, explanation)
```

Pages follow the [Diátaxis](https://diataxis.fr/) framework.

### Build-time generated content

Some pages are generated during `prebuild` and are gitignored — they are
regenerated on every build:

- **Examples** (`arena/examples/`) — copied from `examples/*/README.md` by
  [`scripts/prepare-examples-docs.sh`](../scripts/prepare-examples-docs.sh).
- **Deploy adapters** (`arena/*/deploy/{agentcore,omnia}/`) — pulled from the
  deploy adapter repos ([promptarena-deploy-agentcore](https://github.com/AltairaLabs/promptarena-deploy-agentcore),
  [promptarena-deploy-omnia](https://github.com/AltairaLabs/promptarena-deploy-omnia))
  at build time by [`scripts/fetch-adapter-docs.mjs`](./scripts/fetch-adapter-docs.mjs).
  Each adapter maintains its own docs; the script maps them into the arena deploy
  sections and rewrites links. It uses `gh api`, so CI passes `GH_TOKEN`; set
  `SKIP_ADAPTER_DOCS=1` to skip the fetch for fast local builds.

## Versioning

Docs are versioned by **minor** release (`major.minor`). The latest version is
served at the site root; archived minor versions live under `/vX-Y/` (e.g.
`/v1-5/`) with a version switcher in the header and a banner on non-latest pages.
Version metadata lives in [`versions.json`](./versions.json); the release
workflow archives the outgoing minor and updates it automatically. See
[`scripts/update-versions.js`](./scripts/update-versions.js).

## Notes

- Runtime, SDK, and API reference docs live in the
  [PromptKit docs](https://promptkit.altairalabs.ai); cross-references point there.
- JSON schemas remain hosted at
  <https://promptkit.altairalabs.ai/schemas/v1alpha1/> — they are not served
  from this site.
