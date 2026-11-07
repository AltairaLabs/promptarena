---
title: Add Voice PII Guardrails
description: Wire the pii_leakage guardrail into a pack so it replaces a PII-bearing reply before it is spoken, and assert that it fired with guardrail_triggered.
verified:
  commit: 3df344ffbc8c604e1a8935929c28499d778d19f0
  sources:
    - arena/cmd/promptarena/main_integration.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/engine/duplex_executor_pipeline_integration.go
    - arena/stages/duplex_guardrail.go
    - arena/turnexecutors/pipeline_stages_integration.go
    - examples/pii-guardrail-red-team/scenarios/legitimate-question.scenario.yaml
    - examples/voice-guardrails/config.arena.yaml
    - examples/voice-guardrails/prompts/support-agent.yaml
    - examples/voice-guardrails/providers/mock-judge.yaml
    - examples/voice-guardrails/scenarios/pii-redaction-duplex.scenario.yaml
    - examples/voice-guardrails/scenarios/pii-redaction.scenario.yaml
---

This how-to walks through `examples/voice-guardrails/`: one scenario with the `pii_leakage` guardrail declared in the pack. The runtime catches PII in the agent's reply before it reaches the TTS layer, and the test observes the firing with the `guardrail_triggered` assertion.

## What it proves

A voice agent's PII leak goes out as audio, so the guardrail has to fire before TTS, and the test has to show that it did:

- The pack's `validators:` block declares `pii_leakage`.
- The runtime wraps it as a `ProviderHook`. On every assistant message, a regex pre-pass scans the reply. On a hit, the runtime replaces `resp.Message.Content` with the safe message before any later stage speaks it.
- The hook also records a `validations:` block on the message with what the guardrail did. The `guardrail_triggered` assertion reads that block; it does not re-run the eval.

## Run it

```bash
cd examples/voice-guardrails
promptarena serve
```

`serve` loads the single scenario. The recorded assistant message holds the replaced, safe content, plus the `validations:` block (what the guardrail did). The PII the agent tried to say does not appear in the recording.

Headless / CI:

```bash
promptarena run --ci --formats json,markdown
open out/results.md
```

The run needs no LLM key: `pii_leakage`'s regex pre-pass catches the PII, and the example's judge is a mock provider.

## The assertion shape

```yaml
conversation_assertions:
  - type: guardrail_triggered
    params:
      validator: pii_leakage
      should_trigger: true
    message: "pii_leakage guardrail must fire on the agent's would-be-spoken response"
```

`guardrail_triggered` reads the `validations:` block the runtime records on the message. The result is deterministic: it tells you whether the production guardrail caught the PII.

## Side-by-side with red-team

This demo has one scenario and one assertion. For a second scenario that checks the guardrail stays quiet on a legitimate question, see [Red-team a support agent for PII leakage](/arena/how-to/scenarios/pii-guardrail-red-team/).

## Switching to live voice

The duplex voice path runs the pack's output guardrails too, on each assistant turn when the realtime session completes it, and records the firing the same way, so `guardrail_triggered` works on a scenario with a `duplex:` block (`pii-redaction-duplex` in this example). The provider streams the reply's audio while it speaks, so the caller has heard it before the guardrail sees the finished transcript; enforcement rewrites the turn as recorded (state, session recording, reports, later assertions), not the audio already played. Input guardrails do not run on the duplex path.

## CI gate

```yaml
# .github/workflows/voice-guardrails.yml
name: Voice guardrails

on:
  pull_request:
    paths:
      - 'examples/voice-guardrails/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run voice-guardrails scenario
        working-directory: examples/voice-guardrails
        run: ../../bin/promptarena run --ci --formats json
```

The demo is one scenario with one regex-based assertion, so the job needs no secrets and is safe for PRs from forks. To add the LLM-judged guardrails (`bias`, `toxicity`, `role_violation`), put them in a separate job that reads provider keys from secrets.

## Related pages

- [Red-team a support agent for PII leakage](/arena/how-to/scenarios/pii-guardrail-red-team/)
- [Run in CI](/arena/how-to/interfaces/run-in-ci/)
