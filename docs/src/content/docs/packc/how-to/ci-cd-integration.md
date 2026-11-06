---
title: CI/CD Integration
description: Compile and validate packs on every change in GitHub Actions, GitLab CI or any other CI system, and attach them to releases.
sidebar:
  order: 5
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - .github/actions/packc-action/action.yml
    - .github/actions/packc-action/src/compiler.ts
    - .github/actions/packc-action/src/installer.ts
    - arena/arenaconfig/types.go
    - npm/packc/lib/resolve.js
    - npm/packc/package.json
    - packc/compiler/check.go
    - packc/compiler/compiler.go
    - packc/main.go
    - packc/main_interactive.go
    - schemas/v1alpha1/arena.json
---
Compile and validate your packs in CI, so a broken prompt fails the build instead of a deployment.

## Prerequisites

- A repository with an arena config that `packc compile` accepts locally.
- A CI system that can run shell commands. Installing with `go install` needs Go 1.26 or later on the runner, and installing with npm needs Node.js.

Every example runs the same two commands. Both exit 1 on failure, which fails the CI step:

```bash
packc compile --config config.arena.yaml --output packs/app.pack.json --id app
packc validate packs/app.pack.json
```

`packc compile` creates `packs/` if it is missing.

## GitHub Actions with packc-action

The PromptArena repository ships a JavaScript action that downloads a `packc` release binary, compiles, and validates:

```yaml
# .github/workflows/build-packs.yml
name: Build Packs

on:
  push:
    branches: [main]
  pull_request:

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - id: pack
        uses: AltairaLabs/promptarena/.github/actions/packc-action@v2.4.0
        with:
          config-file: config.arena.yaml
          pack-id: app
          output: packs/app.pack.json

      - uses: actions/upload-artifact@v4
        with:
          name: packs
          path: ${{ steps.pack.outputs.pack-file }}
```

`validate` defaults to `true`, and `packc-version` defaults to the latest release. The action's outputs include `pack-file`, `pack-id`, `prompts` and `tools`. It can also push the pack to an OCI registry and sign it; see the action's `action.yml` for those inputs.

## GitHub Actions with go install

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Install packc
        run: go install github.com/AltairaLabs/promptarena/v2/packc@v2.4.0

      - name: Compile and validate
        run: |
          packc compile --config config.arena.yaml --output packs/app.pack.json --id app
          packc validate packs/app.pack.json

      - uses: actions/upload-artifact@v4
        with:
          name: packs
          path: packs/*.pack.json
```

`actions/setup-go` puts Go's bin directory on `PATH`, so `packc` runs straight after the install. Pin a release tag instead of `@latest` so that every build uses the same compiler.

### One pack per environment

Use a matrix when each environment has its own arena config:

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        environment: [dev, staging, prod]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go install github.com/AltairaLabs/promptarena/v2/packc@v2.4.0
      - run: |
          packc compile \
            --config config/arena.${{ matrix.environment }}.yaml \
            --output packs/${{ matrix.environment }}/app.pack.json \
            --id app-${{ matrix.environment }}
          packc validate packs/${{ matrix.environment }}/app.pack.json
      - uses: actions/upload-artifact@v4
        with:
          name: pack-${{ matrix.environment }}
          path: packs/${{ matrix.environment }}/app.pack.json
```

### Attach packs to a release

`packc compile` always writes `v1.0.0` as the pack's `version` field, so carry the release version in the file name:

```yaml
on:
  push:
    tags: ['v*.*.*']

jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go install github.com/AltairaLabs/promptarena/v2/packc@v2.4.0
      - run: |
          packc compile --config config.arena.yaml --output "packs/app-${GITHUB_REF_NAME}.pack.json" --id app
          packc validate "packs/app-${GITHUB_REF_NAME}.pack.json"
      - uses: softprops/action-gh-release@v2
        with:
          files: packs/*.pack.json
```

## GitLab CI

The `golang` image has Go on `PATH` and `$GOPATH/bin` on `PATH`:

```yaml
# .gitlab-ci.yml
build-packs:
  image: golang:1.26
  script:
    - go install github.com/AltairaLabs/promptarena/v2/packc@v2.4.0
    - packc compile --config config.arena.yaml --output packs/app.pack.json --id app
    - packc validate packs/app.pack.json
  artifacts:
    paths:
      - packs/*.pack.json
```

For an image without Go, install with npm instead: `npm install -g @altairalabs/packc`.

## Other CI systems

Jenkins, CircleCI and the rest need the same three steps: install `packc`, compile, validate. A Jenkins declarative stage:

```groovy
stage('Packs') {
  steps {
    sh 'npm install -g @altairalabs/packc'
    sh 'packc compile --config config.arena.yaml --output packs/app.pack.json --id app'
    sh 'packc validate packs/app.pack.json'
    archiveArtifacts artifacts: 'packs/*.pack.json', fingerprint: true
  }
}
```

A Makefile keeps the commands identical locally and in CI:

```makefile
.PHONY: packs
packs:
	packc compile --config config.arena.yaml --output packs/app.pack.json --id app
	packc validate packs/app.pack.json
```

## Verify

Push a change and check that the job is green and the pack artifact is attached. To confirm the job can fail, set a prompt's `spec.version` to `latest` on a branch. Compile fails with a `[load]` schema error and the job goes red.

## Troubleshooting

### packc: command not found

The install directory is not on `PATH`. After `go install`, add Go's bin directory:

```yaml
- run: echo "$(go env GOPATH)/bin" >> "$GITHUB_PATH"
```

### The config is not found in CI

`--config` and the default pack ID both depend on the working directory. Run `packc` from the directory you run it from locally, or pass `--config` and `--id` explicitly. With packc-action, set `working-directory`.

### Validating a glob checks only one pack

`packc validate` checks only its first argument. Loop over the files, as shown in [Validate packs](/packc/how-to/validate-packs/#validate-several-packs).

## Related

- [Validate packs](/packc/how-to/validate-packs/)
- [Organize pack files](/packc/how-to/organize-packs/)
- [CI/CD pipeline tutorial](/packc/tutorials/05-ci-cd-pipeline/)
- [compile reference](/packc/reference/compile/)
