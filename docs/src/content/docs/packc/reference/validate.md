---
title: validate
description: Checks, output and exit codes of packc validate, which validates a compiled pack file against the PromptPack schema and its own structure rules.
sidebar:
  order: 3
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - packc/compiler/check.go
    - packc/main_interactive.go
    - packc/schema_validator.go
---
`packc validate` checks a compiled `.pack.json` file against the [PromptPack](https://promptpack.org) schema, then against the structural rules the schema cannot express. It does not change the file.

## Synopsis

```bash
packc validate <pack-file>
```

## Arguments

| Argument | Description |
|---|---|
| `<pack-file>` | Path to the pack JSON file. Required. Arguments after it are ignored. |

`validate` has no flags.

## Environment

| Variable | Default | Description |
|---|---|---|
| `PROMPTKIT_SCHEMA_SOURCE` | unset | Where the PromptPack schema comes from. Unset or `local`: the schema embedded in `packc`, so no network access is needed. `remote`: the URL in the pack's `$schema`, or `https://promptpack.org/schema/latest/promptpack.schema.json` when the pack has none. A value starting with `/` or `./`: that file. Any other value: that URL. |

## Checks

The checks run in this order. The first group that finds an error ends the run with exit code `1`.

| Order | Check | On failure |
|---|---|---|
| 1 | The file can be read. | `Error reading pack file: ...` |
| 2 | The JSON validates against the PromptPack schema: required fields, types, ID and semantic-version patterns, parameter ranges such as `temperature` 0 to 2. | `✗ Pack failed schema validation:` and one line per violation |
| 3 | The JSON parses as a pack. | `Error loading pack: ...` |
| 4 | Pack structure, workflow, agents and governance (below). | `✗ Pack validation failed:` and every finding from this step, warnings included |
| 5 | Skills: sources and paths, resolved relative to the pack file's directory. | `✗ Skill validation errors:` and one line per error |

If the schema cannot be applied at all, for example because the file is not valid JSON, step 2 prints `⚠ Schema validation could not be performed: ...` to stderr and the run continues to step 3.

Step 4 checks:

| Area | Errors |
|---|---|
| Pack | `id` missing; `version` missing or not a semantic version; no prompts; `template_engine` missing, or its `version` or `syntax` unset; `compilation` missing |
| Each prompt | `system_template` missing; `system_template` contains `{{` but `variables` is empty; `version` missing or not a semantic version |
| `workflow` | `version` not 1 or 2; `states` empty; `entry` not a state; a state's `prompt_task` not a prompt (except `orchestration: composition` states); an `on_event` or `on_max_visits` target not a state; `persistence` not `transient` or `persistent`; `orchestration` not `internal`, `external`, `hybrid` or `composition`; `control` not `user` or `agent`; `composition` set without `orchestration: composition`, or missing with it |
| `agents` | `members` empty; `entry` not a member; a member key not a prompt; a member's `state` set without a workflow, or not a workflow state; an `input_modes` or `output_modes` entry without a `/`; an empty tag |
| Governance | an obligation or review naming an eval, validator, field or obligation the pack does not declare |

Some findings are warnings: they print and the pack still passes.

| Area | Warnings |
|---|---|
| `workflow` | an event name that is not PascalCase; a cycle; a `control: agent` state that is terminal, or in a cycle with nothing that yields, ends or caps visits; a terminal state with `on_event`; an `on_max_visits` target that has its own `max_visits`; in version 2, a non-terminal state with no `on_event` and no `max_visits` |
| Skills | non-fatal skill findings |
| Governance | an undeclared vocabulary prefix |

`validate` does not check that a prompt's `tools` are defined in the pack's `tools`.

## Output

A valid pack:

```text
Validating pack: packs/greeting.pack.json
Validating against PromptPack schema...
✓ Schema validation passed
✓ Pack structure is valid
```

A valid pack with warnings prints skill warnings as `⚠ <message>`, governance warnings under `⚠ Governance warnings:`, and workflow warnings after the success line:

```text
✓ Pack structure is valid
⚠ Workflow warnings:
  - workflow contains a cycle: billing -> triage
```

A schema failure:

```text
Validating pack: empty.json
Validating against PromptPack schema...
✗ Pack failed schema validation:
  - (root): id is required (value: map[])
  - (root): name is required (value: map[])
  - (root): version is required (value: map[])
  - (root): template_engine is required (value: map[])
  - (root): prompts is required (value: map[])
```

Failures and the schema warning go to stderr; everything else goes to stdout.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | The pack is valid. Warnings do not change the code. |
| `1` | No pack file argument, or a check failed. |

## Examples

Validate one pack:

```bash
packc validate packs/greeting.pack.json
```

Validate every pack in a directory and fail if any is invalid:

```bash
status=0
for pack in packs/*.pack.json; do
  packc validate "$pack" || status=1
done
exit $status
```

## Related

- [compile](/packc/reference/compile/): runs these checks before writing a pack
- [inspect](/packc/reference/inspect/): print a pack's contents
- [Validate packs](/packc/how-to/validate-packs/)
- [Validation](/packc/explanation/validation/)
