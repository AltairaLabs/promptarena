---
title: PackC Reference
description: Command reference for packc, the compiler that turns PromptArena prompt configs into PromptPack JSON files.
sidebar:
  order: 0
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - packc/compiler/compiler.go
    - packc/main_interactive.go
    - packc/schema_validator.go
---
`packc` compiles prompt configs into [PromptPack](https://promptpack.org) JSON files (`.pack.json`), validates them, and prints their contents. It compiles with the PromptKit runtime's pack compiler and validates against the PromptPack schema embedded in that runtime.

## Commands

| Command | Description |
|---|---|
| [`compile`](/packc/reference/compile/) | Compile every prompt an Arena config lists into one pack. |
| [`compile-prompt`](/packc/reference/compile-prompt/) | Compile one PromptConfig file into a single-prompt pack. |
| [`validate`](/packc/reference/validate/) | Validate a pack file against the PromptPack schema and structure rules. |
| [`inspect`](/packc/reference/inspect/) | Print a summary of a pack file. |
| [`version`](/packc/reference/version/) | Print the `packc` version. |
| `completion <shell>` | Print a shell completion script for `bash`, `zsh`, `fish` or `powershell`. See [Install packc](/packc/how-to/install/). |
| `help` | Print usage. `--help` and `-h` before a command do the same. |

An unknown command prints `Unknown command: <name>` and the usage, and exits with code `1`. `packc` with no command prints the usage and exits with code `1`.

`compile` and `compile-prompt` accept `--help`, which prints their flags. `validate` and `inspect` treat any first argument, `--help` included, as the pack file path.

## Example

```bash
packc compile -c config.arena.yaml -o packs/app.pack.json --id my-app
packc validate packs/app.pack.json
packc inspect packs/app.pack.json
```

## Related

- [PackC tutorials](/packc/tutorials/)
- [PackC how-to guides](/packc/how-to/)
- [Pack format](/packc/explanation/pack-format/)
