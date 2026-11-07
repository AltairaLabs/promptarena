---
title: Run Workflow Regression Tests
description: Drive a workflow agent through a stateful conversation, assert the state transitions, and gate merges on the lifecycle reaching its end state.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - .github/workflows/ci.yml
    - Makefile
    - arena/arenaconfig/types.go
    - arena/engine/conversation_executor.go
    - arena/engine/execution_workflow_integration.go
    - arena/engine/workflow_state_resolver.go
    - arena/engine/workflow_tool_executor.go
    - arena/redteam/generate.go
    - examples/voice-ivr/scenarios/balance-check.scenario.yaml
    - examples/workflow-agent-loops/scenarios/loop-converges.scenario.yaml
    - examples/workflow-agent-loops/scenarios/loop-hits-max-visits.scenario.yaml
    - examples/workflow-order-processing/config.arena.yaml
    - examples/workflow-order-processing/mock-responses.yaml
    - examples/workflow-order-processing/providers/mock-provider.yaml
    - examples/workflow-order-processing/scenarios/order-flow.scenario.yaml
    - examples/workflow-order-processing/scenarios/unit-fulfillment-stage.scenario.yaml
    - examples/workflow-support/config.arena.yaml
    - examples/workflow-support/mock-responses.yaml
    - examples/workflow-support/providers/mock-provider.yaml
    - examples/workflow-support/scenarios/basic-flow.scenario.yaml
    - examples/workflow-support/scenarios/state-assertions.scenario.yaml
---

Assert that an agent drives a PromptKit workflow state machine through the expected lifecycle, and fail CI when it does not. The steps use `examples/workflow-support/` (intake, specialist, closed; two scenarios) and `examples/workflow-order-processing/` (new_order, payment, fulfillment, complete; three scenarios). The agent advances the machine by calling the `workflow__transition` tool.

Workflow assertions read the state machine's own state, so they catch an agent that lands in the wrong state, skips a transition or stalls, without parsing logs or tool calls.

## Prerequisites

- A pack with a `workflow:` block and a scenario set that uses it.
- A built `promptarena` binary (the examples call `../../bin/promptarena`).

## Assert the lifecycle

A transition takes effect within the turn that made it. The destination state's prompt and tools are swapped in, and the state answers in the same turn. One user turn can therefore span several states.

Add these assertions to the turns of a scenario:

```yaml
turns:
  - role: user
    content: "I just placed an order. Can you confirm?"
    assertions:
      # The machine has advanced into `payment`.
      - type: transitioned_to
        params:
          state: payment

  - role: user
    content: "Has my payment gone through?"
    assertions:
      # The current state after this turn is `fulfillment`.
      - type: state_is
        params:
          state: fulfillment

  - role: user
    content: "I received the package. Thanks!"
    assertions:
      - type: transitioned_to
        params:
          state: complete
      # The machine reached a terminal state.
      - type: workflow_complete
        params: {}
      # The states the agent drove, in order.
      - type: workflow_transition_order
        params:
          sequence: ["payment", "fulfillment", "complete"]
```

| Assertion | Passes when |
|---|---|
| `transitioned_to` | Any transition recorded so far in the run entered the given state. The record accumulates across turns. |
| `state_is` | The current state after the turn is the given state. Use it for turns that must not transition. |
| `workflow_complete` | The machine reached a terminal state. |
| `workflow_transition_order` | `sequence` appears, in order, among the states the recorded transitions entered. Extra intermediate transitions are allowed. |
| `spoke_in_state` | The given state produced a non-blank assistant message. |
| `workflow_tool_access` | The agent called only the allowed tools in each state (`rules: [{state, allowed: [tools]}]`). States without a rule are unrestricted. |

Each class of regression maps to an assertion:

- Wrong or missing state: `transitioned_to`, `state_is`.
- Skipped or reversed state: `workflow_transition_order`.
- Stalled before the end: `workflow_complete`.
- Wrong tool in a state, such as a refund tool before verification: `workflow_tool_access`.

Add `content_includes` or `content_excludes` on each turn to check what the agent said.

### Asserting that a state spoke

The first four assertions read only the transition history. A handoff whose destination state produces no output passes all of them, and the suite stays green.

Pair `transitioned_to` with `spoke_in_state` wherever a handoff matters:

```yaml
conversation_assertions:
  - type: transitioned_to
    params: { state: handoff }     # the machine advanced
  - type: spoke_in_state
    params: { state: handoff }     # the destination replied
```

A scripted user turn after the transition gives the destination state somewhere to speak, so `spoke_in_state` passes even if the handoff itself fails. To test the handoff, run the scenario from a single opening turn, as `examples/workflow-agent-loops/` does.

## Run the scenarios

Run each example from its own directory:

```bash
cd examples/workflow-support
../../bin/promptarena run --ci --formats json,markdown
open out/results.md
```

```bash
cd examples/workflow-order-processing
../../bin/promptarena run --ci --formats json,markdown
```

`out/results.md` is an overview table with one row per run: provider, scenario, status, duration, turns, assertion pass count and tool-call count. Per-assertion results and transitions are in the JSON result files in `out/`. To see the transitions as a state graph, run `promptarena serve` and open a run's Workflow tab, which has Live and Path modes.

## Test a single stage

To exercise one stage without walking the agent through every earlier transition, set the scenario's `task_type` to that stage's `prompt_task`:

```yaml
spec:
  id: unit-fulfillment-stage
  task_type: fulfillment   # start in the non-entry 'fulfillment' stage
  turns:
    - role: user
      content: "Where is my package?"
      assertions:
        - type: state_is
          params:
            state: fulfillment
```

The per-run state machine starts in that stage, so the turn runs its prompt directly. Full-lifecycle scenarios and single-stage tests can share one config; `examples/workflow-order-processing` has both. A `task_type` that matches no workflow state's `prompt_task`, or matches several non-entry states, is a hard error at run time.

## Gate merges in CI

Add this workflow to your repository. Both examples use mock providers with scripted `workflow__transition` calls, so the job needs no API keys.

```yaml
# .github/workflows/workflow-regression.yml
name: Workflow regression

on:
  pull_request:
    paths:
      - 'examples/workflow-support/**'
      - 'examples/workflow-order-processing/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run workflow scenarios
        run: |
          for dir in examples/workflow-support examples/workflow-order-processing; do
            (cd "$dir" && ../../bin/promptarena run --ci --formats json) || exit 1
          done
```

## Extend the suite

- Add a state: put it in the pack's `workflow:` block with `on_event:` mappings, add a scenario that exercises the path, and extend the `workflow_transition_order` sequence.
- Cover each lifecycle branch (happy path, escalation, error) with its own scenario. The same assertions apply to each.

## Related pages

- [Voice IVR with workflow state machine](/arena/how-to/voice/voice-ivr/): the same workflow primitive with the voice harness, asserted with `state_is`, `transitioned_to` and `workflow_complete`.
