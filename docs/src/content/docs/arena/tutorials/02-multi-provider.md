---
title: 'Tutorial 2: Multi-Provider Testing'
description: Run one customer-support scenario against OpenAI, Claude and Gemini in a single command, then read each provider's replies from the run output.
verified:
  commit: 02ec7c860b978fcce24022d9ccb868d94208c751
  sources:
    - arena/arenaconfig/ignored_fields.go
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/config_inspect_interactive.go
    - arena/cmd/promptarena/run.go
    - arena/configschema/configschema.go
    - arena/inspect/collector.go
    - examples/capability-matrix/providers/anthropic-haiku55.provider.yaml
    - examples/capability-matrix/providers/anthropic-sonnet46.provider.yaml
    - examples/capability-matrix/providers/gemini-38-flash.provider.yaml
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/promptconfig.json
    - schemas/v1alpha1/provider.json
    - schemas/v1alpha1/scenario.json
---
In this tutorial we run one scenario against three providers and read how each one answered. We build on [Tutorial 1: Your First Test](/arena/tutorials/01-first-test/), so we reuse its `config.arena.yaml` layout and `promptarena run --simple`. For why the comparison is worth making, see [Provider comparison](/arena/explanation/provider-comparison/).

## What we will build

A project with one prompt, one scenario and three providers. One run produces one result per provider:

```text
multi-provider/
├── config.arena.yaml
├── prompts/support.yaml
├── providers/
│   ├── openai.yaml
│   ├── claude.yaml
│   └── gemini.yaml
└── scenarios/customer-support.yaml
```

## Prerequisites

- PromptArena installed, as in Tutorial 1
- An OpenAI API key (`OPENAI_API_KEY`)
- An Anthropic API key (`ANTHROPIC_API_KEY`)
- A Gemini API key (`GEMINI_API_KEY`)

## Step 1: Create the project and export your keys

```bash
mkdir -p multi-provider/prompts multi-provider/providers multi-provider/scenarios
cd multi-provider
```

`promptarena` does not read `.env` files, so export the keys in the shell you run it from:

```bash
export OPENAI_API_KEY="sk-your-openai-key"
export ANTHROPIC_API_KEY="sk-ant-your-anthropic-key"
export GEMINI_API_KEY="your-gemini-key"
```

## Step 2: Create a prompt

Create `prompts/support.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: support

spec:
  version: v1.0.0
  description: Customer support assistant
  task_type: support

  system_template: |
    You are a helpful customer support assistant.
    Keep responses brief.
```

All three providers receive this same system message.

## Step 3: Configure three providers

Create `providers/openai.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai-gpt54-mini

spec:
  type: openai
  model: gpt-5.4-mini

  defaults:
    temperature: 0.7
    max_tokens: 500
```

Create `providers/claude.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: claude-haiku

spec:
  type: claude
  model: claude-haiku-5-5

  defaults:
    temperature: 0.7
    max_tokens: 500
```

Create `providers/gemini.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: gemini-flash

spec:
  type: gemini
  model: gemini-3.8-flash

  defaults:
    temperature: 0.7
    max_tokens: 500
```

The three files differ only in `spec.type` and `spec.model`. `metadata.name` is the provider ID that appears in the run output. The `openai`, `claude` and `gemini` providers read `OPENAI_API_KEY`, `ANTHROPIC_API_KEY` and `GEMINI_API_KEY`.

## Step 4: Write the scenario

Create `scenarios/customer-support.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: customer-support

spec:
  description: Customer support comparison
  task_type: support

  turns:
    - role: user
      content: "I'm having trouble logging into my account. Can you help?"
      assertions:
        - type: content_matches
          params:
            pattern: "(?s)^.{1,1000}$"
          message: "Keep response concise"

    - role: user
      content: "I've tried resetting my password but didn't receive an email."
      assertions:
        - type: content_includes
          params:
            patterns: ["email"]
          message: "Should address email issue"
```

Every provider answers the same two turns and the same assertions check every answer. The assertions stay loose because the providers word their replies differently. The `(?s)` flag lets `.` match newlines, so a reply with line breaks still matches `content_matches`. Regular expression repeat counts are limited to 1000.

## Step 5: Create the arena configuration

Create `config.arena.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: multi-provider-test

spec:
  prompt_configs:
    - id: support
      file: prompts/support.yaml

  providers:
    - file: providers/openai.yaml
    - file: providers/claude.yaml
    - file: providers/gemini.yaml

  scenarios:
    - file: scenarios/customer-support.yaml

  defaults:
    temperature: 0.7
    max_tokens: 500
    output:
      dir: out
      formats: [json, markdown]
```

Listing three files under `providers` is all it takes: `promptarena run` runs every scenario against every provider.

## Step 6: Run against all providers

```bash
promptarena run --simple
```

Log lines scroll past, then a progress bar and a summary (trimmed here):

```text
Generated 3 run combinations
Starting execution…

Done  [████████████████████████]  3/3

Run Summary
────────────────────────────────────────────────────

Total runs      3

Providers       claude-haiku (1), gemini-flash (1), openai-gpt54-mini (1)
Scenarios       1 scenarios
Regions         default

Saved to        out
```

A run is one scenario against one provider, so three providers and one scenario make three runs. The `Passed` and `Failed` lines, cost, tokens and durations also appear and vary from run to run. If one provider's key is missing or out of credit, only that run fails and the summary lists the error under `FAILURES`.

## Step 7: Compare the providers

Open the Markdown report:

```bash
open out/results.md
```

Its Test Results table has one row per provider, with status, duration, cost and the number of assertions. The Cost Breakdown table lists the total cost per provider.

The replies themselves are in the run files. `out/index.json` lists the `run_ids`, and each `out/<run-id>.json` holds the messages, including each assistant reply with its `latency_ms` and `cost_info`. Print the assistant replies of every run:

```bash
python3 -c "
import glob, json
for path in sorted(glob.glob('out/20*.json')):
    run = json.load(open(path))
    print('==', run['ProviderID'])
    for message in run['Messages']:
        if message['role'] == 'assistant':
            print(message['content'][:200])
"
```

The providers answer the same prompt in different lengths and styles: one may reply with a numbered list of steps while another asks for more detail.

## Step 8: Run a subset of providers

Pass provider IDs to `--provider` to run only those:

```bash
promptarena run --simple --provider openai-gpt54-mini,claude-haiku
```

The summary now shows two runs and lists only those providers:

```text
Total runs      2
Passed          2 (100.0%)
Failed          0 (0.0%)

Providers       openai-gpt54-mini (1), claude-haiku (1)
```

## Next steps

You have run one scenario against three providers.

- [Tutorial 3: Multi-Turn Conversations](/arena/tutorials/03-multi-turn/): build longer dialogs.
- [Tutorial 4: MCP Tools](/arena/tutorials/04-mcp-tools/): test tool calling.
- [Configure Providers](/arena/how-to/providers/configure-providers/): every provider option, including Azure OpenAI and other OpenAI-compatible endpoints.
