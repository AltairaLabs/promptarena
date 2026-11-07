---
title: Enable Model Reasoning
description: Turn on extended thinking for Gemini, Claude and OpenAI, and capture the trace separately from the answer. Each provider uses a different control, and setting the wrong one usually yields an empty trace rather than an error.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - arena/arenaconfig/validator_tools.go
    - arena/engine/builder_integration.go
    - arena/engine/conversation_executor.go
    - arena/engine/types.go
    - arena/results/json/json_repository.go
    - arena/results/markdown/markdown_repository.go
    - arena/statestore/store.go
    - arena/tui/app/livefeed.go
    - arena/tui/event_adapter.go
    - arena/tui/panels/conversation.go
    - arena/tui/panels/conversation_render.go
    - arena/turnexecutors/pipeline_stages_integration.go
    - arena/web/event_adapter.go
    - arena/web/frontend/src/components/InteractiveChat.tsx
    - arena/web/frontend/src/hooks/useArenaEvents.ts
    - arena/web/frontend/src/lib/atlasAdapter.ts
    - examples/reasoning-test/providers/claude-sonnet.provider.yaml
    - examples/reasoning-test/providers/gemini-3-flash.provider.yaml
    - examples/reasoning-test/providers/openai-gpt5.provider.yaml
    - examples/reasoning-test/scenarios/reasoning-tools.scenario.yaml
    - examples/reasoning-test/scenarios/reasoning.scenario.yaml
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/provider.json
---

Thinking models can return their reasoning alongside the answer. PromptArena captures it as `Message.Reasoning`, a sibling of content that stays out of the answer, the content exports and the context passed to later turns.

