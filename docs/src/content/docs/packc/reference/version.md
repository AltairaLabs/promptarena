---
title: version
description: Output of packc version, and how the version reaches the compilation record of every pack packc compiles.
sidebar:
  order: 5
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - .goreleaser.yml
    - Makefile
    - npm/packc/package.json
    - packc/main.go
    - packc/main_interactive.go
---
`packc version` prints the version of the `packc` binary.

## Synopsis

```bash
packc version
```

`version` has no flags or arguments; any that follow it are ignored.

## Output

```text
packc <version>
```

| Build | `<version>` |
|---|---|
| A release binary: a GitHub release archive, the Homebrew cask or the npm package | the release version without its `v` prefix, for example `2.4.0` |
| `go install`, `go build` or `make build-go` | `dev` |

The version is set at link time with `-X main.version=...`. A build without that flag reports `dev`.

## Version in compiled packs

[`packc compile`](/packc/reference/compile/) and [`packc compile-prompt`](/packc/reference/compile-prompt/) write the same version into each pack's `compilation` record, prefixed with `packc-`:

```json
"compilation": {
  "compiled_with": "packc-2.4.0",
  "created_at": "2026-11-06T18:08:46Z",
  "schema": "v1"
}
```

[`packc inspect`](/packc/reference/inspect/) prints it on its `Compilation:` line.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Always. |

## Related

- [Install packc](/packc/how-to/install/)
- [inspect](/packc/reference/inspect/): shows the compiler version a pack was built with
