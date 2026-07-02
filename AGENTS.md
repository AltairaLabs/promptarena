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

## Don't
- Don't re-introduce a dependency on `PromptKit/tools/*` — those paths were the source of
  this repo and no longer exist upstream.
- Don't reference monorepo paths (`examples/`, `docs/`, `schema-gen`) that live in PromptKit;
  some layout/drift tests are skipped pending an examples migration.
