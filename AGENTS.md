# AGENTS.md — rules for AI coding assistants

PromptArena is a single Go module (`github.com/AltairaLabs/promptarena`) with two tools:
`arena/` (testing/eval CLI + engine) and `packc/` (pack compiler). It builds on the
published [PromptKit](https://github.com/AltairaLabs/PromptKit) `runtime` + `pkg` libraries.

## Before you commit
1. `go build ./...` and `go test ./... -count=1` must pass.
2. `golangci-lint run ./...` — **do not churn the inherited lint baseline.** This repo was
   extracted from PromptKit and carries pre-existing findings; CI lints **new** code only
   (`only-new-issues`). Hold your changes to standard; don't mass-fix or mass-suppress the
   baseline.
3. Conventional commits (`feat:`, `fix:`, `chore:`, `ci:`, `docs:`, `refactor:`).
4. Sign off every commit (DCO): `git commit -s`.

## Dependencies
- The committed `go.mod` pins **published** PromptKit `runtime`/`pkg`. To work against an
  unreleased PromptKit, use a local `go.work` overlay (never commit it):
  `go work use . ../PromptKit/runtime ../PromptKit/pkg`.
- CGO: voice/portaudio needs ALSA dev headers on Linux (`libasound2-dev`).

## Schemas
`schemas/v1alpha1/` is a **vendored snapshot** for local validation
(`PROMPTKIT_SCHEMA_SOURCE=local`). Canonical schemas are generated in PromptKit and served
from `promptkit.altairalabs.ai/schemas/`; refresh the snapshot, don't hand-edit it.

## Example READMEs are published documentation
Every `examples/<name>/README.md` is copied verbatim into the public docs site as
`arena/examples/<name>` by `scripts/prepare-examples-docs.sh` at docs build time, and
linked from the examples index. There is no review step between writing one and it
being published. Write them for someone outside the project:

- **No internal tracking.** Issue numbers, "Day N of the roadmap", milestone names.
  If a limitation is worth documenting, describe the limitation.
- **No links a reader can't follow.** `docs/local-backlog/` is local-only; citing a
  proposal by section number tells an outside reader nothing.
- **No working notes.** "Spike", "v1/v2", "deferred to", single-trial findings written
  as if settled. If the example is worth shipping, describe what it does.
- **No competitor comparisons.** Say what this does and when it fits. Naming another
  tool to say it's worse ages badly and invites argument; naming one as a vocabulary
  reference ("if you know X's terminology") is fine.
- **Run the commands.** Every flag in a README is a claim about the CLI. Check them
  (`promptarena run --help`); flags get renamed and READMEs don't notice.
- **No issue links, open or closed.** Docs are not a tracker view. An issue cited as
  a pending limitation becomes a lie the moment it closes, and nothing in the docs
  build notices — four voice caveats pointed at issues that had been closed for two
  months, telling readers a shipped feature was missing. Describe the limitation
  itself; a reader needs to know what does not work, not who is working on it.

The same applies to anything under `docs/src/content/docs/`.

**The other half of this rule lives on the issue.** If closing an issue would make a
statement in the docs untrue — a documented limitation, a "not supported yet", a
described behaviour — then updating those docs belongs in that issue's acceptance
criteria. That is what keeps them accurate: the docs change ships with the fix,
rather than waiting for someone to notice the drift later.

## Don't
- Don't re-introduce a dependency on `PromptKit/tools/*` — those paths were the source of
  this repo and no longer exist upstream.
- Don't reference monorepo paths (`examples/`, `docs/`, `schema-gen`) that live in PromptKit;
  some layout/drift tests are skipped pending an examples migration.
