---
title: Validate Packs
description: Check a compiled pack against the PromptPack schema and packc's structural rules, in a script, a pre-commit hook or CI, and fix the common failures.
sidebar:
  order: 3
verified:
  commit: 66ec692303a313b1f7ece7ce55dd5f73859efe29
  sources:
    - arena/cmd/promptarena/validate_json.go
    - packc/compiler/check.go
    - packc/compiler/pack_file.go
    - packc/compiler/skills_validator_integration.go
    - packc/main_interactive.go
---
Check that a `.pack.json` file is valid before you ship it.

## Prerequisites

- `packc` installed ([Install PackC](/packc/how-to/install/)).
- A pack file. `packc compile` and `packc compile-prompt` run the same checks before they write one, so `packc validate` matters most for packs built elsewhere or edited since.

## Validate a pack

1. Run `packc validate` on the file:

   ```bash
   packc validate packs/app.pack.json
   ```

2. Read the result. A valid pack prints:

   ```text
   Validating pack: packs/app.pack.json
   ✓ Schema validation passed
   ✓ Pack structure is valid
   ```

   Non-blocking findings print with a `⚠` prefix and their stage, and the exit code stays 0:

   ```text
   ✓ Schema validation passed
   ⚠ [workflow] workflow.states["intake"].on_event["escalate"]: event name should be PascalCase
   ✓ Pack structure is valid
   ```

   Blocking findings are printed to stderr under `✗ Pack validation failed: <file>`, one per line with its stage, and the exit code is 1.

[Validation](/packc/explanation/validation/) lists every check and whether it blocks.

## Validate several packs

Pass every file. `packc validate` checks each one, even after one fails:

```bash
packc validate packs/*.pack.json
```

```text
Validating pack: packs/app.pack.json
✓ Schema validation passed
✓ Pack structure is valid

Validating pack: packs/support.pack.json
✓ Schema validation passed
✓ Pack structure is valid

2 of 2 packs valid
```

It exits 1 if any pack fails.

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

`promptarena validate` runs the pack checks on an arena config without writing a pack, and adds PromptArena's own checks, including assertion types and params:

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
✗ Pack validation failed: packs/app.pack.json
  - [load] reading pack file: open packs/app.pack.json: no such file or directory
```

Check the path, or compile the pack first.

### Schema validation fails

```text
✗ Pack validation failed: packs/app.pack.json
  - [pack_schema] prompts.greeting.version: Does not match pattern '^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)...' (value: 1.0)
  - [pack] prompt 'greeting': invalid version '1.0': invalid semantic version: invalid semantic version
```

A `[pack_schema]` line gives the JSON path of the problem, and the structural check can report the same problem again under `[pack]`. Fix the source config and recompile rather than editing the pack.

### The file is not valid JSON

```text
✗ Pack validation failed: packs/app.pack.json
  - [load] invalid JSON: invalid character '}' after top-level value
```

`packc validate` parses the file before any other check, so a syntax error such as a stray closing brace is reported as one. Recompile the pack, or fix the JSON.

### A skill path does not exist

```text
✗ Pack validation failed: packs/app.pack.json
  - [skills] skills[0]: directory "skills/billing" does not exist
```

`packc validate` resolves skill paths relative to the pack file's directory, and a path must stay inside that directory. Validate the pack where it sits next to its `skills/` directory, or move the directory.

An `@org/name` skill reference resolves to the directory `promptarena skill install` used, outside the pack directory. If that skill is not installed, `packc validate` reports it:

```text
  - [skills] skills[0]: skill acme/billing is not installed (install it with: promptarena skill install @acme/billing)
```

## Related

- [Validation](/packc/explanation/validation/)
- [validate reference](/packc/reference/validate/)
- [inspect reference](/packc/reference/inspect/)
- [CI/CD integration](/packc/how-to/ci-cd-integration/)
