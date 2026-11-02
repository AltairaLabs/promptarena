---
title: Test How an Agent Handles Tool Failures
description: Use a turn's chaos block to make tool calls fail, then assert that the agent recovers or reports the failure instead of inventing a result.
---

An agent that works when every tool answers can still fall apart when one doesn't. It might invent an order status, retry forever, or tell the user something false. A turn's `chaos:` block makes calls to a named tool fail during that turn, so you can test what the agent does next.

## Fail a tool on one turn

Add `chaos` to the turn whose tool calls should fail:

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

Your `message` appears word for word after that prefix. Write it the way the real service would fail, because that text is all the model has to go on.

## Assert on the recovery

Don't assert that the tool was called. A blocked call doesn't count for `tools_called`. Assert on what the agent tells the user instead:

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

To check that the failure happened at all, `no_tool_errors` fails on a blocked call, and the tool result message carries `chaos_injected: true` in its metadata.

## Fail some calls, not all

Set `probability` to fail a share of calls rather than every one. Each call is rolled separately. Pair it with `trials` and `pass_threshold` so a single unlucky run doesn't decide the result:

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

## Limits

- **`timeout` is not a real timeout.** It blocks the call at once, like `error`, with different default text and metadata.
- **`slow` adds a fixed 2-second delay** and then runs the tool normally.
- **Duplex turns ignore `chaos`.**
- **Results can't be rewritten.** A tool hook can block a call or delay it, but not replace a real tool's result. To feed the agent a specific tool result, use a mock tool's `mock_result` or `mock_template`.

See the [`chaos` reference](/arena/reference/config-schema/#chaos) for every field and default.
