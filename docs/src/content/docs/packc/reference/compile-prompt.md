---
title: compile-prompt
description: Flags, output and exit codes of packc compile-prompt, which compiles one PromptConfig file into a single-prompt pack.
sidebar:
  order: 2
verified:
  commit: 72f35dcda537f9ebff0eaf33808ce728f0c8b3f8
  sources:
    - packc/compiler/check.go
    - packc/compiler/helpers_integration.go
    - packc/compiler/pack_file.go
    - packc/compiler/pack_file_test.go
    - packc/main.go
    - packc/main_interactive.go
---
`packc compile-prompt` compiles one `kind: PromptConfig` YAML file into a pack that holds only that prompt. It reads no Arena config, so the pack carries no tools, workflow, agents or pack-level evals.

## Synopsis

```bash
packc compile-prompt -p <prompt.yaml> -o <pack-file> [--pack-version <semver>]
```

## Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `-p`, `--prompt` | string | none (required) | Path to the PromptConfig YAML file. |
| `-o`, `--output` | string | none (required) | Path of the pack file to write. Missing parent directories are created. |
| `--pack-version` | string | the prompt's `spec.version` | Pack version, written to the pack's `version` field. A semantic version, with an optional `v` prefix. |

## Input

A PromptConfig file:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: Customer Support Agent
spec:
  task_type: customer_support
  description: Handles customer inquiries
  version: v1.0.0
  system_template: |
    You are a helpful customer support agent.
    Customer: {{customer_name}}
  template_engine:
    version: v1
    syntax: "{{variable}}"
  variables:
    - name: customer_name
      type: string
      required: true
  allowed_tools:
    - search_knowledge_base
```

## Output

The pack's top-level fields come from the prompt config:

| Pack field | Source |
|---|---|
| `id` | `spec.task_type`, with `_` replaced by `-` |
| `name` | `metadata.name` |
| `version` | `--pack-version`, or `spec.version` |
| `description` | `spec.description` |
| `template_engine` | `spec.template_engine` |
| `metadata` | `spec.metadata` |
| `prompts.<task_type>` | the compiled prompt; `allowed_tools` becomes `tools` |
| `compilation.compiled_with` | `packc-<version>`, where `<version>` is what [`packc version`](/packc/reference/version/) prints |

`$schema` is `https://promptpack.org/schema/latest/promptpack.schema.json`, as in a `packc compile` pack. For the example above the command prints a warning, because the pack carries no tools for `allowed_tools` to name:

```text
Compiling prompt prompts/support.yaml to pack...
⚠ [pack] prompt 'customer_support': tool 'search_knowledge_base' is not defined in the pack's tools
✓ Pack compiled successfully: packs/support.pack.json
```

## Validation

`compile-prompt` validates the prompt config against the PromptConfig schema before compiling, then runs the same checks on the pack as [`packc compile`](/packc/reference/compile/): the pack checks of [`packc validate`](/packc/reference/validate/) and the PromptPack schema. A pack it writes passes `packc validate`. When `spec.media.enabled` is true, media files referenced under `spec.media.examples` are resolved relative to the prompt file's directory, and a missing one prints a warning:

```text
⚠ [compile] media(customer_support): Media file not found: images/logo.png (example: product-photo, part 0)
```

Errors print to stderr, and no file is written:

| Message | Cause |
|---|---|
| `Error: --prompt and --output are required`, then the usage and flags | A flag is missing. |
| `Compilation failed: [load] reading prompt file: ...` | The prompt file cannot be read. |
| `Compilation failed: [load] prompt config failed schema validation: ...` | The prompt config does not match its schema, for example `spec.version: "1.0"`. |
| `Compilation failed: [load] parsing prompt config: ...` | The file is not a valid PromptConfig. |
| `Compilation failed: [pack_version] ...` | `--pack-version` is not a semantic version. |
| `Compilation failed: [<stage>] ...` | A pack check failed, with the stage that found it. |
| `Failed to write pack file: ...` | The output path cannot be written. |

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Pack written. |
| `1` | A required flag is missing, the prompt could not be read, validated or compiled, or the pack could not be written. |
| `2` | A flag is unknown. |

## Examples

Compile one prompt:

```bash
packc compile-prompt -p prompts/support.yaml -o packs/support.pack.json
```

Compile every prompt in a directory into its own pack:

```bash
for prompt in prompts/*.yaml; do
  packc compile-prompt -p "$prompt" -o "packs/$(basename "$prompt" .yaml).pack.json"
done
```

## Related

- [compile](/packc/reference/compile/): compile every prompt in an Arena config into one pack
- [validate](/packc/reference/validate/): validate a pack file
- [Pack format](/packc/explanation/pack-format/)
