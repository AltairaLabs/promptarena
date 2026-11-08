---
title: 'Tutorial 3: Validation Workflows'
description: See what packc compile and packc validate catch, then wire both into a build script and a Git pre-commit hook.
sidebar:
  order: 3
verified:
  commit: 66ec692303a313b1f7ece7ce55dd5f73859efe29
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - packc/compiler/check.go
    - packc/main.go
    - packc/main_interactive.go
    - schemas/v1alpha1/arena.json
---
In this tutorial we break a prompt and a pack to see what `packc` catches, then build a script and a Git pre-commit hook that stop a broken pack from being committed.

## Prerequisites

- [Tutorial 2: Multi-Prompt Packs](/packc/tutorials/02-multi-prompt/) completed
- Git, with `user.name` and `user.email` set

## Step 1: Create the project

```bash
mkdir validation-workflow
cd validation-workflow
git init
mkdir -p prompts config packs scripts
```

```bash
cat > config/arena.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: validation-workflow
spec:
  prompt_configs:
    - id: assistant
      file: ../prompts/assistant.yaml
  providers: []
  defaults: {}
EOF
```

## Step 2: Compile a broken prompt

This prompt uses `{{user_message}}` but declares no variables:

```bash
cat > prompts/assistant.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: AI Assistant
spec:
  task_type: assistant
  description: General purpose assistant
  version: v1.0.0
  system_template: |
    You are a helpful AI assistant.
    User: {{user_message}}
  template_engine:
    version: v1
    syntax: "{{variable}}"
EOF
```

```bash
packc compile --config config/arena.yaml --output packs/assistant.pack.json --id assistant
```

The compile fails and writes nothing:

```text
Compilation failed: [pack] prompt 'assistant': system_template references variables, but none are declared
```

`packc compile` runs every check `packc validate` runs before it writes a pack. The `[pack]` tag names the check that failed.

## Step 3: Fix the prompt

Append the variable declaration:

```bash
cat >> prompts/assistant.yaml <<'EOF'
  variables:
    - name: user_message
      type: string
      required: true
EOF
```

```bash
packc compile --config config/arena.yaml --output packs/assistant.pack.json --id assistant
```

You see `✓ Pack compiled successfully: packs/assistant.pack.json`.

## Step 4: Validate a damaged pack

A pack can change after it is compiled, by hand or by another tool. We change both version strings in the pack to `1.0`, which is not a semantic version:

```bash
sed -i.bak 's/"version": "v1.0.0"/"version": "1.0"/' packs/assistant.pack.json
packc validate packs/assistant.pack.json
```

Validation fails on the pack's `version` and the prompt's `version`. The schema check and the structural check each report both (the pattern is shortened here):

```text
Validating pack: packs/assistant.pack.json
✗ Pack validation failed: packs/assistant.pack.json
  - [pack_schema] version: Does not match pattern '^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)...' (value: 1.0)
  - [pack_schema] prompts.assistant.version: Does not match pattern '^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)...' (value: 1.0)
  - [pack] invalid pack version '1.0': invalid semantic version: invalid semantic version
  - [pack] prompt 'assistant': invalid version '1.0': invalid semantic version: invalid semantic version
```

Restore the pack and validate again:

```bash
mv packs/assistant.pack.json.bak packs/assistant.pack.json
packc validate packs/assistant.pack.json
```

You see `✓ Schema validation passed` and `✓ Pack structure is valid`.

## Step 5: Write a build script

The script compiles every pack and validates it, and stops at the first failure:

```bash
cat > scripts/build.sh <<'EOF'
#!/bin/bash
set -e

packc compile --config config/arena.yaml --output packs/assistant.pack.json --id assistant
packc validate packs/assistant.pack.json

echo "Build complete"
EOF
chmod +x scripts/build.sh
./scripts/build.sh
```

The output ends with `✓ Pack structure is valid` and `Build complete`.

## Step 6: Add a pre-commit hook

The hook runs the build script before every commit and blocks the commit if it fails:

```bash
cat > .git/hooks/pre-commit <<'EOF'
#!/bin/bash
if ! ./scripts/build.sh; then
  echo "Pack build failed: commit blocked"
  exit 1
fi
EOF
chmod +x .git/hooks/pre-commit
```

Commit the project:

```bash
git add prompts config scripts
git commit -m "Add assistant prompt"
```

The hook's build output prints, then Git records the commit.

## Step 7: Watch the hook block a commit

Change the prompt's version to `1.0`:

```bash
sed -i.bak 's/version: v1.0.0/version: "1.0"/' prompts/assistant.yaml
rm prompts/assistant.yaml.bak
git commit -am "Break the version"
```

The compile fails on the prompt config's schema, and the hook stops the commit:

```text
Compilation failed: [load] loading arena config: schema validation failed for ../prompts/assistant.yaml: promptconfig configuration does not match schema:
  - spec.version: Does not match pattern ... (value: 1.0)
Pack build failed: commit blocked
```

`git log --oneline` still shows one commit. Put the version back to `v1.0.0` and the commit goes through.

## Next steps

- [Tutorial 4: Pack Management](/packc/tutorials/04-pack-management/): build and compare packs per environment
- [validate](/packc/reference/validate/): every check and its message
- [Validate packs](/packc/how-to/validate-packs/)
