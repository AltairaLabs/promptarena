---
title: Enable Model Reasoning
description: Turn on extended thinking for Gemini, Claude and OpenAI, and capture the trace separately from the answer. Each provider uses a different control, and setting the wrong one usually yields an empty trace rather than an error.
---

Thinking models can return their reasoning alongside the answer. PromptArena captures it as `Message.Reasoning` — a **sibling of content**, never mixed into the answer, the exports, or the context passed to later turns.

Every provider gates this differently, and the important part is what happens when you get it wrong: you almost always get an **empty trace and a passing run**, not an error. Nothing tells you reasoning was requested and not delivered. So verify capture explicitly rather than assuming a green run means it worked.

Worked example: `examples/reasoning-test/`.

## Turn it on

Reasoning is configured per provider, under `spec.additional_config`.

### Gemini

Gemini 3 and Gemini 2.5 use **different, non-interchangeable** controls:

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

Swapping them does not fail loudly:

- `thinking_budget` on a **Gemini 3** model returns no thought summaries at all. The run passes, `reasoning` is empty.
- `thinking_level` on a **Gemini 2.5** model is a hard 400.

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

You set a budget; you do **not** choose the wire format. PromptKit selects it from the model generation — 4.5-and-older take `enabled`+`budget_tokens`, 4.6 takes either, and the 5-series takes `adaptive` and rejects `enabled`. Sending the wrong shape is a 400, not a degraded response, which is why this is inferred rather than configured.

Two adjustments happen for you:

- **`temperature` is dropped** while thinking is on, because the API rejects a custom one. The provider schema still requires a value under `defaults`, so set one and expect it not to reach the wire.
- **`max_tokens` is raised** if the budget would leave no headroom for an answer, since reasoning tokens count toward the output cap.

### OpenAI

OpenAI needs one extra step that catches people out — reasoning summaries are a **Responses API** feature, and PromptArena defaults every model that isn't `-pro` to Chat Completions:

```yaml
spec:
  model: gpt-5.2
  unsupported_params:
    - top_p                    # gpt-5.2 rejects it outright
  additional_config:
    api_mode: responses        # required for any visible trace
    reasoning_effort: high     # minimal | low | medium | high
    reasoning_summary: auto    # auto | concise | detailed
```

Without `api_mode: responses`, `reasoning_summary` does nothing whatever you set it to, and `reasoning` comes back empty.

`reasoning_summary` is separately gated: requesting summaries requires a **verified OpenAI org**. Unverified, the model still reasons and you are still billed reasoning tokens — the trace just never reaches you. An empty `reasoning` on OpenAI therefore has two quite different causes worth distinguishing: wrong API mode (fixable in config) or an unverified org (not).

## Verify capture

Do this rather than trusting a passing run — an empty trace does not fail anything:

```bash
promptarena run --ci --formats json
jq '.Messages[] | select(.role=="assistant") | {content, reasoning}' out/*.json
```

A populated `reasoning` proves capture. If your prompt also asks for a terse answer, a short `content` proves the trace didn't leak into the reply.

Reasoning is visible in:

- **JSON results** — the check above.
- **TUI** (`promptarena run` without `--ci`) — a `💭 Reasoning` section in the turn detail; interactive and voice sessions stream it live.
- **Web UI** (`promptarena serve`) — a collapsible reasoning disclosure on the message, for both live runs and historical results, including on either side of a tool call. Note that unlike the TUI it does not stream token-by-token: the trace appears once, when the message lands.

One surface does **not** show it: the **markdown report** (`out/results.md`) carries the summary, per-run and cost tables only. It does print a `reasoning` value against assertions, but that is an LLM judge's reasoning for its verdict — unrelated to model thinking.

### Reasoning is written to disk

Reasoning **is** persisted. Arena's save stage stores it on the message, and run results are rebuilt from the conversation store — which is why `out/*.json` contains the traces.

Two consequences worth knowing before you ship those files anywhere:

- The JSON holds the model's full chain-of-thought, not a summary of it.
- It also holds **opaque** reasoning entries — provider-native signatures and encrypted blocks kept for intra-turn round-trip. These are never displayed and are routinely larger than the text beside them (536–1288 bytes per message against Claude). They are stripped before reaching the browser, but not before reaching disk.

So treat `out/*.json` from a reasoning run as sensitive: attaching one to a bug report or a CI artifact publishes the model's thinking along with provider-internal tokens.

## Write scenarios that actually produce a trace

Trace length tracks problem difficulty, and this surprises people more than the configuration does.

A one-step question yields little or nothing to summarise — models frequently return an **empty** trace for it while producing hundreds of characters on either side of it in the same conversation. That is not a capture failure.

If you are building a reasoning test, give it something to chew on: several chained derivations, or a follow-up that has to combine earlier results. `examples/reasoning-test/` does this deliberately, and its turn 3 (a single addition) reliably returns an empty trace between two turns that don't — a useful calibration point when you are deciding whether your own empty trace is a bug or the expected result.

## Reasoning with tools

Reasoning is captured on **both sides** of a tool loop: before the call, and again once the results land. Nothing extra is required to enable that beyond configuring reasoning and tools normally — see `examples/reasoning-test/scenarios/reasoning-tools.scenario.yaml`.

Remember that the prompt config's `allowed_tools` is an allowlist: a prompt with no entry gets no tools. Keep it in step with what your system prompt tells the model it can do, or the model will be instructed to call tools it has not been granted.

## Compatibility notes

- **Seed.** A seed is only sent when you configure a non-zero one under `defaults.seed`. This matters for OpenAI: the Responses API rejects `seed` outright, and `unsupported_params` cannot suppress it (see [PromptKit #1870](https://github.com/AltairaLabs/PromptKit/issues/1870)). If you need both a fixed seed and OpenAI reasoning, you are blocked until that is fixed upstream.
- **Model-specific rejections.** Use `unsupported_params` to withhold a parameter a model refuses — `top_p` on `gpt-5.2`, for example. It is honoured for sampling parameters on both API paths.
