---
title: Guardrails Reference
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/agentkb/evalmeta.yaml
    - arena/turnexecutors/pipeline_stages_integration.go
    - examples/guardrails-test/prompts/content-filter.yaml
    - examples/guardrails-test/scenarios/guardrail-should-trigger.scenario.yaml
    - packc/main.go
    - schemas/v1alpha1/promptconfig.json
---

Guardrails are runtime checks that enforce policies on LLM responses. They use the same check types as assertions and evals, run during generation, and rewrite the response content when they trigger.

For the complete list of check types and their parameters, see the [Checks Reference](https://promptkit.altairalabs.ai/reference/checks/).

## Guardrails vs Assertions

| Aspect | Guardrails | Assertions |
|--------|-----------|------------|
| Purpose | Enforce policies | Verify behavior |
| When | During generation | After the turn completes |
| Streaming | Streaming-capable checks run on each chunk | Run after the stream completes |
| Defined in | PromptConfig `validators` | Scenario `assertions` |
| On failure | Rewrites the assistant message; the run continues | Test failure |

## Guardrail Structure

Guardrails are defined in the `validators` section of a PromptConfig:

```yaml
spec:
  validators:
    - type: check_type
      params:
        param1: value1
      message: "Policy description"
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Check type. Required. |
| `params` | object | Parameters of the check type. Required. |
| `message` | string | Text that replaces the response when the guardrail fires (the length guardrails truncate instead). Defaults to a policy message. |
| `enabled` | boolean | Set to `false` to turn the guardrail off. |
| `id` | string | Identifier for the guardrail. |

An unregistered `type` fails the turn. A judge-backed type needs a `provider` param.

### The `message` Field

During pack compilation, `packc` folds `message` into `params["message"]` when `params` has no `message` of its own. This conforms to the [PromptPack schema](https://promptpack.org/schema/latest/promptpack.schema.json), which has no top-level `message` property on validators. Declare `message` at the top level in your PromptConfig YAML.

### Enforcement Behavior

When a guardrail triggers, it rewrites the assistant message in place. The pipeline continues and no error is returned.

| Check type | Rewrite |
|-----------|---------|
| `content_excludes` / `banned_words` | Replaces the content with `message`, or the default policy message. |
| `max_length` / `length` | Truncates the content to the configured maximum. |
| Any other check type that triggers | Replaces the content with `message`, or the check's own replacement, or the default blocked message. |

`max_tokens` is an estimate of 4 characters per token. `banned_words` matches whole words by default.

## Guardrail-Compatible Check Types

Any registered check type can be a guardrail. A guardrail triggers when its check scores below 1.0.

| Check Type | Aliases | Streaming | Enforcement |
|-----------|---------|-----------|-------------|
| `content_excludes` | `banned_words` | Yes | Replaces content |
| `max_length` | `length` | Yes | Truncates |
| `field_presence` | `required_fields` | No | Replaces content when any field is missing |

`banned_words` accepts `words` as an alias of `patterns`. `max_length` accepts `max_characters` as an alias of `max`.

## Streaming Guardrail Behavior

Streaming-capable guardrails (`content_excludes`, `max_length`) run on each chunk. When one triggers:

- The chunk content is replaced with the blocked text or truncated to the maximum length.
- The stage stops reading the stream.
- The turn completes normally with no error.

Chunks already emitted cannot be recalled. Streaming matching is substring-based.

Guardrails that are not streaming-capable allow every chunk and run on the completed response.

The stage stops using the stream but reads the remainder in the background, so the provider still generates the remaining tokens.

## Combining Guardrails

Multiple guardrails enforce different policies at once:

```yaml
validators:
  - type: banned_words
    params:
      words: ["guarantee", "promise", "definitely"]
    message: "Avoid absolute promises"

  - type: max_length
    params:
      max_characters: 1000
      max_tokens: 250
    message: "Stay concise"
```

Execution order:

1. Streaming-capable guardrails run on each chunk during the stream.
2. Every guardrail runs on the completed response, in declaration order.
3. The first guardrail that triggers on the completed response stops the chain; later guardrails do not run on that response.

Declaration order does not change when streaming guardrails fire.

## Examples

### Content Safety

```yaml
validators:
  - type: banned_words
    params:
      words:
        - guarantee
        - promise
        - definitely
        - "100%"
    message: "Avoid absolute promises. Use phrases like 'we'll do our best' instead."
```

### Response Length Control

```yaml
validators:
  - type: max_length
    params:
      max_characters: 2000
      max_tokens: 500
    message: "Keep responses concise"
```

### Testing Guardrails in Arena

The `guardrail_triggered` assertion checks whether a guardrail fired:

```yaml
spec:
  turns:
    - role: user
      content: "Will this definitely work?"
      assertions:
        - type: guardrail_triggered
          params:
            validator: banned_words
            should_trigger: true
```

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `validator_type` (or `validator`) | string | none | Guardrail type to check. Required. |
| `should_trigger` | boolean | `true` | Whether the guardrail is expected to fire. |
| `direction` | string | none | Optional direction filter. |

## Custom Guardrails

For custom enforcement logic, implement the `hooks.ProviderHook` interface and register it through the SDK:

```go
conv, _ := sdk.Open("./app.pack.json", "chat",
    sdk.WithProviderHook(&MyCustomHook{}),
)
```

See the [Hooks Reference](https://promptkit.altairalabs.ai/runtime/reference/hooks/) for the `ProviderHook` API and [Checks Reference extensibility](https://promptkit.altairalabs.ai/reference/checks/#extending-the-check-system) for adding check types.

## See Also

- [Checks Reference](https://promptkit.altairalabs.ai/reference/checks/): all check types and parameters
- [Unified Check Model](https://promptkit.altairalabs.ai/concepts/validation/): how guardrails, assertions, and evals relate
- [Assertions Reference](/arena/reference/assertions/): test-time checks
- [Hooks & Guardrails](https://promptkit.altairalabs.ai/runtime/reference/hooks/): runtime hook system API
- [Validation Tutorial](https://promptkit.altairalabs.ai/runtime/tutorials/04-validation-guardrails/): step-by-step guardrails tutorial
