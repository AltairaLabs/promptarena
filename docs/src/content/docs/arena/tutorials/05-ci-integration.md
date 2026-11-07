---
title: 'Tutorial 5: CI/CD Integration'
description: Build a keyless Arena test suite with a mock provider, see a failed assertion fail the run, and run the suite in GitHub Actions.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - arena/arenaconfig/helpers.go
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/mock_config_check.go
    - arena/assertions/turn_summary.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/engine/engine.go
    - arena/engine/execution.go
    - arena/results/json/json_repository.go
    - arena/results/junit/junit_repository.go
    - arena/results/markdown/markdown_repository.go
    - arena/tui/tui.go
    - npm/promptarena/lib/resolve.js
    - npm/promptarena/package.json
    - schemas/v1alpha1/scenario.json
---
In this tutorial we build a test suite that runs without an API key, watch a failing assertion turn the exit code to 1, and run the same command in a GitHub Actions workflow.

## What we will build

```text
my-ci-project/
  .github/workflows/arena.yml
  tests/
    config.arena.yaml
    mock-responses.yaml
    prompts/assistant.yaml
    providers/mock.yaml
    scenarios/hours.yaml
```

## Prerequisites

- `promptarena` installed ([Tutorial 1](/arena/tutorials/01-first-test/), Step 1)
- A GitHub repository to push the project to

No provider key is needed. The mock provider replies from a file.

## Step 1: Create the project

```bash
mkdir -p my-ci-project/tests/prompts my-ci-project/tests/providers my-ci-project/tests/scenarios my-ci-project/.github/workflows
cd my-ci-project/tests
```

All commands from here run in `tests/`.

## Step 2: Create a prompt and a mock provider

Create `prompts/assistant.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: assistant

spec:
  task_type: support
  version: v1.0.0
  description: Answers support questions

  system_template: |
    You are a support assistant. Keep answers brief.
```

Create `providers/mock.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: mock

spec:
  id: mock-provider
  type: mock
  model: mock-model
  additional_config:
    mock_config: mock-responses.yaml
```

The `mock` provider reads its replies from `mock_config`, a path relative to the directory holding the arena configuration.

Create `mock-responses.yaml`:

```yaml
defaultResponse: "This is a mock response for testing."

scenarios:
  business-hours:
    turns:
      1: "We are open Monday to Friday, 9am to 5pm."
      2: "You can contact support by email at help@example.com."
```

Scenario keys are scenario names. The numbers count model calls within the scenario.

## Step 3: Create a scenario and the configuration

Create `scenarios/hours.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: business-hours

spec:
  id: business-hours
  description: Answers questions about hours and contact details
  task_type: support

  turns:
    - role: user
      content: "What are your business hours?"
      assertions:
        - type: content_includes
          params:
            patterns: ["Monday"]
          message: "Should mention business days"

    - role: user
      content: "How do I contact support?"
      assertions:
        - type: content_includes
          params:
            patterns: ["email"]
          message: "Should provide contact methods"
```

Create `config.arena.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: ci-tests

spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers:
    - file: providers/mock.yaml

  scenarios:
    - file: scenarios/hours.yaml

  defaults:
    temperature: 0.7
    max_tokens: 1500
    output:
      dir: out
      formats: [json]
```

## Step 4: Run the suite in CI mode

```bash
promptarena run --ci --format junit,json
```

`--ci` turns off the interactive terminal UI and prints a plain summary. `--format` replaces the formats in the configuration, so this run writes `out/junit.xml` and `out/index.json`. The summary reads:

```text
Run Summary
===========

Total Runs:       1
Successful:       1 (100.0%)
Failed:           0 (0.0%)

Assertions:       2 total
Assertions Pass:  all passed
```

`Assertions` counts the turn-level and conversation-level assertions together, here one per turn.

Check the exit code:

```bash
echo $?
```

It prints `0`. A CI job passes when the command exits `0`.

## Step 5: Break an assertion

In `scenarios/hours.yaml`, change the first pattern from `Monday` to `Sunday`, then run again:

```bash
promptarena run --ci --format junit,json
echo $?
```

The mock reply does not contain `Sunday`. The summary reports `Assertions Fail:  1`, ends with the failed assertion, and the exit code is `1`:

```text
Errors:
  • business-hours/mock-provider/default: pipeline execution failed: validation failed: [assertion 0 (content_includes) failed: missing patterns: Sunday]

Error: execution failed: 1 runs had errors, 1 assertions failed
```

The run stops at the failed turn, so the second turn's assertion never runs and the summary counts one assertion.

A CI job fails on that exit code. In `out/junit.xml` the failed run is recorded as an `<error>` element, not `<failure>`.

Change `Sunday` back to `Monday` and run once more to see `0` again.

## Step 6: Add the workflow

Create `.github/workflows/arena.yml` in the project root, one level above `tests/`:

```yaml
name: Arena tests

on:
  push:
    branches: [main]
  pull_request:

jobs:
  arena:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: 20

      - name: Install PromptArena
        run: npm install -g @altairalabs/promptarena

      - name: Run Arena tests
        working-directory: tests
        run: promptarena run --ci --format junit,json

      - name: Upload results
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: arena-results
          path: tests/out/
```

`working-directory: tests` matches where we ran the command locally. `if: always()` uploads `out/` even when the run step fails.

Commit the project and push it. The workflow runs on the push, and the `arena-results` artifact holds `junit.xml` and `index.json`.

## Next steps

You have a keyless Arena suite gating a GitHub Actions workflow.

- [Run Arena in CI](/arena/how-to/interfaces/run-in-ci/): use real provider keys as repository secrets.
- [Tutorial 6: Duplex Testing](/arena/tutorials/06-duplex-testing/): test streaming voice conversations.
- [Arena How-To Guides](/arena/how-to/) and [Arena Reference](/arena/reference/).
