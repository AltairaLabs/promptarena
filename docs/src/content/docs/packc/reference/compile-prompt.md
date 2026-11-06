---
title: compile-prompt
description: Flags, output and exit codes of packc compile-prompt, which compiles one PromptConfig file into a single-prompt pack.
sidebar:
  order: 2
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - packc/main.go
    - packc/main_interactive.go
---
`packc compile-prompt` compiles one `kind: PromptConfig` YAML file into a pack that holds only that prompt. It reads no Arena config, so the pack carries no tools, workflow, agents or pack-level evals.

## Synopsis

```bash
packc compile-prompt -p <prompt.yaml> -o <pack-file>
```

## Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `-p`, `--prompt` | string | none (required) | Path to the PromptConfig YAML file. |
| `-o`, `--output` | string | none (required) | Path of the pack file to write. The parent directory must exist. |

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
| `version` | `spec.version` |
| `description` | `spec.description` |
| `template_engine` | `spec.template_engine` |
| `metadata` | `spec.metadata` |
| `prompts.<task_type>` | the compiled prompt; `allowed_tools` becomes `tools` |
| `compilation.compiled_with` | `packc-<version>`, where `<version>` is what [`packc version`](/packc/reference/version/) prints |

The pack has no `$schema` field. For the example above the command prints:

```text
Compiling prompt 'customer_support' to pack...
✓ Pack compiled successfully: packs/support.pack.json
```

## Validation

`compile-prompt` checks only that the file parses as a PromptConfig. It does not validate the prompt config or the pack against their schemas, and it does not run the structural checks of [`packc validate`](/packc/reference/validate/): a pack it writes can fail `packc validate`. When `spec.media.enabled` is true, media files referenced under `spec.media.examples` are resolved relative to the prompt file's directory, and a missing one prints a warning:

```text
⚠ Media validation warnings:
  - Media file not found: images/logo.png (example: product-photo, part 0)
```

Errors print to stderr:

| Message | Cause |
|---|---|
| `Error: --prompt and --output are required` | A flag is missing. |
| `Error reading prompt file: ...` | The prompt file cannot be read. |
| `Error parsing prompt config: ...` | The file is not valid YAML, or misses a required field such as `apiVersion`. |
| `Compilation failed: failed to write pack file: ...` | The output path cannot be written, for example because its directory does not exist. |

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Pack written. |
| `1` | The prompt file could not be read, parsed or compiled, or the pack could not be written. |
| `2` | A required flag is missing, or a flag is unknown. |

## Examples

Compile one prompt and validate the result:

```bash
mkdir -p packs
packc compile-prompt -p prompts/support.yaml -o packs/support.pack.json
packc validate packs/support.pack.json
```

Compile every prompt in a directory into its own pack:

```bash
mkdir -p packs
for prompt in prompts/*.yaml; do
  packc compile-prompt -p "$prompt" -o "packs/$(basename "$prompt" .yaml).pack.json"
done
```

## Related

- [compile](/packc/reference/compile/): compile every prompt in an Arena config into one pack
- [validate](/packc/reference/validate/): validate a pack file
- [Pack format](/packc/explanation/pack-format/)