A wrong provider control gives an empty trace and a passing run, not an error. Verify capture explicitly (see [Verify capture](#verify-capture)).

Worked example: `examples/reasoning-test/`.

## Turn it on

Configure reasoning per provider, under `spec.additional_config`. An unrecognised `thinking_level`, `reasoning_effort` or `reasoning_summary` value is dropped with a logged warning and the request still goes out.

### Gemini

Gemini 3 and Gemini 2.5 use different, non-interchangeable controls:

```yaml
# Gemini 3.x
spec:
  model: gemini-3.7-flash
  additional_config:
    include_thoughts: true
    thinking_level: high      # low | medium | high
```

```yaml
# Gemini 2.5
spec:
  model: gemini-2.5-flash
  additional_config:
    include_thoughts: true
    thinking_budget: 1024     # reasoning-token cap; counts toward max_tokens
```

If you set both `thinking_level` and `thinking_budget`, the level wins and the budget is dropped. Swapping the controls fails quietly or loudly depending on the model:

- `thinking_budget` on a Gemini 3 model returns no thought summaries. The run passes and `reasoning` is empty.
- `thinking_level` on a Gemini 2.5 model is a hard 400.

Gemini also omits the summary on the occasional call even when configured correctly, so treat its presence as likely rather than guaranteed on any single request.

### Claude

```yaml
spec:
  model: claude-sonnet-4-6
  defaults:
    temperature: 0.0        # dropped from the request while thinking is on
    max_tokens: 4096
    top_p: 1.0
  additional_config:
    thinking_budget: 2048   # must be >= 1024
```

You set a budget; PromptKit picks the wire format from the model name. `claude-3*`, Opus 4.1, Sonnet 4.5, Haiku 4.5 and Opus 4.5 take `enabled` with `budget_tokens`. Every other model, including 4.6 and later, takes `adaptive`, and the value of `thinking_budget` is not sent there. The 1024 minimum applies to the `enabled` shape.

Two adjustments happen for you:

- `temperature` is dropped while thinking is on, because the API rejects a custom one.
- On the `enabled` shape, `max_tokens` is raised to the budget plus 1024 when it is at or below the budget, since reasoning tokens count toward the output cap. Adaptive models get no adjustment.

### OpenAI

Reasoning summaries are a Responses API feature. On `api.openai.com`, or with an empty `base_url`, PromptArena uses the Responses API by default, so the config is:

```yaml
spec:
  model: gpt-5.2
  additional_config:
    reasoning_effort: high     # minimal | low | medium | high
    reasoning_summary: auto    # auto | concise | detailed
```

If `base_url` points at another host, add `api_mode: responses`. Azure and Bedrock always use Chat Completions, so they cannot produce a summary. Without the Responses API, `reasoning_summary` and `reasoning_effort` have no effect and `reasoning` comes back empty.

`reasoning_summary` is separately gated: requesting summaries requires a verified OpenAI org. Unverified, the model still reasons and you are still billed reasoning tokens, but the trace never reaches you. An empty `reasoning` on OpenAI therefore has two causes: the API mode (fixable in config) or an unverified org (not).

## Verify capture

An empty trace does not fail anything, so check the output:

```bash
promptarena run --ci --formats json
jq '(.Messages // [])[] | select(.role=="assistant") | {content, reasoning}' out/*.json
```

The `// []` skips `out/index.json`, which has no `.Messages`. A populated `reasoning` proves capture. If your prompt also asks for a terse answer, a short `content` proves the trace did not leak into the reply.

Where reasoning is visible:

| Surface | What it shows |
|---|---|
| JSON results | The `reasoning` field, as in the check above. |
| TUI (`promptarena run` without `--ci`) | A `💭 Reasoning` section in the turn detail, and a live block while a streaming turn runs. |
| Web UI (`promptarena serve`) | A collapsible reasoning disclosure on the message, for live runs and historical results, on either side of a tool call. Interactive chat also shows a streaming preview while the turn runs; the complete trace arrives with the message. |

The markdown report (`out/results.md`) does not show model reasoning. Its conversation-assertion tables print a `reasoning` value, which is an LLM judge's reasoning for its verdict.

### Reasoning is written to disk

Arena's save stage stores reasoning on the message, and run results are rebuilt from the conversation store, so `out/*.json` contains the traces. Before you share those files:

- The JSON holds every reasoning text the provider returned: Claude's thinking, Gemini's thought summaries and OpenAI's reasoning summaries.
- It also holds opaque reasoning entries: provider-native signatures and encrypted blocks. They are never displayed and are routinely larger than the text beside them (536–1288 bytes per message against Claude). They are stripped before reaching the browser, but not before reaching disk.

Treat `out/*.json` from a reasoning run as sensitive. Attaching one to a bug report or a CI artifact publishes the model's thinking along with provider-internal tokens.

## Write scenarios that actually produce a trace

Trace length tracks problem difficulty. A one-step question yields little or nothing to summarise, so models frequently return an empty trace for it. That is not a capture failure.

Give a reasoning test something to chew on: several chained derivations, or a follow-up that has to combine earlier results. `examples/reasoning-test/` does this, and its turn 3 (a single addition) usually returns an empty trace between two turns that don't.

## Reasoning with tools

Reasoning is captured on both sides of a tool loop: before the call, and again once the results land. Configure reasoning and tools normally. See `examples/reasoning-test/scenarios/reasoning-tools.scenario.yaml`.

The prompt config's `allowed_tools` is an allowlist: a prompt with no entry gets no tools. Keep it in step with what your system prompt tells the model it can do.

## Compatibility notes

- Seed: the arena-level `defaults.seed` (see the [config schema](/arena/reference/config-schema/)) is sent only when non-zero, and only on API paths that accept it. The OpenAI Responses API has no `seed` parameter, so the runtime never sends one there and logs a warning that the request is not reproducible. For a fixed seed, use `api_mode: completions` on a model that supports it. Reasoning summaries come only from the Responses path, so you cannot have both.
- Model-specific rejections: use `unsupported_params` to withhold a parameter a model refuses. It is honoured for sampling parameters on both API paths.
