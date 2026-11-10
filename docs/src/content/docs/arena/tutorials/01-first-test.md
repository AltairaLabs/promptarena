---
title: 'Tutorial 1: Your First Test'
description: Install PromptArena, write a prompt, a provider and a two-turn scenario by hand, and run it against OpenAI.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - .goreleaser.yml
    - arena/assertions/types.go
    - arena/cmd/promptarena/init.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/version.go
    - arena/engine/builder_integration.go
    - arena/engine/conversation_executor.go
    - arena/results/json/json_repository.go
    - arena/results/markdown/markdown_repository.go
    - arena/tui/tui.go
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/promptconfig.json
    - schemas/v1alpha1/provider.json
    - schemas/v1alpha1/scenario.json
---
In this tutorial we build and run a two-turn greeting test against OpenAI's `gpt-5.4-mini`. We create every file by hand, so you see what each one does.

## What we will build

A project with four YAML files and a passing run:

```text
my-first-test/
├── config.arena.yaml
├── prompts/greeter.yaml
├── providers/openai.yaml
└── scenarios/greeting-test.yaml
```

## Prerequisites

- Homebrew
- An OpenAI API key

## Step 1: Install PromptArena

```bash
brew install altairalabs/tap/promptarena
promptarena --version
```

The second command prints `promptarena version` followed by a version string.

For other install methods, see [Install PromptArena](/arena/how-to/setup/installation/).

## Step 2: Create the project directory

```bash
mkdir -p my-first-test/prompts my-first-test/providers my-first-test/scenarios
cd my-first-test
```

## Step 3: Create a prompt

Create `prompts/greeter.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: greeter

spec:
  task_type: greeting
  version: v1.0.0
  description: Greets users warmly

  system_template: |
    You are a friendly assistant who greets users warmly.
    Keep responses brief and welcoming.
```

`spec.task_type` is how a scenario selects this prompt, and `spec.system_template` is the system message sent to the model.

## Step 4: Configure a provider

Create `providers/openai.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai-gpt54-mini

spec:
  id: openai-gpt54-mini
  type: openai
  model: gpt-5.4-mini

  defaults:
    temperature: 0.7
    max_tokens: 150
```

`spec.type` selects the provider implementation and `spec.model` the model. The `openai` provider reads its key from the `OPENAI_API_KEY` environment variable.

## Step 5: Set your API key

`promptarena` does not read `.env` files, so export the key in the shell you run it from:

```bash
export OPENAI_API_KEY="sk-your-api-key-here"
```

## Step 6: Write the scenario

Create `scenarios/greeting-test.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: greeting-test
  labels:
    category: basic

spec:
  id: greeting-test
  description: Greets the user and stays brief
  task_type: greeting  # Links to prompts/greeter.yaml

  turns:
    - role: user
      content: "Hello!"
      assertions:
        - type: content_includes
          params:
            patterns: ["hello"]
          message: "Should include greeting"

        - type: content_matches
          params:
            pattern: "^.{1,100}$"
          message: "Response should be brief"

    - role: user
      content: "How are you?"
      assertions:
        - type: content_matches
          params:
            pattern: "."
          message: "Should reply"
```

Each `role: user` turn sends a message and gets a model reply. The `assertions` check that reply: `content_includes` matches case-insensitively, so `hello` matches `Hello!`, and `content_matches` takes a regular expression. The `message` belongs beside `type` and `params`, not inside `params`.

## Step 7: Create the arena configuration

Create `config.arena.yaml` in the project root:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: my-first-test

spec:
  prompt_configs:
    - id: greeter
      file: prompts/greeter.yaml

  providers:
    - file: providers/openai.yaml

  scenarios:
    - file: scenarios/greeting-test.yaml

  defaults:
    temperature: 0.7
    max_tokens: 1500
    output:
      dir: out
      formats: [json, markdown]
```

This file tells Arena which prompts, providers and scenarios to load. `defaults` is required, and its `temperature` and `max_tokens` override the provider's. `output` writes a JSON and a Markdown report to `out/`.

## Step 8: Run the test

```bash
promptarena run --simple
```

`--simple` prints plain output instead of the interactive terminal UI. Arena lists the run settings, then shows a progress bar and a summary:

```text
Done  [████████████████████████]  1/1

Run Summary
────────────────────────────────────────────────────

Total runs      1
Passed          1 (100.0%)
Failed          0 (0.0%)

Assertions      3 total
Assertions Pass all passed

Total cost      $0.0000
Total tokens    121
Total duration  2.3s
Avg duration    2.3s per run

Providers       openai-gpt54-mini (1)
Scenarios       1 scenarios
Regions         default

Saved to        out
```

A run is one scenario against one provider, so both turns count as one run. Cost, tokens and duration vary from run to run.

## Step 9: Review the results

The summary of the whole run is in `out/index.json`:

```bash
cat out/index.json
```

It lists `total_runs`, `successful`, `errors`, the providers and scenarios, and the `run_ids`. Each run also has its own file, `out/<run-id>.json`, holding every message including the assistant replies and the result of each assertion. Open the Markdown report:

```bash
open out/results.md
```

It shows a pass/fail overview and one table row for the scenario. For why tests check behaviour this way, see [Testing Philosophy](/arena/explanation/testing-philosophy/).

## Step 10: Add an edge case

Create `scenarios/edge-cases.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: edge-cases
  labels:
    category: edge-case

spec:
  description: Sends an empty message
  task_type: greeting

  turns:
    - role: user
      content: ""
      assertions:
        - type: content_matches
          params:
            pattern: "^.{10,}$"
          message: "Response should be at least 10 characters"
```

Add it to the `scenarios` list in `config.arena.yaml`:

```yaml
  scenarios:
    - file: scenarios/greeting-test.yaml
    - file: scenarios/edge-cases.yaml
```

Run again:

```bash
promptarena run --simple
```

The summary now shows `Total runs      2` (one per scenario) and `Assertions      4 total`.

## Common issues

### `spec: defaults is required`

The arena configuration needs a `defaults` block. Add the one from Step 7. The error reads `arena configuration does not match schema`.

### `spec: version is required` or `spec: description is required`

Schema validation names the file and the missing field. Prompts need `spec.version` and `spec.description`, and scenarios need `spec.description`.

## Next steps

You have written and run your first test.

- [Tutorial 2: Multi-Provider Testing](/arena/tutorials/02-multi-provider/): run the same scenario across OpenAI, Claude and Gemini.
- [Tutorial 3: Multi-Turn Conversations](/arena/tutorials/03-multi-turn/): build longer dialogs.
- [Write Scenarios](/arena/how-to/scenarios/write-scenarios/): more scenario patterns.
- [Use project templates](/arena/how-to/setup/use-project-templates/): generate a project like this one with `promptarena init`.
