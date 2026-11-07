---
title: Test A2A Agents
description: Define mock or remote A2A agents in an Arena config and assert on agent delegation in scenarios.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - arena/agentkb/evalmeta.yaml
    - arena/arenaconfig/types.go
    - arena/arenaconfig/validator.go
    - arena/cmd/promptarena/run.go
    - arena/engine/builder_integration.go
    - arena/engine/execution.go
    - arena/stages/assertions.go
    - examples/multi-agent-demo/config.arena.yaml
    - examples/multi-agent-demo/mock-responses.yaml
    - examples/multi-agent-demo/prompts/assistant.yaml
    - examples/multi-agent-demo/providers/mock-provider.yaml
    - examples/multi-agent-demo/scenarios/multi-agent-delegation.yaml
---

Test agent-to-agent delegation with mock A2A agents defined in your Arena config. Write a scenario that triggers a tool call and assert on the delegation.

## Prerequisites

- The `promptarena` binary.
- A mock provider whose `mock_config` points at a mock responses file. `examples/multi-agent-demo/` contains a working setup.

## 1. Configure A2A agents

Add an `a2a_agents` block to your Arena config. Arena starts a mock A2A server for each entry and registers each skill as a tool through the Tool Bridge.

```yaml
# config.arena.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: a2a-demo
spec:
  providers:
    - file: providers/mock-provider.yaml
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml
  scenarios:
    - file: scenarios/delegated_research.yaml

  a2a_agents:
    - name: research_agent
      card:
        name: Research Agent
        description: A mock research agent that returns papers
        skills:
          - id: search_papers
            name: Search Papers
            description: Search for academic papers on a given topic
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
                  2. Advances in Quantum Error Correction (2024)
                  3. Practical Quantum Algorithms for Optimization (2023)
        - skill: search_papers
          response:
            parts:
              - text: |
                  Found 1 paper on the requested topic:
                  1. General Survey of the Field (2024)

  defaults:
    temperature: 0.5
    max_tokens: 1000
    seed: 42
    concurrency: 1
    output:
      dir: out
      formats: ["json", "markdown"]
```

Each skill becomes a tool named `a2a__<card name>__<skill id>`. Arena lowercases both parts and replaces runs of non-alphanumeric characters with `_`. Here `Research Agent` and `search_papers` give `a2a__research_agent__search_papers`. The `name` field on the agent entry does not appear in the tool name.

Rules in `responses` are checked in order. A rule without `match` answers when no earlier rule matched. See the [config schema](/arena/reference/config-schema/) for every `a2a_agents` and `match` field.

## 2. Write the prompt config

The prompt config needs `metadata.name`:

```yaml
# prompts/assistant.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: assistant
spec:
  task_type: assistant
  version: v1.0.0
  description: An assistant that can delegate research to A2A agents
  system_template: |
    You are a helpful assistant with access to research tools.
    When asked to search for papers or research a topic, use the available tools.
    Summarize the results for the user.
```

`a2a__` tools are offered to every prompt without an `allowed_tools` entry. `allowed_tools` gates tools declared in a pack, such as MCP tools.

:::note[Check for a real tool call]
If the system prompt promises tools the model does not have, the model often writes convincing tool calls and results into its reply as plain text. Check for a real `tool` role in the messages, not tool-shaped text in `content`.
:::

## 3. Script the mock LLM

Configure the mock LLM to issue a tool call, then summarize the result. The scenario key matches the scenario's `metadata.name`.

```yaml
# mock-responses.yaml
scenarios:
  delegated-research:
    turns:
      1:
        tool_calls:
          - name: "a2a__research_agent__search_papers"
            arguments:
              query: "Search for papers about quantum computing"
      2:
        response: |
          Based on the research results, I found 3 papers on quantum computing:
          1. Quantum Computing Fundamentals (2024)
          2. Advances in Quantum Error Correction (2024)
          3. Practical Quantum Algorithms for Optimization (2023)
```

In turn 1 the mock LLM emits the tool call. Arena routes it to the mock A2A server, which matches the `quantum` rule and returns the canned response. In turn 2 the mock LLM sees the tool result and produces the summary.

## 4. Write the scenario

```yaml
# scenarios/delegated_research.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: delegated-research
spec:
  id: delegated-research
  task_type: assistant
  description: Test delegation of research queries to an A2A agent
  tool_policy:
    tool_choice: auto
    max_tool_calls_per_turn: 3
  turns:
    - role: user
      content: "Search for papers about quantum computing"
      assertions:
        - type: tools_called
          params:
            tools:
              - a2a__research_agent__search_papers
        - type: content_includes
          params:
            patterns:
              - "Quantum Computing Fundamentals"
```

