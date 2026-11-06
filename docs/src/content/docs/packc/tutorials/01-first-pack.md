---
title: 'Tutorial 1: Your First Pack'
description: Install packc, write one prompt and an Arena config, then compile, inspect and validate your first PromptPack file.
sidebar:
  order: 1
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - packc/main.go
    - packc/main_interactive.go
    - schemas/v1alpha1/arena.json
---

In this tutorial we build a `.pack.json` file that holds one greeting prompt, in the [PromptPack](https://promptpack.org) format. We install `packc`, write the prompt, compile it, and check the result.

## Prerequisites

- Go 1.26 or later
- A terminal and a text editor

## Step 1: Install packc

```bash
go install github.com/AltairaLabs/promptarena/v2/packc@latest
packc version
```

You see:

```text
packc dev
```

A `go install` build always reports `dev`. If your shell cannot find `packc`, add Go's bin directory to your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Step 2: Create the project

```bash
mkdir my-first-pack
cd my-first-pack
mkdir -p prompts config packs
```

`prompts/` holds prompt configs, `config/` holds the Arena config, and `packs/` receives the compiled pack.

## Step 3: Write a prompt

```bash
cat > prompts/greeting.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: Greeting Assistant
spec:
  task_type: greeting
  description: A friendly assistant that greets users
  version: v1.0.0

  system_template: |
    You are a friendly assistant. Greet the user warmly and ask how you can help.
    User name: {{user_name}}
    Time of day: {{time_of_day}}

  template_engine:
    version: v1
    syntax: "{{variable}}"

  variables:
    - name: user_name
      type: string
      required: true
      description: The user's name
    - name: time_of_day
      type: string
      required: false
      default: morning
      description: Time of day for greeting
EOF
```

The prompt declares two template variables, one required and one with a default.

## Step 4: Write the Arena config

The Arena config lists the prompts that go into the pack:

```bash
cat > config/arena.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: my-first-pack
spec:
  prompt_configs:
    - id: greeting
      file: ../prompts/greeting.yaml
  providers: []
  defaults: {}
EOF
```

The `file` path is relative to the config file. The Arena schema requires `providers` and `defaults`; `packc` does not use them, so we leave them empty.

## Step 5: Compile the pack

```bash
packc compile \
  --config config/arena.yaml \
  --output packs/greeting.pack.json \
  --id greeting
```

You see:

```text
✓ Pack compiled successfully: packs/greeting.pack.json
  Contains 1 prompts: [greeting]
```

`packc` loaded the config and the prompt, compiled them, validated the result, and wrote `packs/greeting.pack.json`.

## Step 6: Inspect the pack

```bash
packc inspect packs/greeting.pack.json
```

You see a summary like this one, with your own timestamp:

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

The pack's ID and name come from `--id`. The prompt keeps the name, description, version and variables from `greeting.yaml`.

## Step 7: Validate the pack

```bash
packc validate packs/greeting.pack.json
```

You see:

```text
Validating pack: packs/greeting.pack.json
Validating against PromptPack schema...
✓ Schema validation passed
✓ Pack structure is valid
```

## Step 8: Read the pack JSON

```bash
cat packs/greeting.pack.json
```

The file holds the pack's `id`, `name` and `version`, the `template_engine`, the `greeting` prompt under `prompts` with its `system_template`, `variables` and a default `pipeline`, and a `compilation` record naming the compiler and the time.

We now have a PromptPack file that any PromptPack runtime can load.

## Next steps

- [Tutorial 2: Multi-Prompt Packs](/packc/tutorials/02-multi-prompt/): put several prompts and a tool in one pack
- [compile](/packc/reference/compile/): every flag and default
- [Pack format](/packc/explanation/pack-format/): what each field in the pack means
