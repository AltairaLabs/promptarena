# Contributing to PromptArena

PromptArena is a single Go module (`github.com/AltairaLabs/promptarena`) containing the
`arena` (testing/eval CLI + engine) and `packc` (pack compiler) tools. It builds on the
published [PromptKit](https://github.com/AltairaLabs/PromptKit) runtime + `pkg` libraries.

## Build & test

```bash
go build ./...
go test ./... -count=1
golangci-lint run ./...
```

CGO note: the voice/portaudio code needs ALSA dev headers on Linux
(`sudo apt-get install -y libasound2-dev`).

## Working against pre-release PromptKit runtime/pkg

The committed `go.mod` pins published `runtime`/`pkg` versions. To develop against an
unreleased PromptKit (local checkout), use a `go.work` overlay (not committed):

```bash
go work init
go work use . ../PromptKit/runtime ../PromptKit/pkg
```

## Schemas

`schemas/v1alpha1/` is a vendored snapshot used for local validation
(`PROMPTKIT_SCHEMA_SOURCE=local`). The canonical schemas are generated in PromptKit and
served from `promptkit.altairalabs.ai/schemas/`; refresh the snapshot from there.

## Conventions

- Conventional commits (`feat:`, `fix:`, `chore:`, `ci:`, `docs:`, `refactor:`)
- Sign off every commit (DCO): `git commit -s`
- CI lints **new** code (`only-new-issues`); the repo carries an inherited lint baseline
  from the PromptKit extraction — hold new work to standard, don't churn the baseline.

## License

By contributing you agree your contributions are licensed under Apache 2.0.
