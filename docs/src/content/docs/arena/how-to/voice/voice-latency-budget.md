---
title: Assert Voice Latency Budgets
description: Gate scenarios on measured provider latency with the latency_budget assertion, which reads each turn's latency_ms from the assistant message.
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/engine/eval_orchestrator.go
    - examples/voice-latency-budget/config.arena.yaml
    - examples/voice-latency-budget/scenarios/turn-budget.scenario.yaml
    - schemas/v1alpha1/scenario.json
---

This how-to walks through `examples/voice-latency-budget/`: a small scenario that asserts every turn's provider latency against an explicit `max_ms` budget. Use it to gate deploys on latency regressions, compare providers on the same scenario, and check that an agent stays inside a real-time budget.

## What it proves

- The provider stage records `LatencyMs` on every assistant message: the duration of that provider call. On the duplex voice path it is the time from the start of the turn to the reply.
- Arena copies the latest assistant message's `LatencyMs` into the eval context metadata as `latency_ms`, which the `latency_budget` assertion reads.
- Each turn is gated: `max_ms: 1000` fails any reply slower than a second.

## The assertion shape

```yaml
turns:
  - role: user
    content: "Hi, I need help with my account."
    assertions:
      - type: latency_budget
        params:
          max_ms: 1000
        message: "Turn must respond within 1000ms"
```

`latency_budget` returns a score normalised to the budget: `min(1.0, max_ms / latency_ms)`. A reply within budget scores 1.0; a reply at 2× the budget scores 0.5. The per-run JSON file records each turn's `latency_ms` and `budget_ms`, with an explanation such as `latency 0.0ms vs budget 1000.0ms`. The Markdown report shows the pass/fail count per run.

## Run it

```bash
cd examples/voice-latency-budget
promptarena serve
```

`serve` starts a local server for the web UI (default port 8080; add `--open` to open the browser) and loads the scenario. Headless:

```bash
promptarena run --ci --formats json,markdown
open out/results.md
```

The default config runs against a text mock provider. It answers in under a millisecond, so the budget always passes. Real providers give the measurement meaning.

## Comparing providers

Add provider files and register them in `config.arena.yaml`. Arena runs the scenario against every registered provider, and each provider's per-turn `latency_ms` is in its per-run JSON file. Use this for migration testing ("does the new provider stay inside our 800ms budget on this prompt?"). See [Run a Voice Provider Bake-Off](/arena/how-to/voice/voice-bake-off/).

## What is measured

`latency_budget` checks the latency of the turn's final provider call. On a turn with tool calls, each provider round records its own `LatencyMs`, and the assertion reads the last one. There is no time-to-first-token or time-to-first-audio key in the eval context, so `latency_budget` cannot gate on those.

## CI gate

The mock-provider path runs without keys, so the demo fits a fork-safe CI job. See [Run in CI](/arena/how-to/interfaces/run-in-ci/) for the general setup.

```yaml
# .github/workflows/voice-latency-budget.yml
name: Voice latency budget

on:
  pull_request:
    paths:
      - 'examples/voice-latency-budget/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run latency-budget scenarios
        working-directory: examples/voice-latency-budget
        run: ../../bin/promptarena run --ci --formats json
```

For real-provider runs, use the same step with provider keys from `secrets:`, and set `max_ms` to your production budget.

## Extending it

- Vary `max_ms` per turn: the greeting might be fast, the tool-using turn slower.
- Combine `latency_budget` with `content_includes` or `llm_judge` to test both correctness and speed.
- An assertion's `when:` block conditions on tool calls only (`tool_called`, `tool_called_pattern`, `any_tool_called`, `min_tool_calls`), not on the provider. A budget applies to every provider the scenario runs against.

## Related pages

- [Run a Voice Provider Bake-Off](/arena/how-to/voice/voice-bake-off/)
- [Set Up Voice Testing with Self-Play](/arena/how-to/voice/setup-voice-testing/)
