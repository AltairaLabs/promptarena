---
title: Test Multi-Agent A2A Delegation
description: Assert which A2A agents an assistant invokes and what they return, using examples/multi-agent-demo/ with a research agent and a translation agent.
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - .github/workflows/ci.yml
    - Makefile
    - arena/agentkb/schemas/common/assertions.json
    - arena/arenaconfig/types.go
    - arena/engine/builder_integration.go
    - examples/multi-agent-demo/config.arena.yaml
    - examples/multi-agent-demo/mock-responses.yaml
    - examples/multi-agent-demo/prompts/assistant.yaml
    - examples/multi-agent-demo/providers/mock-provider.yaml
    - examples/multi-agent-demo/scenarios/multi-agent-delegation.yaml
    - scripts/test-examples.sh
---

Use `examples/multi-agent-demo/` to assert that an assistant calls the right A2A agents and that each agent returns the expected content.

## Prerequisites

- The `promptarena` binary.
- The `examples/multi-agent-demo/` directory. The default config needs no API key.

## What it covers

Multi-agent systems fail in two ways: the assistant delegates to the wrong specialist, or the right specialist returns unusable output. `agent_invoked` checks the first and `agent_response_contains` checks the second. See the [eval framework](/arena/explanation/eval-framework/) for how assertions fit into evaluation.

The demo config has one assistant prompt (`prompts/assistant.yaml`) and two mock A2A agents, `research_agent` and `translation_agent`. The `mock-assistant` provider scripts the tool calls from `mock-responses.yaml` by turn number: turn 1 calls `a2a__research_agent__search_papers` and turn 3 calls `a2a__translation_agent__translate`. With a real provider, the prompt decides which agent to call.

## 1. Assert delegation in the scenario

`examples/multi-agent-demo/scenarios/multi-agent-delegation.yaml`:

```yaml
turns:
  - role: user
    content: "Search for papers about quantum computing"
    assertions:
      - type: agent_invoked
        params:
          agents:
            - a2a__research_agent__search_papers
      - type: agent_response_contains
        params:
          agent: a2a__research_agent__search_papers
          contains: "Quantum Computing Fundamentals"

  - role: user
    content: "Translate the summary to French"
    assertions:
      - type: agent_invoked
        params:
          agents:
            - a2a__translation_agent__translate
      - type: agent_response_contains
        params:
          agent: a2a__translation_agent__translate
          contains: "informatique quantique"

conversation_assertions:
  - type: agent_invoked
    params:
      agents:
        - a2a__research_agent__search_papers
        - a2a__translation_agent__translate
```

Per-turn `agent_invoked` checks routing. Per-turn `agent_response_contains` checks specialist content. Conversation-level `agent_invoked` passes only when every listed agent was called during the run.

Tool names follow `a2a__<agent card name>__<skill id>`, with the card name lowercased and non-alphanumerics replaced by `_`. The card name `Research Agent` becomes `research_agent`.

## 2. Define the mock agents

`config.arena.yaml` defines each agent inline. A `responses` rule returns its `response` when the input matches:

```yaml
a2a_agents:
  - name: research_agent
    card:
      name: Research Agent
      description: A mock research agent that searches for academic papers
      skills:
        - id: search_papers
          name: Search Papers
          tags: [research, papers, search]
    responses:
      - skill: search_papers
        match:
          contains: quantum
        response:
          parts:
            - text: |
                Found 3 papers on quantum computing:
                1. Quantum Computing Fundamentals (2024)
                ...
```

A rule without `match` acts as the default response for its skill. Rules are evaluated in order and the first match wins. `contains` is case-insensitive and is matched against the tool's `query` argument. The mock server runs in the `promptarena` process, so no external server, Docker or provider key is needed.

## 3. Run it

1. Change into the example and start the web UI, which serves on port 8080:

   ```bash
   cd examples/multi-agent-demo
   promptarena serve
   ```

2. Or run headless, as CI does:

   ```bash
   promptarena run --ci --formats json,markdown
   ```

## 4. Verify

The headless run exits 0 and writes `out/results.md` and the JSON results. Every per-turn and conversation assertion passes.

## Switching to real A2A servers

1. Remove the inline `card` and `responses` from the `a2a_agents` entry.
2. Add `url:` with the server's endpoint. The runtime fetches the agent card from `/.well-known/agent-card.json`, and falls back to `/.well-known/agent.json` on a 404 or 405.
3. Keep the assertions in step with the real card. Tool names come from the card name and skill ids, so the card must sanitize to `research_agent` and `search_papers` (and so on), or you rename the tool names in the mock responses and scenario.

```yaml
a2a_agents:
  - name: research_agent
    url: https://research.example.com
```

The same entry accepts `auth`, `headers`, `headers_from_env`, `timeout_ms` and `skill_filter`. See [Test A2A Agents](/arena/how-to/agents/test-a2a-agents/) for authentication, headers and skill filtering.

## Adding agents and assertions

- Add a third specialist: add another `a2a_agents` entry in `config.arena.yaml` with its skills and responses, and add a turn that asserts on it.
- Assert ordering: `tool_call_sequence` over the `a2a__*` tool names checks the order in which the assistant called the agents.
- Assert arguments: A2A tools take a `query` string argument, so use `tool_calls_with_args` with `args_match` on `query` (for example a regex matching `French`) to check what the assistant sent to the translation agent.

## Run it in CI

The example runs in the `test-examples` job of `.github/workflows/ci.yml` through `make test-examples`, which calls `scripts/test-examples.sh`. That script runs `promptarena run --config config.arena.yaml --ci --formats json` from inside `examples/multi-agent-demo`.

To gate your own repository, adapt this sample workflow. It builds `promptarena` from source; replace that step with however you install the binary.

```yaml
# Sample: .github/workflows/multi-agent-demo.yml
name: A2A multi-agent

on:
  pull_request:
    paths:
      - 'examples/multi-agent-demo/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: '1.26.0'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run multi-agent demo
        working-directory: examples/multi-agent-demo
        run: ../../bin/promptarena run --ci --formats json
```

The job needs no secrets, so it runs on pull requests from forks.

## Related pages

- [Test A2A Agents](/arena/how-to/agents/test-a2a-agents/) covers real A2A servers, authentication and skill filtering.
- [Eval framework](/arena/explanation/eval-framework/) explains how evals and assertions relate.
