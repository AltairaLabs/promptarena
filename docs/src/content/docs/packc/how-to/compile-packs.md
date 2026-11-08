---
title: Compile Packs
description: Compile an arena config, or a single prompt file, into a .pack.json file with packc, and fix the common compile failures.
sidebar:
  order: 2
verified:
  commit: 72f35dcda537f9ebff0eaf33808ce728f0c8b3f8
  sources:
    - arena/arenaconfig/types.go
    - packc/compiler/check.go
    - packc/compiler/compiler.go
    - packc/compiler/helpers_integration.go
    - packc/compiler/pack_file.go
    - packc/main.go
    - packc/main_interactive.go
    - schemas/v1alpha1/arena.json
---
Compile your prompts into a `.pack.json` file that a PromptPack runtime can load.

## Prerequisites

- `packc` installed ([Install PackC](/packc/how-to/install/)).
- An arena config with `prompt_configs`, or a single prompt config file.

## Compile an arena config

`packc compile` compiles every prompt the arena config lists into one pack, with the config's tools, workflow, agents and skills.

1. Check the arena config. It is the same file `promptarena run` uses, so it needs `providers` and `defaults` as well as `prompt_configs`:

   ```yaml
   # config.arena.yaml
   apiVersion: promptkit.altairalabs.ai/v1alpha1
   kind: Arena
   metadata:
     name: customer-service
   spec:
     prompt_configs:
       - id: support
         file: prompts/support.yaml
       - id: sales
         file: prompts/sales.yaml
     tools:
       - file: tools/lookup-order.tool.yaml
     providers:
       - file: providers/openai.provider.yaml
     defaults:
       temperature: 0.7
   ```

   Paths are relative to the arena config's directory.

2. Compile it:

   ```bash
   packc compile \
     --config config.arena.yaml \
     --output packs/customer-service.pack.json \
     --id customer-service
   ```

   `packc` creates `packs/` if it does not exist. It prints a summary:

   ```text
   ✓ Pack compiled successfully: packs/customer-service.pack.json
     Contains 2 prompts: [customer-support sales]
     Contains 1 tools: [lookup_order]
   ```

   The summary lists prompts by task type. It also lists pack-level evals, the workflow and the agents when the pack has them.

3. Check the contents:

   ```bash
   packc inspect packs/customer-service.pack.json
   ```

### Use the defaults

Every `packc compile` flag has a default (see the [compile reference](/packc/reference/compile/)). `--config` defaults to `config.arena.yaml`, `--id` to the current directory's name made into a valid ID, and `--output` to `<id>.pack.json`. From a project directory named `customer-service` that holds `config.arena.yaml`, this writes `customer-service.pack.json`:

```bash
packc compile
```

The default ID comes from the directory you run `packc` in, not from the config file's directory.

### Choose a pack ID

`--id` must start with a lowercase letter and contain only lowercase letters, digits and hyphens. `packc` rejects anything else before it compiles:

```text
Compilation failed: [pack_id] invalid pack ID "app-v1.2.0": must start with a lowercase letter and contain only lowercase letters, digits and dashes (pattern ^[a-z][a-z0-9-]*$)
```

Keep version numbers out of the ID. Set the pack's `version` field with `--pack-version` instead, and put it in the output path if you keep several builds side by side:

```bash
packc compile --config config.arena.yaml --output "packs/app-v${VERSION}.pack.json" --id app --pack-version "${VERSION}"
```

`--pack-version` takes a semantic version, such as `1.2.0` or `v1.2.0`. Without it the pack's `version` is `v1.0.0`.

## Compile a single prompt

`packc compile-prompt` compiles one prompt config into a pack without an arena config. It suits trying out a prompt. It checks the prompt config against its schema and runs the same pack checks as `packc compile`, so a pack it writes passes `packc validate`.

1. Write the prompt config. The pack takes its version from `spec.version` unless you pass `--pack-version`.

   ```yaml
   # prompts/support.yaml
   apiVersion: promptkit.altairalabs.ai/v1alpha1
   kind: PromptConfig
   metadata:
     name: Customer Support Agent
   spec:
     task_type: customer-support
     version: v1.0.0
     description: Handles customer inquiries
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
   ```

2. Compile it. Both flags are required. `packc` creates the output directory if it is missing:

   ```bash
   packc compile-prompt --prompt prompts/support.yaml --output packs/support.pack.json
   ```

   ```text
   Compiling prompt prompts/support.yaml to pack...
   ✓ Pack compiled successfully: packs/support.pack.json
   ```

3. Validate it:

   ```bash
   packc validate packs/support.pack.json
   ```

The pack's `id` is the task type, with any underscores replaced by hyphens.

## Compile several packs

Run `packc compile` once per arena config:

```bash
#!/bin/bash
set -e

for config in config/*.arena.yaml; do
  name=$(basename "$config" .arena.yaml)
  packc compile --config "$config" --output "packs/${name}.pack.json" --id "$name"
  packc validate "packs/${name}.pack.json"
done
```

Each file name must be a valid pack ID, or `--id` fails.

## Verify

```bash
packc validate packs/customer-service.pack.json
```

It exits 0 and ends with `✓ Pack structure is valid`. See [Validate packs](/packc/how-to/validate-packs/) for what it checks.

## Troubleshooting

`packc compile` prints `Compilation failed:` followed by the stage that failed, in brackets. When there are several problems, it lists them all.

### The arena config does not match its schema

```text
Compilation failed: [load] loading arena config: schema validation failed: arena configuration does not match schema:
  - spec: providers is required (value: ...)
  - spec: defaults is required (value: ...)
```

Add the missing fields. The same check runs on each prompt and tool file, and its message names the file:

```text
Compilation failed: [load] loading arena config: schema validation failed for prompts/support.yaml: promptconfig configuration does not match schema:
  - spec: description is required (value: ...)
```

### The config or a prompt file is missing

```text
Compilation failed: [load] loading arena config: failed to read config file: open config.arena.yaml: no such file or directory
Compilation failed: [load] loading arena config: failed to read prompt file prompts/support.yaml: open prompts/support.yaml: no such file or directory
```

Pass the right path with `--config`. Prompt paths in `prompt_configs` are relative to the arena config's directory.

### Invalid YAML

```text
Compilation failed: [load] loading arena config: schema validation failed: failed to parse YAML: yaml: line 3: mapping values are not allowed in this context
```

Fix the YAML at the line given.

### The template references variables that are not declared

```text
Compilation failed: [pack] prompt 'customer-support': system_template references variables, but none are declared
```

Declare each `{{name}}` in the prompt's `spec.variables`.

### A media file is missing

```text
⚠ [compile] media(customer-support): Media file not found: images/logo.png (example: logo-example, part 0)
```

This is a warning, and the pack is still written. Media paths are relative to the arena config's directory for `packc compile`, and to the prompt file's directory for `packc compile-prompt`.

## Related

- [Validate packs](/packc/how-to/validate-packs/)
- [Organize pack files](/packc/how-to/organize-packs/)
- [CI/CD integration](/packc/how-to/ci-cd-integration/)
- [compile reference](/packc/reference/compile/)
- [compile-prompt reference](/packc/reference/compile-prompt/)
- [Compilation](/packc/explanation/compilation/)
