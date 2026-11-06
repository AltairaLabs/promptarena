---
title: Validate Packs
description: Check a compiled pack against the PromptPack schema and packc's structural rules, in a script, a pre-commit hook or CI, and fix the common failures.
sidebar:
  order: 3
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - arena/cmd/promptarena/export.go
    - arena/cmd/promptarena/validate_json.go
    - packc/compiler/check.go
    - packc/main_interactive.go
    - packc/skills_validator.go
---
Check that a `.pack.json` file is valid before you ship it.

## Prerequisites

- `packc` installed ([Install PackC](/packc/how-to/install/)).
- A pack file. `packc compile` runs the same checks before it writes one, so `packc validate` matters most for packs built elsewhere or edited since, and for `packc compile-prompt` output, which is not checked when it is written.

## Validate a pack

1. Run `packc validate` on the file:

   ```bash
   packc validate packs/app.pack.json
   ```

2. Read the result. A valid pack prints:

   ```text
   Validating pack: packs/app.pack.json
   Validating against PromptPack schema...
   ✓ Schema validation passed
   ✓ Pack structure is valid
   ```

   Non-blocking findings follow with a `⚠` prefix, and the exit code stays 0:

   ```text
   ✓ Pack structure is valid
   ⚠ Workflow warnings:
     - workflow.states["intake"].on_event["escalate"]: event name should be PascalCase
   ```

   Blocking findings are printed to stderr under `✗ Pack failed schema validation:`, `✗ Pack validation failed:` or `✗ Skill validation errors:`, and the exit code is 1.

[Validation](/packc/explanation/validation/) lists every check and whether it blocks.

## Validate several packs

`packc validate` checks one file per run and ignores any further arguments, so `packc validate packs/*.pack.json` checks only the first match. Loop over the files instead:

```bash
#!/bin/bash
status=0
for pack in packs/*.pack.json; do
  packc validate "$pack" || status=1
done
exit $status
```

This checks every pack and fails if any of them fail.

## Validate before each commit

To block a commit that stages an invalid pack, save this as `.git/hooks/pre-commit` and make it executable with `chmod +x .git/hooks/pre-commit`:

```bash
#!/bin/bash
for pack in $(git diff --cached --name-only --diff-filter=ACM | grep '\.pack\.json$'); do
  if ! packc validate "$pack"; then
    echo "✗ $pack is invalid. Fix it, or commit with --no-verify to skip this check."
    exit 1
  fi
done
```

## Validate in CI

Compile and validate in the same job. A failing `packc` command exits 1, which fails the step:

```yaml
# .github/workflows/validate-packs.yml
name: Validate Packs

on:
  pull_request:
    paths:
      - 'prompts/**'
      - 'config.arena.yaml'

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go install github.com/AltairaLabs/promptarena/v2/packc@latest
      - run: packc compile --config config.arena.yaml --output packs/app.pack.json --id app
      - run: packc validate packs/app.pack.json
```

[CI/CD integration](/packc/how-to/ci-cd-integration/) covers other CI systems and the `packc-action` GitHub Action.

## Check the source config instead

`promptarena validate` runs the pack checks on an arena config without writing a pack, and adds PromptArena's own checks:

```bash
promptarena validate config.arena.yaml
```

Use it while editing prompts. Use `packc validate` on the compiled artifact.

## Verify

```bash
packc validate packs/app.pack.json
echo $?
```

`0` means the pack is valid. `1` means it is not, or the file could not be read.

## Troubleshooting

### The file cannot be read

```text
Validating pack: packs/app.pack.json
Error reading pack file: open packs/app.pack.json: no such file or directory
```

Check the path, or compile the pack first.

### Schema validation fails

```text
✗ Pack failed schema validation:
  - (root): id is required (value: map[])
  - prompts.greeting.version: Does not match pattern '^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)...' (value: )
```

Each line gives the JSON path of the problem. Fix the source config and recompile rather than editing the pack. A missing prompt `version` is the usual cause in `packc compile-prompt` output: add `spec.version` to the prompt config.

A file that is not valid JSON can also fail here. For example, a stray closing brace after the pack (`{}}`) is reported as schema errors about the leading `{}`, not as a parse error.

### Structural validation fails

```text
✗ Pack validation failed:
  - missing compilation metadata
```

A pack that was not written by `packc` may lack the `compilation` block. The schema allows that, but `packc validate` does not. Recompile it with `packc compile`.

### A skill path does not exist

```text
✗ Skill validation errors:
  - skills[0]: directory "skills/billing" does not exist
```

`packc validate` resolves skill paths relative to the pack file's directory. Validate the pack where it sits next to its `skills/` directory, or move the directory.

## Related

- [Validation](/packc/explanation/validation/)
- [validate reference](/packc/reference/validate/)
- [inspect reference](/packc/reference/inspect/)
- [CI/CD integration](/packc/how-to/ci-cd-integration/)
