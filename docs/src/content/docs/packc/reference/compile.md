---
title: compile
description: Flags, input, output and exit codes of packc compile, which compiles every prompt an Arena config lists into one PromptPack file.
sidebar:
  order: 1
verified:
  commit: 66ec692303a313b1f7ece7ce55dd5f73859efe29
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/skill_sources.go
    - arena/arenaconfig/types.go
    - packc/compiler/check.go
    - packc/compiler/compiler.go
    - packc/compiler/helpers_integration.go
    - packc/compiler/skills_validator_integration.go
    - packc/main.go
    - packc/main_interactive.go
    - schemas/v1alpha1/arena.json
---
`packc compile` loads an Arena config, compiles every prompt config it lists into one [PromptPack](https://promptpack.org) JSON file, validates the result, and writes it. It is the command for building a pack from a project.

## Synopsis

```bash
packc compile [-c <arena.yaml>] [-o <pack-file>] [--id <pack-id>] [--pack-version <semver>]
```

## Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `-c`, `--config` | string | `config.arena.yaml` | Path to the Arena config file. |
| `-o`, `--output` | string | `<id>.pack.json` | Path of the pack file to write. Missing parent directories are created. |
| `--id` | string | current directory name, sanitized | Pack ID, written to the pack's `id` and `name` fields. |
| `--pack-version` | string | `v1.0.0` | Pack version, written to the pack's `version` field. A semantic version, with an optional `v` prefix: `1.2.0`, `v1.2.0`, `2.0.0-rc.1`. |

An explicit `--id` must match `^[a-z][a-z0-9-]*$`, the PromptPack pattern for a pack ID. Any other value fails the compile before the config is loaded:

```text
Compilation failed: [pack_id] invalid pack ID "Assistant_1": must start with a lowercase letter and contain only lowercase letters, digits and dashes (pattern ^[a-z][a-z0-9-]*$)
```

A `--pack-version` that is not a semantic version also fails before the config is loaded:

```text
Compilation failed: [pack_version] invalid pack version "latest": must be a semantic version such as 1.2.0 or v1.2.0
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
| `skills` | `skills`. Directory paths are made relative to the config file's directory, whether `-c` is relative (including the default) or absolute. An `@org/name` path installed by `promptarena skill install` stays a reference in the pack and resolves to its install directory |
| `pack_metadata` | `metadata` |

The pack's `template_engine` is the `spec.template_engine` the prompts set. Prompts that set different ones fail the `compile` stage, since a pack has one engine; prompts that set none, or set the default, are not compared. With none set, the pack gets the default `v1` engine. Without `pack_metadata`, the pack's `metadata` is taken from the prompt whose `prompt_configs` ID sorts first among those that set `spec.metadata`, and the compile prints a warning naming it. See [Configuration Schema](/arena/reference/config-schema/) for every Arena field.

## Output

The pack's top-level `version` is `--pack-version`, or `v1.0.0` without it, and its `description` is `Pack containing <n> prompts`. Each prompt keeps its own `version`. `compilation.compiled_with` is `packc-<version>`, where `<version>` is what [`packc version`](/packc/reference/version/) prints.

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
    "features": [
      "basic_substitution"
    ],
    "syntax": "{{variable}}",
    "version": "v1"
  },
  "version": "v1.0.0"
}
```

On success the command prints a summary, with prompt and tool names sorted. The `tools`, `evals`, `workflow` and `agents` lines appear only when the pack has them:

```text
✓ Pack compiled successfully: packs/customer-support.pack.json
  Contains 2 prompts: [escalation support]
  Contains 1 tools: [search_kb]
```

## Validation

Before writing, `compile` runs the same checks as [`packc validate`](/packc/reference/validate/) against the compiled pack:

| Stage | Checks |
|---|---|
| `pack_id` | the pack ID |
| `pack_version` | the `--pack-version` value |
| `load` | the Arena config and every file it references, against their schemas |
| `compile` | prompt parsing, missing media files, metadata promotion, prompts that declare different template engines, a `required: true` fragment that does not resolve |
| `skills` | skill sources and paths |
| `workflow` | the workflow state machine |
| `compositions` | composition step graphs and their references |
| `agents` | the `agents` block |
| `governance` | governance references |
| `pack` | pack structure: required fields, semantic versions, a `system_template` that uses `{{` with no declared variables, eval definitions; a prompt tool missing from the pack's `tools` and an unknown eval trigger are warnings |
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

Name the config, the output, the ID and the version:

```bash
packc compile -c config/arena.yaml -o packs/customer-support.pack.json --id customer-support --pack-version 1.2.0
```

## Related

- [compile-prompt](/packc/reference/compile-prompt/): compile one prompt file
- [validate](/packc/reference/validate/): validate a pack file
- [inspect](/packc/reference/inspect/): print a pack's contents
- [Compile packs](/packc/how-to/compile-packs/)
