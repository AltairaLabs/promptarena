# MCP Memory Server Test

Arena starts the official `@modelcontextprotocol/server-memory` server over
stdio, discovers its tools, and checks that the model stores and recalls a
fact through them.

## What this tests

- **Server start**: Arena spawns the server with `npx` and speaks MCP over stdio.
- **Tool discovery**: the server's nine knowledge-graph tools are registered as
  `mcp__memory__<tool>` (`mcp__memory__create_entities`,
  `mcp__memory__read_graph`, and so on).
- **Tool execution**: tool calls go to the server and its results, including
  `structuredContent`, come back to the model.

## Prerequisites

- Node.js, so that `npx` is available.
- For a live run, an OpenAI API key in `OPENAI_API_KEY`.
- The `promptarena` binary: `make -C ../.. build-go`.

## Run it

Without an API key, the mock provider plays the model. It calls
`create_entities` in the first turn and `read_graph` in the second, and the
calls run against the real server:

```bash
cd examples/mcp-memory-test
../../bin/promptarena run --mock-provider --mock-config mock-responses.yaml --ci
```

With OpenAI:

```bash
export OPENAI_API_KEY="your-api-key"
../../bin/promptarena run --ci
```

The scenario passes when turn 1 calls `mcp__memory__create_entities`, turn 2
calls `mcp__memory__read_graph`, and both replies mention Alice and her job.

## Troubleshooting

- **Server does not start**: check `node --version`, and try the server by hand
  with `npx -y @modelcontextprotocol/server-memory`.
- **`tools not called enough`**: `tools_called` matches the qualified name, so
  list `mcp__memory__create_entities`, not `create_entities`.
