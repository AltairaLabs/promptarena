---
title: 'Tutorial 4: Testing MCP Tools'
description: Connect the npm memory MCP server to Arena, let OpenAI call its tools, and assert on the tool calls.
verified:
  commit: 45cbd09a1d70928da45e071755b0c2172abfc8ca
  sources:
    - arena/arenaconfig/manifest_helpers.go
    - arena/arenaconfig/mcp_servers.go
    - arena/arenaconfig/types.go
    - arena/arenaconfig/validator_tools.go
    - arena/assertions/types_test.go
    - arena/assertions/validate_params.go
    - arena/cmd/promptarena/run.go
    - arena/engine/builder_integration.go
    - arena/engine/engine.go
    - arena/engine/mcp_tool_descriptor.go
    - arena/mcp/config.go
    - examples/mcp-filesystem-test/config.arena.yaml
    - examples/mcp-memory-test/config.arena.yaml
    - examples/mcp-memory-test/prompts/assistant.yaml
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/promptconfig.json
    - schemas/v1alpha1/scenario.json
    - schemas/v1alpha1/tool.json
---
In this tutorial we test a model that stores and recalls a fact through a Model Context Protocol (MCP) server. Arena starts the server, offers its tools to the model, and checks which tools the model called.

## What we will build

A project that runs two turns against OpenAI and the `memory` MCP server:

```text
mcp-test/
├── config.arena.yaml
├── prompts/assistant-with-tools.yaml
├── providers/openai.yaml
└── scenarios/tool-calling-test.yaml
```

## Prerequisites

- `promptarena` installed ([Tutorial 1](/arena/tutorials/01-first-test/))
- Node.js, so that `npx` is available
- An OpenAI API key

## Step 1: Create the project directory

```bash
mkdir -p mcp-test/prompts mcp-test/providers mcp-test/scenarios
cd mcp-test
export OPENAI_API_KEY="sk-your-api-key-here"
```

`promptarena` does not read `.env` files, so the key is exported in the shell you run it from.

## Step 2: Create the provider

Create `providers/openai.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai
spec:
  id: openai
  type: openai
  model: gpt-4o-mini
  defaults:
    temperature: 0.2
    max_tokens: 500
```

## Step 3: Create a prompt that may use the tools

Create `prompts/assistant-with-tools.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: assistant-with-tools

spec:
  task_type: assistant
  version: "v1.0.0"
  description: "Assistant with MCP memory tools"

  system_template: |
    You are a helpful assistant with access to a knowledge graph memory.

    When users ask you to remember information, use the create_entities tool.
    When users ask you to recall information, use the read_graph tool.

    Always confirm when you've stored or retrieved information.

  allowed_tools:
    - mcp__memory__*
```

Arena names every MCP tool `mcp__<server>__<tool>`. A prompt only offers the tools listed in `allowed_tools`, so `mcp__memory__*` offers every tool of the server named `memory`. Without it, the model sees no tools.

## Step 4: Create the scenario

Create `scenarios/tool-calling-test.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: basic-tool-calling
  labels:
    category: tools
    protocol: mcp

spec:
  task_type: assistant
  description: Store and recall a fact through the MCP memory server

  turns:
    - role: user
      content: "Remember that my favorite color is blue"
      assertions:
        - type: tools_called
          params:
            tools: ["mcp__memory__create_entities"]
          message: "Should call create_entities"
        - type: content_includes
          params:
            patterns: ["blue"]
          message: "Should confirm storage"

    - role: user
      content: "What's my favorite color?"
      assertions:
        - type: tools_called
          params:
            tools: ["mcp__memory__read_graph"]
          message: "Should call read_graph"
        - type: content_includes
          params:
            patterns: ["blue"]
          message: "Should include recalled information"
```

`tools_called` passes when every listed tool was called at least once in that turn. It matches the qualified name, so a bare `create_entities` fails.

## Step 5: Declare the MCP server

Create `config.arena.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: mcp-tools-test

spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant-with-tools.yaml

  providers:
    - file: providers/openai.yaml

  scenarios:
    - file: scenarios/tool-calling-test.yaml

  mcp_servers:
    - name: memory
      command: npx
      args: ["-y", "@modelcontextprotocol/server-memory"]

  defaults:
    temperature: 0.2
    max_tokens: 500
    output:
      dir: out
      formats: ["json", "markdown"]
```

`mcp_servers` is a list. Each entry has a `name` (the `<server>` in the tool names) and exactly one of `command`, `url` or `source`; Arena rejects an entry that sets none or more than one. Here `command` and its `args` start a local server. Arena starts the process over stdio and discovers its tools.

## Step 6: Run the test

```bash
promptarena run --simple --scenario basic-tool-calling
```

`--scenario` takes the scenario `id` (or `metadata.name` when there is no `id`), not its file name. Our scenario has no `id`, so it is `basic-tool-calling`. The first run downloads the server, so it takes longer. The run ends with a summary:

```text
Done  [████████████████████████]  1/1

Run Summary
────────────────────────────────────────────────────

Total runs      1
Passed          1 (100.0%)
Failed          0 (0.0%)
```

## Step 7: See the tool calls

Each run is saved as `out/<run-id>.json`. List the tool calls in it with `jq`:

```bash
jq '.Messages[] | select(.tool_calls != null) | .tool_calls[] | {name, args}' out/*_basic-tool-calling_*.json
```

The output has one object per call, with the qualified `name` and the `args` the model sent:

```text
{"name":"mcp__memory__create_entities","args":{"entities":[{"entityType":"UserPreference","name":"FavoriteColor","observations":["User's favorite color is blue."]}]}}
{"name":"mcp__memory__read_graph","args":{}}
```

The arguments vary from run to run. For the file layout, see [Output formats](/arena/reference/output-formats/).

## Common issues

### The model does not call a tool

Check that the prompt lists the tool under `allowed_tools`, using the `mcp__<server>__<tool>` form.

### `tools not called enough`

The assertion names a tool the model did not call, or used the bare tool name. Use the qualified name from Step 7.

### `scenario ... not found`

`--scenario` takes the scenario `id` (or `metadata.name` when there is no `id`). Use `basic-tool-calling`.

## Next steps

You have tested a model against a real MCP server.

- [Test MCP tools](/arena/how-to/agents/test-mcp-tools/): more MCP server configurations and troubleshooting.
- [Write Scenarios](/arena/how-to/scenarios/write-scenarios/): more assertion patterns.
- [Tutorial 5: CI Integration](/arena/tutorials/05-ci-integration/): run these tests on every commit.
