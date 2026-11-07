---
title: validate
description: Checks, output and exit codes of packc validate, which validates a compiled pack file against the PromptPack schema and its own structure rules.
sidebar:
  order: 3
verified:
  commit: 95b220864e8df93d5a36b38ad288897bf70a4c5b
  sources:
    - packc/compiler/check.go
    - packc/compiler/pack_fields.go
    - packc/compiler/pack_file.go
    - packc/main_interactive.go
---
`packc validate` checks compiled `.pack.json` files against the [PromptPack](https://promptpack.org) schema, then against the structural rules the schema cannot express. It runs the same pack checks [`packc compile`](/packc/reference/compile/) runs before it writes a pack, so a finding is an error or a warning in both alike. It does not change the files.

## Synopsis

```bash
packc validate <pack-file> [<pack-file>...]
```

## Arguments

| Argument | Description |
|---|---|
| `<pack-file>` | Path to a pack JSON file. At least one is required. Each one is validated, even after one fails. |
| `-h`, `--help` | Print usage and exit 0. |

## Environment

| Variable | Default | Description |
|---|---|---|
| `PROMPTKIT_SCHEMA_SOURCE` | unset | Where the PromptPack schema comes from. Unset or `local`: the schema embedded in `packc`, so no network access is needed. `remote`: the URL in the pack's `$schema`, or `https://promptpack.org/schema/latest/promptpack.schema.json` when the pack has none. A value starting with `/` or `./`: that file. Any other value: that URL. |

## Checks

For each file, the checks run in this order. Every finding carries the stage that found it.

| Order | Stage | Check |
|---|---|---|
| 1 | `load` | The file can be read, and parses as JSON. A failure here ends the checks for that file. |
| 2 | `pack_schema` | The JSON validates against the PromptPack schema: required fields, types, ID and semantic-version patterns, parameter ranges such as `temperature` 0 to 2. |
| 3 | `skills` | Skill sources and paths, resolved relative to the pack file's directory. |
| 4 | `workflow`, `compositions`, `agents`, `governance`, `pack` | The pack checks below. |

Errors:

| Area | Errors |
|---|---|
| Pack | `id` missing; `version` missing or not a semantic version; no prompts; `template_engine` missing, or its `version` or `syntax` unset |
| Each prompt | `system_template` missing; `system_template` contains `{{` but `variables` is empty; `version` missing or not a semantic version |
| `workflow` | `version` not 1 or 2; `states` empty; `entry` not a state; a state's `prompt_task` not a prompt (except `orchestration: composition` states); an `on_event` or `on_max_visits` target not a state; `persistence` not `transient` or `persistent`; `orchestration` not `internal`, `external`, `hybrid` or `composition`; `control` not `user` or `agent`; `composition` set without `orchestration: composition`, or missing with it |
| `compositions` | a step graph that is not valid; a step referencing a prompt, tool or eval the pack does not define; a workflow state naming a composition that does not exist |
| `agents` | `members` empty; `entry` not a member; a member key not a prompt; a member's `state` set without a workflow, or not a workflow state |
| Skills | a missing directory, a path outside the pack's directory, an unparseable `SKILL.md`, a duplicate name, an incomplete inline skill |
| Governance | an obligation or review naming an eval, validator, field or obligation the pack does not declare |

Warnings print and the pack still passes:

| Area | Warnings |
|---|---|
| Pack | no `compilation` block (the schema makes it optional); a prompt `tools` entry the pack's `tools` does not define |
| `workflow` | an event name that is not PascalCase; a cycle; a `control: agent` state that is terminal, or in a cycle with nothing that yields, ends or caps visits; a terminal state with `on_event`; an `on_max_visits` target that has its own `max_visits`; in version 2, a non-terminal state with no `on_event` and no `max_visits` |
| `compositions` | a composition no workflow state uses |
| `agents` | an `input_modes` or `output_modes` entry without a `/`; an empty tag |
| Skills | a skill's `allowed-tools` naming a tool the pack does not define |
| Governance | an undeclared vocabulary prefix |

## Output

A valid pack:

```text
Validating pack: packs/greeting.pack.json
✓ Schema validation passed
✓ Pack structure is valid
```

Warnings print as `⚠ [<stage>] <message>` before the last line:

```text
Validating pack: packs/support.pack.json
✓ Schema validation passed
⚠ [workflow] workflow contains a cycle: billing -> triage
✓ Pack structure is valid
```

A failing pack prints every error, one per line, under a header naming the file:

```text
Validating pack: x.pack.json
✗ Pack validation failed: x.pack.json
  - [load] invalid JSON: invalid character '}' after top-level value
```

With more than one file, each file's result is separated by a blank line, and a final line counts the valid packs:

```text
1 of 2 packs valid
```

Failures go to stderr; everything else goes to stdout.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Every pack is valid, or `--help` was given. Warnings do not change the code. |
| `1` | No pack file argument, or any pack failed a check. |

## Examples

Validate one pack:

```bash
packc validate packs/greeting.pack.json
```

Validate every pack in a directory and fail if any is invalid:

```bash
packc validate packs/*.pack.json
```

## Related

- [compile](/packc/reference/compile/): runs these checks before writing a pack
- [inspect](/packc/reference/inspect/): print a pack's contents
- [Validate packs](/packc/how-to/validate-packs/)
- [Validation](/packc/explanation/validation/)
