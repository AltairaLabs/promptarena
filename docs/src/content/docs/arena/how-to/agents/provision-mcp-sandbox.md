---
title: Provision an MCP Sandbox per Scenario
description: Open an MCP server (for example a containerised codegen sandbox) on demand at run, scenario or session scope.
sidebar:
  order: 13
verified:
  commit: 72f35dcda537f9ebff0eaf33808ce728f0c8b3f8
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/mcp_servers.go
    - arena/arenaconfig/types.go
    - arena/arenaconfig/validator.go
    - arena/cmd/promptarena/run.go
    - arena/engine/builder_integration.go
    - arena/engine/engine.go
    - arena/engine/execution.go
    - arena/engine/execution_mcpsource.go
    - arena/engine/mcp_tool_descriptor.go
    - arena/engine/mcpsource_scope.go
    - arena/mcpsource/docker/cli.go
    - arena/mcpsource/docker/cli_interactive.go
    - arena/mcpsource/docker/register/register.go
    - arena/mcpsource/docker/source.go
    - arena/mcpsource/registry.go
    - arena/mcpsource/source.go
    - arena/results/markdown/markdown_repository.go
    - examples/codegen-sandbox/config.arena.yaml
    - examples/codegen-sandbox/mock-responses.yaml
    - examples/codegen-sandbox/prompts/codegen-agent.yaml
    - examples/codegen-sandbox/scenarios/simple-fix.scenario.yaml
---

Open an MCP server per run, scenario or session (a codegen sandbox, for example) through an MCPSource, and have Arena tear it down when that scope closes. Source-backed entries are the third MCP transport, alongside stdio (`command`) and static HTTP+SSE (`url`). For long-lived shared servers, use [Test MCP Tools](/arena/how-to/agents/test-mcp-tools/).

## Prerequisites

- Docker on the host running `promptarena`.
- An image that serves an MCP HTTP+SSE endpoint on port 8080.

## 1. Declare the entry

```yaml
spec:
  mcp_servers:
    - name: sandbox            # MCP registry key; also the key in sandbox_* metadata
      source: docker           # name of a registered MCPSource
      scope: session           # when to open and close
      source_args:             # opaque, source-specific
        image: ghcr.io/altairalabs/codegen-sandbox:latest
        repo: https://github.com/example/some-project
        branch: main
        env:
          DEV_MODE: "1"
```

| Field | Purpose |
| --- | --- |
| `source` | Name of a registered `MCPSource` (e.g. `docker`). |
| `scope` | `run`, `scenario` or `session`. |
| `source_args` | Free-form map handed to the source's `Open()`. |

Give every entry a `name` and set exactly one of `source`, `url` and `command`. A `source` entry also needs a `scope` of `run`, `scenario` or `session`. A config that breaks either rule fails to load, in `promptarena validate` and `promptarena run` alike.

## 2. Choose a scope

| Scope | Opens | Closes | Use for |
| --- | --- | --- | --- |
| `run` | Once at arena startup | At arena shutdown | Infrastructure shared across all tests. |
| `scenario` | Each run, before session sources | Each run, after session sources | Per-scenario fixtures. |
| `session` | Each run (each repetition) | After that run's assertions | Sandboxes that must be fresh per agent run. |

Scenario scope opens once per run (each repetition, provider and region combination), so it behaves like session scope apart from ordering. Inner scopes close before outer scopes. Closer errors are logged as warnings. If `Open()` fails partway through a scope's entries, the entries already opened in that scope are torn down before the error propagates.

## 3. Template from scenario variables

`source_args` is expanded against each scenario's `variables` block before the source sees it. `{{scenario.<key>}}` is the only supported form: no fallbacks, no expressions. Expansion applies at `scenario` and `session` scope.

```yaml
# scenario
variables:
  repo: https://github.com/example/foo
  branch: feature-x

# arena config, under the entry's source_args
repo: "{{scenario.repo}}"
branch: "{{scenario.branch}}"
```

Each session opens a fresh container with that scenario's repo cloned into `/workspace`.

## 4. Allow the tools

When `Open()` returns, Arena registers the URL and headers in the runtime MCP registry under the entry's `name`, calls `tools/list`, and registers each discovered tool in the tools registry under its raw MCP name (`Read`, `Edit`, ...). The names are not qualified with `mcp__<server>__`, so `name` does not appear in them.

List the tools in the prompt config's `allowed_tools`:

```yaml
allowed_tools:
  - Read
  - Edit
  - Bash
  - run_tests
```

:::note[allowed_tools is an allowlist]
A prompt only reaches the tools it names here. With no entry, the model has no tools and usually improvises, writing convincing-looking tool calls and results into its reply as plain text. Check for a real `tool` role in the messages, not tool-shaped text in `content`.
:::

If two source-backed servers expose a tool with the same name, the second registration overwrites the first. Stdio and url entries are namespaced as `mcp__<server>__<tool>`.

## The `docker` source

PromptArena registers a `docker` source automatically. It drives the container with the local `docker` CLI.

