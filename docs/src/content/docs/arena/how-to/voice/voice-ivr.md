---
title: Test a Voice IVR with a Workflow State Machine
description: Drive a bank IVR through workflow state transitions and assert the tool-call pattern of each path, keyless against a mock provider.
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/engine/composite_conversation_executor.go
    - arena/engine/duplex_conversation_executor.go
    - arena/engine/execution_workflow_integration.go
    - arena/engine/workflow_state_resolver.go
    - arena/engine/workflow_tool_executor.go
    - arena/web/frontend/src/lib/arenaView.ts
    - arena/web/frontend/src/lib/atlasAdapter.ts
    - arena/web/workflow_graph.go
    - examples/voice-ivr/config.arena.yaml
    - examples/voice-ivr/mock-responses.yaml
    - examples/voice-ivr/providers/mock-provider.yaml
    - examples/voice-ivr/scenarios/balance-check.scenario.yaml
    - examples/voice-ivr/scenarios/handoff-to-agent.scenario.yaml
    - examples/voice-ivr/tools/check-balance.tool.yaml
    - examples/voice-ivr/tools/lookup-account.tool.yaml
    - examples/voice-ivr/tools/transfer-to-agent.tool.yaml
    - examples/voice-refund-demo/scenarios/anxious-delivery.scenario.yaml
---

This how-to walks through `examples/voice-ivr/`: a workflow-driven bank IVR that uses PromptKit's workflow state machine to route callers to a self-service balance lookup or a human-agent handoff. The demo runs deterministically against a mock provider, with no API keys. To run the same workflow over live voice, add a duplex provider and a `duplex:` block to each scenario (see [Switching to live voice](#switching-to-live-voice)).

## What it proves

The example checks that the agent verifies identity before discussing the account, reaches the right terminal state, and calls only the tools the current state permits:

- The `workflow` block in `config.arena.yaml` defines the IVR: an entry `verifying` state that branches via `ServeBalance` (self-service) or `EscalateToAgent` (human handoff) to terminal states.
- The agent under test calls `workflow__transition` to drive the machine. Each transition is deferred while the tools execute and committed at the end of the turn.
- The tools `lookup_account`, `check_balance` and `transfer_to_agent` are mock-backed handlers whose results feed back into the conversation.
- Conversation-level assertions (`tools_called`, `tools_not_called`) check the tool-call pattern per scenario. The balance scenario fails if the agent transfers to a human; the handoff scenario fails if the agent fetches a balance.

`workflow__transition` is recorded as a tool call, so the test sees every state transition.

## Run it

```bash
cd examples/voice-ivr
promptarena serve
```

`serve` starts a local server on port 8080 and prints the URL to open; add `--open` to open the browser. The UI loads both scenarios. Each message shows its tool calls inline, including `workflow__transition` and its result, and a workflow graph highlights the path the run took.

For headless / CI:

```bash
promptarena run --ci --formats json,markdown
open out/results.md
```

For the dev loop:

```bash
promptarena run
```

All three share the same config. The mock provider runs deterministically, so CI is stable.

## How the assertions work

Each scenario lives at `examples/voice-ivr/scenarios/*.scenario.yaml`. The pattern:

```yaml
conversation_assertions:
  - type: tools_called
    params:
      tool_names: ["lookup_account"]
      min_calls: 1
    message: "Agent must verify identity before serving account data"
  - type: tools_called
    params:
      tool_names: ["check_balance"]
      min_calls: 1
  - type: tools_not_called
    params:
      tool_names: ["transfer_to_agent"]
    message: "Agent must NOT transfer the caller — this is self-service"
```

The workflow definition in `config.arena.yaml` handles the state machine. The scenarios assert what the agent must, and must not, do along the way.

## CI gate

The mock-provider path runs without API keys, so it fits a fork-safe CI job. See [Run in CI](/arena/how-to/interfaces/run-in-ci/) for the general setup.

```yaml
# .github/workflows/voice-ivr.yml
name: Voice IVR

on:
  pull_request:
    paths:
      - 'examples/voice-ivr/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run voice-ivr scenarios
        working-directory: examples/voice-ivr
        run: ../../bin/promptarena run --ci --formats json
      - name: Upload report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: voice-ivr-report
          path: examples/voice-ivr/out/
```

## Switching to live voice

The shipped scenarios use text turns. Arena runs a scenario through the duplex voice path only when it has a `duplex:` block. To drive the same workflow through a duplex provider:

1. Add a duplex provider (e.g., `providers/openai-realtime.provider.yaml`) and register it in `config.arena.yaml` under `providers:`.
2. Add a `duplex:` block to each scenario (see `examples/voice-refund-demo/scenarios/*.yaml` for the shape).
3. Optionally, replace the scripted user turns with `role: selfplay-user` turns and personas, as `voice-refund-demo` does.
4. Run with provider keys in your environment.

The workflow definition, tools, prompts, and assertions stay the same; only the I/O layer changes.

## Extending

- To add another self-service path (recent transactions, transfer initiated, fraud alert), define a new terminal state in `config.arena.yaml`, add an `on_event:` mapping in `verifying`, and add a prompt and scenario.
- To add an intermediate state (multi-factor auth challenge, identity escalation), insert it between `verifying` and the terminal states. The `workflow_tool_access` assertion can constrain which tools each state permits.
- For stricter assertions, add `tool_call_sequence` to assert the order of tool calls (`lookup_account` before `check_balance`), or `tool_calls_with_args` to assert specific argument values.

## Related pages

- [Test a Voice Customer Support Agent with Self-Play](/arena/how-to/voice/voice-customer-support/)
- [Set Up Voice Testing with Self-Play](/arena/how-to/voice/setup-voice-testing/)
