---
title: inspect
description: Output sections and exit codes of packc inspect, which prints a readable summary of a compiled pack file.
sidebar:
  order: 4
verified:
  commit: 95b220864e8df93d5a36b38ad288897bf70a4c5b
  sources:
    - packc/main.go
    - packc/main_interactive.go
---
`packc inspect` prints a readable summary of a compiled pack file: its identity, prompts, tools, fragments, evals, skills, workflow, compositions, agents, metadata and compilation record. It does not validate the pack; use [`packc validate`](/packc/reference/validate/) for that.

## Synopsis

```bash
packc inspect <pack-file>
```

## Arguments

| Argument | Description |
|---|---|
| `<pack-file>` | Path to the pack JSON file. Required, and only one. |
| `-h`, `--help` | Print usage and exit 0. |

## Output sections

Sections print in this order. A section, or a line inside one, is omitted when the pack has no value for it.

| Section | Lines |
|---|---|
| `=== Pack Information ===` | `Pack:` (the pack's `name`), `ID:`, `Version:`, `Description:` |
| `Template Engine:` | `<version> (<syntax>)`, then `Features:` |
| `=== Prompts (<n>) ===` | one `[<task_type>]` block per prompt, sorted by task type, with `Name:`, `Description:`, `Version:`, `Variables: <n> required, <n> optional`, `Required:`, `Tools (<n>):`, `Validators (<n>):` (each as `<type> (enabled)` or `<type> (disabled)`), and `Tested Models (<n>):` (each as `<provider>/<model>: <rate>% success, avg <n> tokens`) |
| `=== Tools (<n>) ===` | one line per pack-level tool, sorted: `<name>: <description>` |
| `Shared Fragments (<n>):` | fragment names, sorted |
| `=== Evals (<n>) ===` | one line per pack-level eval: `<id> (type: <type>, trigger: <trigger>)` |
| `=== Skills (<n>) ===` | one line per skill source: `path: <dir>` or `inline: <name>` |
| `=== Workflow (v<n>) ===` | `Entry:`, `States (<n>):`, then per state, sorted: `[<name>] prompt_task=<task>`, `Description:`, one `on <Event> → <target>` line per event, `Persistence:`, `Orchestration:` |
| `=== Compositions (<n>) ===` | composition names, sorted |
| `=== Agents ===` | `Entry:`, `Members (<n>):`, then per member, sorted: `[<name>]`, `Description:`, `Tags:`, `Input modes:`, `Output modes:` |
| `Pack Metadata:` | `Domain:`, `Language:`, `Tags:` |
| `Compilation:` | `<compiled_with>, <created_at>, Schema: <schema>` |

The `Pack Information` and `Prompts` headings always print. A file with neither an `id` nor any prompts is rejected as not a pack.

## Example

```bash
packc inspect packs/greeting.pack.json
```

```text

=== Pack Information ===
Pack: greeting
ID: greeting
Version: v1.0.0
Description: Pack containing 1 prompts

Template Engine: v1 ({{variable}})

=== Prompts (1) ===

[greeting]
  Name: Greeting Assistant
  Description: A friendly assistant that greets users
  Version: v1.0.0
  Variables: 1 required, 1 optional
    Required: [user_name]

Compilation: packc-dev, 2026-11-06T18:08:46Z, Schema: v1

```

## Exit codes

| Code | Meaning |
|---|---|
| `0` | The file parsed as a pack and its summary printed. |
| `0` | `--help` was given. |
| `1` | No pack file argument (stderr: `Error: pack file path required`), more than one (`Error: inspect takes one pack file`), the file cannot be read or is not valid pack JSON (`Error loading pack: ...`), or it has no `id` and no prompts (`Error: <file> is not a PromptPack (no id and no prompts)`). |

## Related

- [validate](/packc/reference/validate/): validate a pack file
- [compile](/packc/reference/compile/): compile a pack
- [Pack format](/packc/explanation/pack-format/)