| Arg | Type | Default | Notes |
| --- | --- | --- | --- |
| `image` | string, required |  | Image reference. The container must serve MCP HTTP+SSE on port 8080. |
| `repo` | string |  | Cloned with `docker exec <cid> git clone [--branch <branch>] <repo> /workspace`. |
| `branch` | string | repo default | Ignored when `repo` is empty. |
| `env` | map |  | Passed via `-e`. Numbers and booleans are passed as text (`DEV_MODE: 1` becomes `DEV_MODE=1`); a list or map value fails the open. |
| `mounts` | list of objects |  | Bind mounts with `source`, `target` and `readonly`. |
| `ports` | list of ints |  | Extra container ports, each published on a free host port. `8080` is rejected. |
| `network` | string | Docker default | Passed to `--network`. `none` and `host` are rejected. |
| `cpus` | string or number | unlimited | Passed to `--cpus`. |
| `memory` | string | unlimited | Passed to `--memory` (for example `2g`). |

The source publishes the container's `8080` on a free local port, polls `http://localhost:<port>/sse` until it returns 2xx (20s budget), then returns that `/sse` URL as the server's endpoint. On close it stops and removes the container.

The clone is unauthenticated, so a private repo needs credentials baked into the image. See [Configuration Schema](/arena/reference/config-schema/#mcp_servers) for the other `mcp_servers` fields.

### Reach services the agent starts

A web service the agent starts inside the sandbox is reachable from the host only if its port is listed under `ports`:

```yaml
source_args:
  image: ghcr.io/altairalabs/codegen-sandbox:latest
  ports: [3000]
```

Session hooks receive the mapping as `sandbox_ports` in the event metadata, next to `sandbox_containers`:

```json
{"sandbox_containers": {"sandbox": "<cid>"},
 "sandbox_ports": {"sandbox": {"3000": "http://localhost:54321"}}}
```

A `session_end` hook can point a scanner or load generator at that URL from the host. To restrict egress, put the container on a user-defined bridge network routed through a filtering proxy.

## 5. Mount skills

For every source-backed entry, Arena adds a read-only mount to `source_args.mounts` for each skill source with a `path`:

- A skill directory (one holding `SKILL.md`) mounts at `/skills/<name>`. `<name>` is the source's `name:`, or the directory's base name when there is none, so `path: skills/codegen` mounts at `/skills/codegen`.
- A directory of skills mounts each skill inside it at `/skills/<skill-directory>`.

Inline skills get no mount. Mounts you set on the entry yourself are kept, and come first. When one of them already uses a `/skills/<name>` target, the skill mount for that target is dropped.

Each skill mount has `source` (absolute host path of the skill directory), `target` and `readonly: true`. The docker source passes it as `-v <src>:<tgt>:ro`, so skill scripts run in the sandbox as `Bash /skills/<name>/scripts/<script>`.

## 6. Gate on a sandbox tool

The [`tool_exec` check](https://promptkit.altairalabs.ai/reference/checks/#tool-invocation-checks) invokes a registered tool at the end of the session and asserts the call succeeded. Codegen sandboxes typically expose `run_tests`, `run_lint` and `run_typecheck`, so the check gates the run on whether the agent's edits pass.

Add the assertion to a scenario that uses the `session`-scoped sandbox from step 1:

```yaml
spec:
  conversation_assertions:
    - type: tool_exec
      params:
        tool: run_tests
      message: "Hidden test suite must pass"
```

The session-scoped container stays alive across the agent's tool calls and the gate. The gate runs `run_tests` once more after the agent finishes.

## Verify

[`examples/codegen-sandbox/`](https://github.com/AltairaLabs/promptarena/tree/main/examples/codegen-sandbox) is a runnable end-to-end demo. It provisions the sandbox per session and runs a mock-LLM scenario that seeds a buggy Go module, edits it, and verifies via `run_tests`.

```bash
go build -o bin/promptarena ./arena/cmd/promptarena
(cd examples/codegen-sandbox && ../../bin/promptarena run --ci --format markdown)
open examples/codegen-sandbox/out/results.md
```

`--mock-provider` swaps in the canned LLM responses but still opens the Docker sandbox, so Docker is required either way.

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| `unknown source "X"` | The source is not registered in the running binary. The error lists the registered names and appears when the source opens: at engine construction for `run` scope, as a failed run for `scenario` and `session`. |
| `container not healthy: health timeout after 20s` | The image is not serving HTTP+SSE on port 8080, or `/sse` returns non-2xx. |
| `tool <name> validation error (args_invalid)` | Tool call args do not match the MCP server's input schema. |
| `tool not found: <name>` after a successful open | `tools/list` returned nothing for that server, or two source-backed servers collided. |

## Related pages

- [Test MCP Tools](/arena/how-to/agents/test-mcp-tools/): static stdio and url MCP servers.
- [Configuration Schema](/arena/reference/config-schema/#mcp_servers): full field reference.
- [Write Scenarios](/arena/how-to/scenarios/write-scenarios/): the `variables:` block.
- [Integrate MCP (Runtime)](https://promptkit.altairalabs.ai/runtime/how-to/integrate-mcp/): low-level MCP registry API.
