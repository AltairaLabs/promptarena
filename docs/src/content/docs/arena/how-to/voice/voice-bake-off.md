---
title: Run a Voice Provider Bake-Off
description: Run one scenario against several providers and compare their assertion results and latency in one report.
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/main_integration.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/engine/eval_orchestrator.go
    - arena/engine/execution.go
    - arena/results/markdown/markdown_repository.go
    - arena/web/frontend/src/App.tsx
    - arena/web/frontend/src/components/arena/TrialMatrix.tsx
    - examples/voice-bake-off/config.arena.yaml
    - examples/voice-bake-off/providers/mock-fast.provider.yaml
    - examples/voice-bake-off/providers/mock-slow.provider.yaml
    - examples/voice-bake-off/scenarios/standard-call.scenario.yaml
    - examples/voice-refund-demo/scenarios/aggressive-refund.scenario.yaml
    - schemas/v1alpha1/provider.json
    - schemas/v1alpha1/scenario.json
---

This how-to walks through `examples/voice-bake-off/`: a three-turn support call run against two registered providers (`mock-fast`, `mock-slow`), with per-turn `latency_budget` and `max_length` assertions. Arena runs the scenario once per provider, and `promptarena serve` shows the results in a scenario-by-provider matrix.

## What it proves

Running the same scenario across providers evaluates each one on your prompt, your tools and your assertions. Register several providers in `config.arena.yaml` and every scenario runs against each of them. Assertions run per provider, and the report has one row per provider.

## Run it

```bash
cd examples/voice-bake-off
promptarena serve
```

The web UI loads the scenario and shows a scenario-by-provider results matrix, with standings and a results ledger.

Headless:

```bash
promptarena run --ci --formats json,markdown
open out/results.md
```

`results.md` has a results table with one row per provider and its assertion pass count. Each provider's per-turn `latency_ms` and response text are in the per-run JSON files in `out/`. The mock providers answer in a few milliseconds, so in this demo they differ in reply length, not speed.

## The fan-out

In `config.arena.yaml`:

```yaml
providers:
  - file: providers/mock-fast.provider.yaml
  - file: providers/mock-slow.provider.yaml
```

The scenario does not mention providers. Arena pairs every scenario with every registered provider and runs each combination. To add a provider, add its provider YAML file and one more line here. A mock provider also needs its mock responses file.

## Per-provider thresholds

An assertion's `when:` block conditions on tool calls only (`tool_called`, `tool_called_pattern`, `any_tool_called`, `min_tool_calls`). It has no `provider` key, and the scenario schema rejects one. A `latency_budget` assertion's `max_ms` applies to every provider the scenario runs against:

```yaml
- type: latency_budget
  params:
    max_ms: 800
```

To compare providers against different budgets, read each provider's `latency_ms` in the per-run JSON. This is useful for migration testing ("does the new provider stay inside the same budget on our prompts?").

## Adding real voice providers

The default config uses text mock providers, so the demo runs without provider keys. To run across real voice providers:

1. Add provider configs, for example `providers/openai-realtime.provider.yaml`:

   ```yaml
   apiVersion: promptkit.altairalabs.ai/v1alpha1
   kind: Provider
   metadata:
     name: openai-realtime
   spec:
     id: openai-realtime
     type: openai
     model: gpt-realtime
     additional_config:
       realtime: true
   ```

2. Register them in `config.arena.yaml` under `providers:`.
3. Add a `duplex:` block to the scenario (see `examples/voice-refund-demo/scenarios/*.yaml`).
4. Run with provider keys.

## CI gate

```yaml
# .github/workflows/voice-bake-off.yml
name: Voice bake-off

on:
  pull_request:
    paths:
      - 'examples/voice-bake-off/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run bake-off scenarios
        working-directory: examples/voice-bake-off
        run: ../../bin/promptarena run --ci --formats json,markdown
      - name: Upload report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: bake-off-report
          path: examples/voice-bake-off/out/
```

The uploaded artifact holds `results.md` and the per-run JSON files, so reviewers can read each provider's responses from the PR.

## Related pages

- [Assert Voice Latency Budgets](/arena/how-to/voice/voice-latency-budget/)
- [Set Up Voice Testing with Self-Play](/arena/how-to/voice/setup-voice-testing/)
- [Run in CI](/arena/how-to/interfaces/run-in-ci/)
