---
id: schema-validation
title: Validate against the binary's own schemas
summary: Run `promptarena schema <type>` for authoritative structure and `promptarena validate` to check configs.
tags: [schema, validation]
---
Every config type (scenario, provider, prompt, tool, arena) has a JSON schema. The
schema embedded in your installed `promptarena` binary is the source of truth — it is
the exact version `promptarena validate` enforces. Prefer it over the public web copy,
which may be a different release.

- `promptarena schema <type>` — print the authoritative schema for a type.
- `promptarena validate` — check your configs before running.

Author configs to the schema first; don't guess field names.

On an arena config, `promptarena validate` goes beyond the schema. It runs the arena's
consistency checks, then builds the pack in memory and runs every check `packc` runs:
workflow, compositions, agents, skills, pack structure, and the compiled pack against
the PromptPack spec. If `validate` passes, `packc compile` will too. A config with no
`prompt_configs` builds no pack, so those checks are skipped and the report says so.

Use `promptarena validate --json <config>` and fix everything it reports in one pass.
Each entry in `errors` has a `stage` (which check found it), a `field` (where), a
`source` (the file to edit, when that isn't the config itself), and a `description`.
`stages` lists the checks that ran, so a missing stage means it was never checked, not
that it passed.
