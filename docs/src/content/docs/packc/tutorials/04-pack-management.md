---
title: 'Tutorial 4: Pack Management'
description: Build a versioned pack per environment from one prompt, then release a new prompt version and compare the two packs.
sidebar:
  order: 4
verified:
  commit: 66ec692303a313b1f7ece7ce55dd5f73859efe29
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - packc/compiler/compiler.go
    - packc/main.go
    - packc/main_interactive.go
    - schemas/v1alpha1/arena.json
---
In this tutorial we build a dev pack and a prod pack from one prompt, with a version in each file name, then release a new prompt version and compare it with the previous one.

## Prerequisites

- [Tutorial 3: Validation Workflows](/packc/tutorials/03-validation-workflow/) completed
- A shell with `bash` or `zsh`

## Step 1: Create the project

```bash
mkdir pack-management
cd pack-management
mkdir -p prompts config/dev config/prod packs scripts
```

## Step 2: Write the prompt

```bash
cat > prompts/assistant.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: AI Assistant
spec:
  task_type: assistant
  description: Production assistant
  version: v1.0.0
  system_template: |
    You are a helpful AI assistant.
    User: {{user_message}}
  template_engine:
    version: v1
    syntax: "{{variable}}"
  variables:
    - name: user_message
      type: string
      required: true
EOF
```

The prompt's `version` is the version we release.

## Step 3: Write one Arena config per environment

Both configs use the same prompt. `pack_metadata` tags each pack with its environment:

```bash
cat > config/dev/arena.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: assistant-dev
spec:
  prompt_configs:
    - id: assistant
      file: ../../prompts/assistant.yaml
  pack_metadata:
    tags: [dev]
  providers: []
  defaults: {}
EOF

cat > config/prod/arena.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: assistant-prod
spec:
  prompt_configs:
    - id: assistant
      file: ../../prompts/assistant.yaml
  pack_metadata:
    tags: [prod]
  providers: []
  defaults: {}
EOF
```

## Step 4: Write the build script

The script takes a version, compiles one pack per environment into `packs/<env>/`, and validates each:

```bash
cat > scripts/build-all.sh <<'EOF'
#!/bin/bash
set -e

VERSION="$1"
if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version>"
  exit 1
fi

for env in dev prod; do
  pack="packs/$env/assistant-$VERSION.pack.json"
  packc compile --config "config/$env/arena.yaml" --output "$pack" --id "assistant-$env"
  packc validate "$pack"
done
EOF
chmod +x scripts/build-all.sh
```

`packc compile` creates `packs/dev/` and `packs/prod/` when they do not exist.

## Step 5: Build version 1.0.0

```bash
./scripts/build-all.sh 1.0.0
ls packs/*
```

You see one pack per environment:

```text
packs/dev:
assistant-1.0.0.pack.json

packs/prod:
assistant-1.0.0.pack.json
```

Inspect the prod pack:

```bash
packc inspect packs/prod/assistant-1.0.0.pack.json
```

The summary shows the environment's ID and tag:

```text
=== Pack Information ===
Pack: assistant-prod
ID: assistant-prod
Version: v1.0.0
Description: Pack containing 1 prompts
...
Pack Metadata:
  Tags: [prod]
```

The dev pack shows `assistant-dev` and `[dev]`.

## Step 6: Release version 1.1.0

Change the prompt and bump its version:

```bash
sed -i.bak -e 's/version: v1.0.0/version: v1.1.0/' -e 's/You are a helpful AI assistant./You are a concise AI assistant./' prompts/assistant.yaml
rm prompts/assistant.yaml.bak
./scripts/build-all.sh 1.1.0
```

Each environment directory now holds both versions.

## Step 7: Compare the two prod packs

```bash
diff <(packc inspect packs/prod/assistant-1.0.0.pack.json) \
     <(packc inspect packs/prod/assistant-1.1.0.pack.json)
```

The diff shows the prompt's new version:

```text
15c15
<   Version: v1.0.0
---
>   Version: v1.1.0
```

The `Compilation:` line differs too when the two builds ran in different seconds. The pack-level `Version:` stays `v1.0.0`, because the build script does not pass `--pack-version`, so the prompt version and the file name carry the release number. Add `--pack-version 1.1.0` to the compile to put the release number in the pack too. To see the prompt text change, compare the JSON (it also shows `created_at` when the builds ran in different seconds):

```bash
diff packs/prod/assistant-1.0.0.pack.json packs/prod/assistant-1.1.0.pack.json
```

```text
50c50
<       "system_template": "You are a helpful AI assistant.\nUser: {{user_message}}\n",
---
>       "system_template": "You are a concise AI assistant.\nUser: {{user_message}}\n",
58c58
<       "version": "v1.0.0"
---
>       "version": "v1.1.0"
```

## Next steps

- [Tutorial 5: CI/CD Pipeline](/packc/tutorials/05-ci-cd-pipeline/): build and release packs in GitHub Actions
- [Organize packs](/packc/how-to/organize-packs/): more layouts for environments and versions
- [Pack format](/packc/explanation/pack-format/)
