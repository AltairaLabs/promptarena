---
title: 'Tutorial 5: CI/CD Pipeline'
description: Compile and validate a pack on every push with GitHub Actions, and attach the pack to a GitHub release when you push a version tag.
sidebar:
  order: 5
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - packc/main.go
    - packc/main_interactive.go
    - schemas/v1alpha1/arena.json
---
In this tutorial we set up two GitHub Actions workflows for a pack project. The first compiles and validates the pack on every push and pull request. The second attaches the pack to a GitHub release when we push a version tag.

## Prerequisites

- [Tutorial 4: Pack Management](/packc/tutorials/04-pack-management/) completed
- A GitHub account and an empty GitHub repository you can push to
- Git, with `user.name` and `user.email` set

## Step 1: Create the project

```bash
mkdir cicd-packs
cd cicd-packs
git init -b main
mkdir -p prompts config .github/workflows
```

```bash
cat > prompts/assistant.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: AI Assistant
spec:
  task_type: assistant
  description: A helpful AI assistant
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

cat > config/arena.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: cicd-packs
spec:
  prompt_configs:
    - id: assistant
      file: ../prompts/assistant.yaml
  providers: []
  defaults: {}
EOF
```

## Step 2: Run the build locally

The workflows run these two commands. Run them first to confirm the project builds:

```bash
packc compile --config config/arena.yaml --output packs/assistant.pack.json --id assistant
packc validate packs/assistant.pack.json
```

The output ends with `✓ Pack structure is valid`. Keep build output out of Git:

```bash
echo "packs/" > .gitignore
```

## Step 3: Add the build workflow

```bash
cat > .github/workflows/build-packs.yml <<'EOF'
name: Build and Validate Packs

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - uses: actions/setup-go@v7
        with:
          go-version: '1.26'

      - name: Install packc
        run: go install github.com/AltairaLabs/promptarena/v2/packc@latest

      - name: Compile pack
        run: packc compile --config config/arena.yaml --output packs/assistant.pack.json --id assistant

      - name: Validate pack
        run: packc validate packs/assistant.pack.json

      - name: Upload pack
        uses: actions/upload-artifact@v7
        with:
          name: compiled-packs
          path: packs/*.pack.json
EOF
```

`actions/setup-go` puts Go's bin directory on the `PATH`, so the steps after `go install` find `packc`. A compile or validation failure fails the job, which fails the check on the pull request.

## Step 4: Add the release workflow

```bash
cat > .github/workflows/release.yml <<'EOF'
name: Release Pack

on:
  push:
    tags: ['v*.*.*']

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - uses: actions/setup-go@v7
        with:
          go-version: '1.26'

      - name: Install packc
        run: go install github.com/AltairaLabs/promptarena/v2/packc@latest

      - name: Compile pack
        run: packc compile --config config/arena.yaml --output "packs/assistant-${{ github.ref_name }}.pack.json" --id assistant

      - name: Validate pack
        run: packc validate "packs/assistant-${{ github.ref_name }}.pack.json"

      - name: Create release
        env:
          GH_TOKEN: ${{ github.token }}
        run: gh release create "${{ github.ref_name }}" "packs/assistant-${{ github.ref_name }}.pack.json" --title "${{ github.ref_name }}" --notes "Compiled pack for ${{ github.ref_name }}."
EOF
```

`github.ref_name` is the tag, for example `v1.0.0`. The `contents: write` permission lets the job create the release.

## Step 5: Push and watch the build

Replace `<owner>/<repo>` with your repository:

```bash
git add .
git commit -m "Add pack project and CI workflows"
git remote add origin https://github.com/<owner>/<repo>.git
git push -u origin main
```

Open the repository's **Actions** tab. The **Build and Validate Packs** run compiles and validates the pack, and its summary page offers the `compiled-packs` artifact for download.

## Step 6: Release a version

```bash
git tag v1.0.0
git push origin v1.0.0
```

The **Release Pack** run creates a release named `v1.0.0` on the repository's **Releases** page, with `assistant-v1.0.0.pack.json` attached.

## Next steps

- [CI/CD integration](/packc/how-to/ci-cd-integration/): other CI systems, caching and multi-environment builds
- [PackC explanation](/packc/explanation/): how compilation and validation work
- [PromptArena](/arena/): test the prompts in your pack before you release it
