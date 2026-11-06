---
title: Test MCP Tools
description: Connect stdio and HTTP MCP servers to an Arena config, filter their tools, and assert on the tool calls a scenario makes.
sidebar:
  order: 12
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - arena/arenaconfig/validator.go
    - arena/arenaconfig/validator_tools.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/engine/builder_integration.go
    - arena/engine/engine.go
    - arena/engine/execution.go
    - arena/mcp/config.go
    - examples/mcp-everything-test/config.arena.yaml
    - examples/mcp-everything-test/mock-responses.yaml
    - examples/mcp-everything-test/prompts/mcp-assistant.yaml
    - examples/mcp-everything-test/providers/mock-provider.yaml
    - examples/mcp-everything-test/scenarios/echo-and-add.scenario.yaml
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/promptconfig.json
    - schemas/v1alpha1/scenario.json
---

Connect real MCP servers to an Arena config and assert on the tool calls your scenarios make.

## Prerequisites

- The `promptarena` binary.
- Node.js with `npx`, for the `@modelcontextprotocol/server-everything` server used below.

This guide covers stdio servers (`command`) and static HTTP servers (`url`): long-lived processes shared across all scenarios. A `url` server uses Streamable HTTP, and falls back to HTTP+SSE when the server hosts only SSE. Set `transport` (`stdio`, `sse` or `streamable_http`) to choose explicitly. For per-scenario or per-session containers, such as codegen sandboxes, see [Provision an MCP Sandbox per Scenario](/arena/how-to/agents/provision-mcp-sandbox/).

## 1. Configure MCP servers

Add an `mcp_servers` block to your Arena config:

```yaml
# config.arena.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: mcp-test
spec:
  mcp_servers:
    - name: everything
      command: npx
      args:
        - "-y"
        - "@modelcontextprotocol/server-everything"

  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml
  scenarios:
    - file: scenarios/echo-test.yaml
  providers:
    - file: providers/mock-provider.yaml
```

Arena starts the MCP server, discovers its tools, and registers them for use in scenarios.

## 2. Filter tools

Use `tool_filter` to limit which of a server's tools Arena registers.

### Allowlist

```yaml
mcp_servers:
  - name: everything
    command: npx
    args: ["-y", "@modelcontextprotocol/server-everything"]
    tool_filter:
      allowlist:
        - echo
        - get-sum
```

### Blocklist

In this example, `mcp_db_server.py` stands for your own server:

```yaml
mcp_servers:
  - name: database
    command: python
    args: ["mcp_db_server.py"]
    tool_filter:
      blocklist:
        - drop_table
        - truncate_table
```

## 3. Set server options

### Environment variables

The server process inherits the environment of the shell that runs `promptarena`, so a token you export there reaches the server. Use `env` to set extra variables. Values pass to the server literally: `${VAR}` is not expanded.

```yaml
mcp_servers:
  - name: github
    command: npx
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      LOG_LEVEL: "debug"
```

### Working directory

```yaml
mcp_servers:
  - name: filesystem
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "./data"]
    working_dir: /path/to/project
```

### Timeout

Set a per-request timeout in milliseconds:

```yaml
mcp_servers:
  - name: everything
    command: npx
    args: ["-y", "@modelcontextprotocol/server-everything"]
    timeout_ms: 10000
```

## 4. Write the prompt config

Enable MCP tools in your prompt config with `allowed_tools`. MCP tools follow the naming pattern `mcp__{serverName}__{toolName}`:

:::note[allowed_tools is an allowlist]
A prompt reaches only the tools it names here. Keep the list in step with your system prompt. If the prompt says the model can look things up and the allowlist is empty, the model usually improvises, writing tool-shaped calls and results into its reply as plain text. Check the messages for a real `tool` role.
:::

```yaml
# prompts/assistant.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
spec:
  task_type: assistant
  version: v1.0.0
  description: Assistant with MCP tools
  allowed_tools:
    - mcp__everything__echo
    - mcp__everything__get-sum
  system_template: |
    You are a helpful assistant with access to echo and math tools.
```

## 5. Script the mock LLM

Script the mock LLM to issue tool calls against MCP tools:

```yaml
# mock-responses.yaml
scenarios:
  echo-test:
    turns:
      1:
        tool_calls:
          - name: "mcp__everything__echo"
            arguments:
              message: "Hello from Arena!"
      2:
        response: "The echo tool returned: Hello from Arena!"
```

## 6. Write the scenario

```yaml
# scenarios/echo-test.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: echo-test
spec:
  id: echo-test
  task_type: assistant
  description: Test MCP echo tool
  tool_policy:
    tool_choice: auto
    max_tool_calls_per_turn: 3
  turns:
    - role: user
      content: "Echo the message: Hello from Arena!"
      assertions:
        - type: tools_called
          params:
            tools:
              - mcp__everything__echo
        - type: content_includes
          params:
            patterns:
              - "Hello from Arena"
```

## Use several servers

Register several servers in the same config:

```yaml
mcp_servers:
  - name: everything
    command: npx
    args: ["-y", "@modelcontextprotocol/server-everything"]
    tool_filter:
      allowlist: [echo, get-sum]

  - name: filesystem
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "./data"]
    tool_filter:
      allowlist: [read_file, list_directory]
```

## Verify

`examples/mcp-everything-test/` combines these steps:

```yaml
# config.arena.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: mcp-everything-test
spec:
  mcp_servers:
    - name: everything
      command: npx
      args: ["-y", "@modelcontextprotocol/server-everything"]
      timeout_ms: 10000
      tool_filter:
        allowlist: [echo, get-sum]

  prompt_configs:
    - id: mcp-assistant
      file: prompts/mcp-assistant.yaml
  scenarios:
    - file: scenarios/echo-and-add.scenario.yaml
  providers:
    - file: providers/mock-provider.yaml
  defaults:
    temperature: 0.7
    max_tokens: 500
    concurrency: 1
    output:
      dir: out
      formats: ["json", "markdown"]
```

Run it from the repository root:

```bash
cd examples/mcp-everything-test
promptarena run -c config.arena.yaml
```

The run passes with no API key: the mock LLM calls `echo` and `get-sum`, the tools execute against the real server, and the results land in `out/`.

## Related pages

- [Provision an MCP Sandbox per Scenario](/arena/how-to/agents/provision-mcp-sandbox/): host-managed servers with run / scenario / session lifecycle
- [Configure MCP Servers (SDK)](https://promptkit.altairalabs.ai/sdk/how-to/configure-mcp/): use MCP servers in the Go SDK
- [Integrate MCP (Runtime)](https://promptkit.altairalabs.ai/runtime/how-to/integrate-mcp/): low-level MCP registry API
- [Test A2A Agents](/arena/how-to/agents/test-a2a-agents/): test agent-to-agent delegation
- [Write Scenarios](/arena/how-to/scenarios/write-scenarios/): general scenario authoring