`agent_invoked`, `agent_not_invoked` and `agent_response_contains` express delegation checks directly. They match on tool names, like `tools_called`. See the [assertions reference](/arena/reference/assertions/) for every assertion type.

```yaml
assertions:
  - type: agent_invoked
    params:
      agents:
        - a2a__research_agent__search_papers
  - type: agent_response_contains
    params:
      agent: a2a__research_agent__search_papers
      contains: "Quantum Computing Fundamentals"
```

## 5. Run and verify

From `examples/multi-agent-demo/`:

```bash
promptarena run -c config.arena.yaml
```

Arena starts the mock A2A servers, registers the skills as tools, runs each scenario turn with the mock LLM, routes tool calls to the mock server and evaluates the assertions. Results go to `out/`, relative to where you run: `results.md`, `index.json` and one JSON file per run.

## Test several agents

Add more entries to `a2a_agents`. This registers `a2a__research_agent__search_papers` and `a2a__translation_agent__translate`:

```yaml
a2a_agents:
  - name: research_agent
    card:
      name: Research Agent
      skills:
        - id: search_papers
          name: Search Papers
    responses:
      - skill: search_papers
        response:
          parts:
            - text: "Research results..."

  - name: translation_agent
    card:
      name: Translation Agent
      skills:
        - id: translate
          name: Translate
    responses:
      - skill: translate
        response:
          parts:
            - text: "Translated text..."
```

Check which agents a turn delegates to. `message` is a sibling of `type` and `params`:

```yaml
turns:
  - role: user
    content: "Research quantum computing and translate the summary to French"
    assertions:
      - type: agent_invoked
        params:
          agents:
            - a2a__research_agent__search_papers
            - a2a__translation_agent__translate
        message: "Should delegate to both agents"
      - type: agent_not_invoked
        params:
          agents:
            - a2a__admin_agent__execute
        message: "Should not invoke admin agent"
      - type: agent_response_contains
        params:
          agent: a2a__research_agent__search_papers
          contains: "Research results"
        message: "Research agent should return results"
```

For conversation-wide checks, use `conversation_assertions`:

```yaml
conversation_assertions:
  - type: agent_invoked
    params:
      agents:
        - a2a__research_agent__search_papers
    message: "Research agent should be invoked at least once across the conversation"
  - type: agent_not_invoked
    params:
      agents:
        - a2a__admin_agent__execute
    message: "Admin agent should never be invoked"
```

## Return an error from a mock agent

A rule with `error` returns a failed task:

```yaml
responses:
  - skill: search_papers
    error: "Service temporarily unavailable"
```

## Test a remote A2A agent

Set `url` on the agent entry to test a real A2A server. Arena fetches the agent card from `/.well-known/agent-card.json`, falling back to `/.well-known/agent.json` when the first returns 404 or 405, and registers the skills it finds. `card` and `responses` are ignored.

```yaml
a2a_agents:
  - name: echo_agent
    url: "http://localhost:9877"
    timeout_ms: 5000
    auth:
      scheme: Bearer
      token_env: AGENT_TOKEN
    headers:
      X-Tenant-ID: acme
    headers_from_env:
      - "X-API-Key=API_KEY_ENV"
    skill_filter:
      allowlist:
        - echo
```

| Field | Effect |
|-------|--------|
| `auth.scheme` | Authorization scheme, such as `Bearer`. |
| `auth.token` | Inline token. |
| `auth.token_env` | Environment variable holding the token. Keeps tokens out of config files. |
| `headers` | Static headers sent with every request. |
| `headers_from_env` | Entries of the form `Header-Name=ENV_VAR_NAME`. An unset variable skips the header. |
| `skill_filter.allowlist` | Expose only the listed skill IDs. |
| `skill_filter.blocklist` | Hide the listed skill IDs. |
| `timeout_ms` | Per-request timeout in milliseconds. |

## Related pages

- [A2A concept](https://promptkit.altairalabs.ai/concepts/a2a/) explains the A2A protocol.
- [Use the mock server](https://promptkit.altairalabs.ai/runtime/how-to/use-a2a-mock-server/) covers the mock in Go tests.
- [Write Scenarios](/arena/how-to/scenarios/write-scenarios/) is the general scenario guide.
- [Assertions reference](/arena/reference/assertions/) lists every assertion type.
