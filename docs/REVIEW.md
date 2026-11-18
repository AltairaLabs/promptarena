# Docs review: PromptArena notes

The review procedure is the `docs-review` plugin's skill (from
[AltairaLabs/skills](https://github.com/AltairaLabs/skills)), configured by
`docs-kit.toml`, which points here for this repository's part of it. Where
this file and the skill disagree, this file wins. `docs/STYLE.md` is the
authority on what a page says.

## Getting the skill

`.claude/settings.json` declares the `altairalabs` marketplace and enables
`docs-review@altairalabs`. After trusting the folder, run
`/plugin install docs-review@altairalabs` if Claude Code has not offered it.
The skill runs as `/docs-review:docs-review <page-or-directory>`,
`/docs-review:docs-review --stale [path]` for upkeep.

The mechanical checks are the plugin's `docs-kit` tool, pinned in
`DOCS_KIT` in the `Makefile` and the docs workflows. `make docs-voice`,
`make docs-stale` and `make docs-coverage` run it through `uvx`, so you need
[uv](https://docs.astral.sh/uv/) installed. Bump the pin in the `Makefile`,
both workflows and `.claude/settings.json` together.

## Product

PromptArena, the `promptarena` and `packc` CLIs. Build with `make build-go`
(binaries in `bin/`); `--help` on any subcommand is the authority on flags.

## Quadrants

`arena/<quadrant>/` and `packc/<quadrant>/`, where `<quadrant>` is
`tutorials`, `how-to`, `reference` or `explanation`. `arena/index.md` and
`arena/why-arena.md` are explanation.

## Verify sources (`{sources}`)

`arena/` (`arena/cmd/promptarena/` for commands and flags,
`arena/arenaconfig/` for config fields, `arena/assertions/` for assertion
types, `arena/engine/` for run behaviour), `packc/`, and the schemas in
`schemas/v1alpha1/` (generated from the Go config types by `make schemas`;
never hand-edited). For PromptKit runtime behaviour, read the module cache at
the version pinned in `go.mod`
(`go list -m -f '{{.Dir}}' github.com/AltairaLabs/PromptKit/runtime`), not a
sibling checkout.

## Generated and fetched pages

- **Every `examples/<name>/README.md` is published verbatim** as a public
  docs page by `scripts/prepare-examples-docs.sh`, with no review step in
  between. This is where internal working notes leak out; see AGENTS.md.
  Review and edit the README. `docs/src/content/docs/arena/examples/` is
  generated from them and gitignored; never edit it.
- `docs/src/content/docs/arena/*/deploy/<adapter>/` is **fetched from the
  adapter repositories** by `docs/scripts/fetch-adapter-docs.mjs`. Fix those
  in `promptarena-deploy-<adapter>`.

## Checking commands

- When checking flags, grep `examples/*/README.md` as well as the docs tree:
  the published copies of those READMEs exist only after
  `scripts/prepare-examples-docs.sh` runs.
- Assertion and eval types have a registry behind them; regenerate the
  catalog with `promptarena gen-reference` and compare against it.
- The CLI is `promptarena`. Quickstarts once told readers to run `arena`,
  which no binary provides, so the first command of two quickstarts could not
  be followed.
- The PromptKit runtime and PromptArena both emit `workflow state transition`
  with identical fields, so raw log counts are twice the real number.

## Link check

`commands.check_links` binds port 4455 because a preview server already on
the default port makes the checker bind elsewhere and fail to start.
`npx astro preview stop` clears a stale preview daemon.

## Commits

Conventional (`docs:`), signed off: `git commit -s`. Hold new code to
`golangci-lint`, but do not churn the inherited baseline.
