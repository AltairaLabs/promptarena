---
title: compile
description: Flags, input, output and exit codes of packc compile, which compiles every prompt an Arena config lists into one PromptPack file.
sidebar:
  order: 1
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - packc/compiler/check.go
    - packc/compiler/compiler.go
    - packc/compiler/helpers_integration.go
    - packc/main.go
    - packc/main_interactive.go
    - schemas/v1alpha1/arena.json
---
`packc compile` loads an Arena config, compiles every prompt config it lists into one [PromptPack](https://promptpack.org) JSON file, validates the result, and writes it. It is the command for building a pack from a project.

## Synopsis

```bash
packc compile [-c <arena.yaml>] [-o <pack-file>] [--id <pack-id>]
```

## Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `-c`, `--config` | string | `config.arena.yaml` | Path to the Arena config file. |
| `-o`, `--output` | string | `<id>.pack.json` | Path of the pack file to write. Missing parent directories are created. |
| `--id` | string | current directory name, sanitized | Pack ID, written to the pack's `id` and `name` fields. |

An explicit `--id` must match `^[a-z][a-z0-9-]*$`, the PromptPack pattern for a pack ID. Any other value fails the compile before the config is loaded:

```text
Compilation failed: [pack_id] invalid pack ID "Assistant_1": must start with a lowercase letter and contain only lowercase letters, digits and dashes (pattern ^[a-z][a-z0-9-]*$)
```

The default ID is the name of the directory you run `packc` from, lowercased, with spaces turned into hyphens, other characters outside `a-z`, `0-9` and `-` removed, repeated hyphens collapsed, and leading and trailing hyphens trimmed. A name that does not then start with a letter gets a `pack-` prefix, and an empty one becomes `pack`.

## Input

The Arena config is a `kind: Arena` manifest. Its schema requires `spec.providers` and `spec.defaults`; `packc compile` reads neither, so empty values satisfy it:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: customer-support
spec:
  prompt_configs:
    - id: support
      file: prompts/support.yaml
    - id: escalation
      file: prompts/escalation.yaml
  tools:
    - file: tools/search_kb.yaml
  providers: []
  defaults: {}
```

File paths resolve relative to the Arena config's directory. These `spec` fields reach the pack:

| Arena field | Pack field |
|---|---|
| `prompt_configs`, `prompt_specs` | `prompts`, keyed by each prompt's `spec.task_type` |
| `tools` | `tools`, keyed by tool name |
| `pack_evals` | `evals` |
| `workflow` | `workflow` |
| `agents` | `agents` |
| `compositions` | `compositions` |
| `skills` | `skills`, with paths made relative to the config directory |
| `pack_metadata` | `metadata` |

The pack's `template_engine` is copied from one of the prompts that sets `spec.template_engine`; packc does not check that the prompts agree. Without `pack_metadata`, the pack's `metadata` is taken from the prompt whose `prompt_configs` ID sorts first among those that set `spec.metadata`, and the compile prints a warning naming it. See [Configuration Schema](/arena/reference/config-schema/) for every Arena field.

## Output

The pack's top-level `version` is `v1.0.0` and its `description` is `Pack containing <n> prompts`. Each prompt keeps its own `version`. `compilation.compiled_with` is `packc-<version>`, where `<version>` is what [`packc version`](/packc/reference/version/) prints.

```json
{
  "$schema": "https://promptpack.org/schema/latest/promptpack.schema.json",
  "compilation": {
    "compiled_with": "packc-dev",
    "created_at": "2026-11-06T18:08:46Z",
    "schema": "v1"
  },
  "description": "Pack containing 1 prompts",
  "id": "greeting",
  "name": "greeting",
  "prompts": {
    "greeting": {
      "id": "greeting",
      "name": "Greeting Assistant",
      "version": "v1.0.0",
      "system_template": "You are a friendly assistant...",
      "variables": [ ... ],
      "pipeline": { ... }
    }
  },
  "template_engine": {
    "syntax": "{{variable}}",
    "version": "v1"
  },
  "version": "v1.0.0"
}
```

On success the command prints a summary. The `tools`, `evals`, `workflow` and `agents` lines appear only when the pack has them:

```text
✓ Pack compiled successfully: packs/customer-support.pack.json
  Contains 2 prompts: [support escalation]
  Contains 1 tools: [search_kb]
```

## Validation

Before writing, `compile` runs the same checks as [`packc validate`](/packc/reference/validate/), plus composition checks, against the compiled pack:

| Stage | Checks |
|---|---|
| `pack_id` | the pack ID |
| `load` | the Arena config and every file it references, against their schemas |
| `compile` | prompt parsing, missing media files, metadata promotion |
| `skills` | skill sources and paths |
| `workflow` | the workflow state machine |
| `compositions` | composition step graphs and their references |
| `agents` | the `agents` block |
| `governance` | governance references |
| `pack` | pack structure: required fields, semantic versions, a `system_template` that uses `{{` with no declared variables |
| `pack_schema` | the compiled pack against the PromptPack schema |

An error at any stage stops the compile and writes nothing. Errors print to stderr as `Compilation failed:` followed by `[<stage>] <message>`, or by `<n> problems:` and one `  - [<stage>] <message>` line per error when there are several. Warnings print to stdout as `⚠ [<stage>] <message>` and do not stop the compile.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Pack compiled and written. |
| `1` | Compilation failed, or the pack file could not be written. |
| `2` | Unknown flag. |

## Examples

Compile with every default, from the directory that holds `config.arena.yaml`:

```bash
packc compile
```

Name the config, the output and the ID:

```bash
packc compile -c config/arena.yaml -o packs/customer-support.pack.json --id customer-support
```

## Related

- [compile-prompt](/packc/reference/compile-prompt/): compile one prompt file
- [validate](/packc/reference/validate/): validate a pack file
- [inspect](/packc/reference/inspect/): print a pack's contents
- [Compile packs](/packc/how-to/compile-packs/)
