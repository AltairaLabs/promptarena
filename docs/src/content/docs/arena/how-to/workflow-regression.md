---
title: Run workflow scenarios as a regression suite
description: Workflow state machines are first-class test subjects in PromptArena. Drive an agent through a stateful conversation, assert the transitions, and gate merges on the lifecycle reaching the expected end state.
---

This how-to walks through `examples/workflow-support/` and `examples/workflow-order-processing/` — two scenarios that use PromptKit's workflow primitive (state machine in the pack, `workflow__transition` tool the agent calls). Scenarios assert that the agent drives the expected lifecycle.

## What it proves

Workflow-driven agents fail in ways pure single-turn eval can't catch: the agent reaches the wrong terminal state, skips a required transition, or loops forever. Eval-only frameworks don't have a workflow concept at all — every demo of "agent state" ends up being a custom log scraper. PromptArena ships the state machine as a pack primitive and assertions can observe the transitions directly.

Two examples ship with the runtime:

- `examples/workflow-support/` — three-state support workflow (intake → specialist → closed) with two scenarios covering different paths.
- `examples/workflow-order-processing/` — four-state order lifecycle (new_order → payment → fulfillment → complete) with full and partial-flow scenarios.

## The assertion shape

Workflow scenarios drive a stateful agent across multiple turns. The agent calls `workflow__transition` to advance the state machine; state-aware assertions observe the machine's state directly, per turn. Four assertion types cover the lifecycle:

```yaml
turns:
  - role: user
    content: "I just placed an order. Can you confirm?"
    assertions:
      # A transition happened on this turn (the machine advanced into `payment`).
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
      # The full ordered path the agent drove, start to finish.
      - type: workflow_transition_order
        params:
          sequence: ["payment", "fulfillment", "complete"]
```

- `transitioned_to` — assert the machine advanced into a given state on this turn.
- `state_is` — assert the current state after the turn (useful when a turn should *not* transition).
- `workflow_complete` — assert the machine reached a terminal state.
- `workflow_transition_order` — assert the exact ordered sequence of states the agent drove.

These observe the workflow metadata directly, so they assert on actual state rather than on the tool call that drives it. All four pass deterministically against the mock provider's scripted `workflow__transition` responses.

## Running

```bash
cd examples/workflow-support
../../bin/promptarena run --ci --formats html,json
open out/report.html
```

The HTML report's timeline view shows each transition with its from/to states and the event that triggered it, alongside the assertion results for each turn.

Same shape for the order-processing example:

```bash
cd examples/workflow-order-processing
../../bin/promptarena run --ci --formats html,json
```

## What's a regression suite, then?

Three questions the assertions answer, each catching a distinct class of regression:

1. **Did the agent reach the right states?** — `transitioned_to` / `state_is` per turn. Catches "agent stopped driving the state machine" or "landed in the wrong state" regressions.
2. **In the right order?** — `workflow_transition_order` with the expected lifecycle. Catches "agent skipped a state" or "went backwards" regressions.
3. **Did it finish?** — `workflow_complete`. Catches "agent stalled before the terminal state" regressions.

Combine with content checks (`content_includes`, `content_excludes`) on each turn for "agent confirmed payment received" / "agent provided tracking number" assertions, and the suite gives a deterministic deploy gate for any prompt change that might break the workflow.

## CI gate

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
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: make build-arena
      - name: Run workflow scenarios
        run: |
          for dir in examples/workflow-support examples/workflow-order-processing; do
            (cd "$dir" && ../../bin/promptarena run --ci --formats json) || exit 1
          done
```

Keyless: both examples use mock providers with scripted `workflow__transition` calls.

## Unit-testing a single stage

The scenarios above drive the whole lifecycle from the entry state. To exercise
**one** stage in isolation — without first walking the agent through every earlier
transition — pin the scenario's `task_type` to that stage's `prompt_task`:

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

The per-run state machine starts in that stage instead of the workflow entry, so
the turn runs the `fulfillment` prompt directly. Full-lifecycle scenarios (no
`task_type`, or `task_type` set to the entry) and single-stage unit tests can live
in the same config. A `task_type` that names no workflow state's `prompt_task` is a
hard error at run time — it is never silently rerouted through the entry. See
`examples/workflow-order-processing` for both shapes side by side.

## Extending it

- **Add a new state**: drop it in the pack's `workflow:` block with `on_event:` mappings. Add a scenario that exercises the new path. Extend the `workflow_transition_order` sequence to include it.
- **Assert per-state tool access**: `workflow_tool_access` asserts that the agent only uses certain tools in certain states — useful for enforcing "no refund tool before the verification stage".
- **Multi-path scenarios**: register multiple scenarios that exercise different lifecycle branches (e.g., happy-path, escalation-path, error-path). The same state-aware assertions work per scenario.

## Related how-tos

- [Voice IVR with workflow state machine](/arena/how-to/voice-ivr/) — the same workflow primitive paired with the voice harness, asserted with the same `state_is` / `transitioned_to` / `workflow_complete` shape.
- The voice-ivr scenario is the reference for the workflow + voice pairing; the workflow-support / workflow-order-processing scenarios are the reference for the text-only workflow lifecycle pattern.
