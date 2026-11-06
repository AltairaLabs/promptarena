---
title: Test How an Agent Handles Tool Failures
description: Make a tool fail on one turn with a chaos block, then assert that the agent reports the failure instead of inventing a result.
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - arena/arenaconfig/types.go
    - arena/chaos/hook.go
    - arena/engine/conversation_executor.go
    - arena/toolresult/hook.go
    - arena/turnexecutors/pipeline_stages_integration.go
    - schemas/v1alpha1/tool.json
---

Make calls to a named tool fail during one turn, and check that the agent tells the user rather than inventing an answer.

## Prerequisites

- A scenario whose agent calls a tool, such as a mock tool. See [Write Test Scenarios](/arena/how-to/scenarios/write-scenarios/).
- The tool's name exactly as the agent calls it.

## Steps

1. Add `chaos` to the turn whose tool calls should fail:

   ```yaml
   turns:
     - role: user
       content: "Look up order ORD-12345 for me"
       chaos:
         tool_failures:
           - tool: lookup_order
             mode: error
             message: "Order service unavailable (503). Retry later."
   ```

   The call to `lookup_order` never runs. The model receives this as the tool result:

   ```text
   Tool lookup_order blocked by hook: Order service unavailable (503). Retry later.
   ```

   Your `message` appears word for word after that prefix. Write it the way the real service fails, because that text is all the model has to go on.

2. Assert on what the agent tells the user. A blocked call does not count for `tools_called`, so do not assert that the tool was called:

   ```yaml
       assertions:
         - type: content_includes_any
           params:
             patterns: ["unavailable", "try again", "later"]
           message: "Agent should tell the user the lookup failed"
         - type: content_excludes
           params:
             patterns: ["has shipped", "is being processed"]
           message: "Agent must not invent an order status"
   ```

3. To fail a share of calls instead of every one, set `probability`. Each call is rolled separately. Pair it with `trials` and `pass_threshold` so one unlucky run does not decide the result:

   ```yaml
   spec:
     trials: 10
     turns:
       - role: user
         content: "Look up order ORD-12345 for me"
         chaos:
           tool_failures:
             - tool: lookup_order
               mode: error
               probability: 0.3
         assertions:
           - type: content_excludes
             params:
               patterns: ["has shipped"]
             pass_threshold: 1.0   # must never invent a status
   ```

4. Run the scenario with `promptarena run --scenario <name>`.

## Verify

`no_tool_errors` fails on a blocked call, so adding it shows that the failure happened. The blocked call's tool result message carries `chaos_injected: true` in its metadata.

## Limits

- `mode: timeout` blocks the call at once, like `error`, with different default text and metadata. It does not wait.
- `mode: slow` adds a fixed 2-second delay, then runs the tool normally.
- Duplex turns ignore `chaos`.
- `chaos` blocks or delays a call. To rewrite a real tool's result, use the scenario's [`tool_result_transforms`](/arena/reference/config-schema/#tool-result-transforms). To feed the agent a fixed result, use a mock tool's `mock_result` or `mock_template`.

## Related pages

- [`chaos` in the configuration schema](/arena/reference/config-schema/#chaos), for every field and default
- [Validate Outputs](/arena/how-to/scenarios/validate-outputs/)
