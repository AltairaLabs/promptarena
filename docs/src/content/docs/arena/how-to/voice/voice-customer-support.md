---
title: Test a Voice Customer Support Agent with Self-Play
description: Drive a voice agent through a multi-turn customer support call with scripted personas, runtime tools, and per-turn assertions.
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/assertions/register.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/cmd/promptarena/validate.go
    - arena/web/frontend/src/lib/arenaView.ts
    - arena/web/frontend/src/lib/atlasAdapter.ts
    - arena/web/server.go
    - examples/voice-refund-demo/config.arena.yaml
    - examples/voice-refund-demo/mock-responses.yaml
    - examples/voice-refund-demo/personas/aggressive-entitled.persona.yaml
    - examples/voice-refund-demo/personas/anxious-recipient.persona.yaml
    - examples/voice-refund-demo/personas/impersonator.persona.yaml
    - examples/voice-refund-demo/personas/patient-customer.persona.yaml
    - examples/voice-refund-demo/providers/cartesia-confident-man.provider.yaml
    - examples/voice-refund-demo/providers/elevenlabs-arnold.provider.yaml
    - examples/voice-refund-demo/providers/gemini-2-flash.provider.yaml
    - examples/voice-refund-demo/providers/mock-duplex.provider.yaml
    - examples/voice-refund-demo/providers/openai-gpt4o-realtime.provider.yaml
    - examples/voice-refund-demo/providers/openai-nova.provider.yaml
    - examples/voice-refund-demo/scenarios/aggressive-refund.scenario.yaml
    - examples/voice-refund-demo/scenarios/anxious-delivery.scenario.yaml
    - examples/voice-refund-demo/scenarios/impersonator-refund.scenario.yaml
    - examples/voice-refund-demo/scenarios/patient-baseline.scenario.yaml
    - examples/voice-refund-demo/tools/check-warranty-status.tool.yaml
    - examples/voice-refund-demo/tools/issue-refund.tool.yaml
    - examples/voice-refund-demo/tools/lookup-order.tool.yaml
    - schemas/v1alpha1/scenario.json
---

This how-to walks through the `examples/voice-refund-demo/` scenario set: a refund-support agent under voice, driven by four scripted customer personas, with conversation-level assertions on the tools the agent must (and must not) call. The same config runs in `promptarena serve`, in the TUI, and headless with `run --ci`.

## What it proves

The example tests conversational failures: whether the agent verifies the warranty before discussing a refund, gives in to pressure, or escalates when policy requires it.

- A self-play persona initiates and sustains the call. Four personas ship with the example (aggressive, impersonator, anxious, patient). By default the `selfplay-user` role uses the `mock-duplex` provider, which plays the canned `selfplay:` block of `mock-responses.yaml`. To drive the personas with a text LLM, set the role's provider to `openai-gpt4o-mini-text`.
- A realtime voice agent (OpenAI GPT-4o Realtime, Gemini Live) receives the persona's audio and responds with audio and tool calls.
- Runtime tools execute for real, mock-backed for the demo. The agent's `lookup_order`, `check_warranty_status`, `issue_refund` and `escalate_to_human` calls hit real handlers, and the results feed back into the conversation.
- Conversation-level assertions check which tools fired, plus per-turn content checks. They are pass/fail signals.

## Required API keys

| Key | Used for |
|-----|----------|
The shipped config needs none: the `selfplay-user` role uses `mock-duplex` and the four `voices:` entries are bound to `mock-tts`, so `promptarena run --provider mock-duplex` runs all four scenarios keyless and every `tools_called` / `tools_not_called` assertion passes. Keys come in when you bind the voices to their vendor TTS providers (the comment above `voices:` lists the pairs) or run a real agent:

| Key | Used for |
|-----|----------|
| `OPENAI_API_KEY` | OpenAI TTS (`openai-nova`, the patient-baseline persona's voice) and OpenAI Realtime; also the self-play text LLM if you switch the `selfplay-user` role to `openai-gpt4o-mini-text` |
| `GEMINI_API_KEY` | Gemini Live (the voice agent under test) |
| `CARTESIA_API_KEY` | Cartesia TTS for two of the personas |
| `ELEVENLABS_API_KEY` | ElevenLabs v3 TTS for the anxious-delivery persona |

With `nova` bound to `openai-nova`, the patient-baseline scenario needs only `OPENAI_API_KEY` + `GEMINI_API_KEY` (no Cartesia / ElevenLabs).

`aggressive-refund` ships an `audio_emotion` assertion commented out. It names the `hf` inference provider, which Arena builds only with `HF_TOKEN`, and a check naming an unbound provider is an error rather than a skip, so it would fail every keyless run. The scenario comment says how to turn it on.

## Run it

```bash
cd examples/voice-refund-demo
promptarena serve
```

`serve` starts a local server for the web UI (default port 8080); add `--open` to open the browser. It loads the four scenarios and lets you play each one back. Each message shows its tool calls inline.

For headless / CI use:

```bash
promptarena run --ci --formats json,markdown
open out/results.md
```

Without `--provider`, the run uses every provider in the config. Pass `--provider` to pick one.

For the dev loop (live transcript + tool table updating per turn):

```bash
promptarena run
```

## What you'll see in the report

Per scenario:

- Aggressive caller: hostile out-of-warranty refund demand. Pass = agent calls `lookup_order` + `check_warranty_status`, refuses the refund (no `issue_refund` call), and escalates via `escalate_to_human`. Fail = any unauthorized refund or skipped verification.
- Impersonator: caller supplies a fake order ID. Pass = agent attempts `lookup_order`, fails, escalates rather than guessing.
- Patient (baseline): genuine in-warranty defect. Pass = full happy path, refund issued.
- Anxious delivery: anxious customer can't find a delivered parcel. Pass = agent looks up the order, sees it was delivered with tracking, reassures.

The headline assertion in each adversarial scenario is `tools_not_called(issue_refund)` paired with `tools_called(escalate_to_human, min_calls: 1)`: the agent did not issue an unauthorized refund, and it escalated.

## CI gate

For CI without provider keys, run the shipped config with `promptarena run --ci --provider mock-duplex`: it runs all four scenarios through the duplex pipeline against the mock agent. `promptarena validate` checks the configs without running anything. The scenarios against a live voice agent need provider keys. See [Run in CI](/arena/how-to/interfaces/run-in-ci/) for the general setup.

```yaml
# .github/workflows/voice-refund-demo.yml
name: Voice Refund Demo

on:
  pull_request:
    paths:
      - 'examples/voice-refund-demo/**'

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Validate configs (no provider keys needed)
        working-directory: examples/voice-refund-demo
        run: ../../bin/promptarena validate config.arena.yaml

  run-against-real-providers:
    runs-on: ubuntu-latest
    if: github.event.pull_request.head.repo.full_name == github.repository
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run scenarios against real providers
        working-directory: examples/voice-refund-demo
        env:
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
          GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
          CARTESIA_API_KEY: ${{ secrets.CARTESIA_API_KEY }}
          ELEVENLABS_API_KEY: ${{ secrets.ELEVENLABS_API_KEY }}
        run: ../../bin/promptarena run --ci --formats json,markdown
      - name: Upload report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: arena-report
          path: examples/voice-refund-demo/out/
```

The fork-aware `if:` check on the real-provider job prevents the secret-bearing job from running on PRs from external forks.

## Extending it

- To add a persona, create a new file under `personas/`, anchor it to one of the existing order IDs (see `tools/*.tool.yaml` for the mock_template branches), add a scenario in `scenarios/` that references it, and list both files in `config.arena.yaml`.
- To add a product or warranty case, extend the `mock_template` branches in `tools/lookup-order.tool.yaml`, `tools/check-warranty-status.tool.yaml`, and `tools/issue-refund.tool.yaml`. No code changes are needed.
- To swap the agent under test, pass `--provider` on the CLI, or give a scenario a `providers:` list. Available providers in the example: `gemini-2-flash`, `openai-gpt4o-realtime`, plus `mock-duplex` for pipeline smoke testing.

## Related pages

- [Test Voice Personas](/arena/how-to/voice/voice-characterization/)
- [Test Voice Tool Calls](/arena/how-to/voice/voice-tool-calls/)
- [Set Up Voice Testing with Self-Play](/arena/how-to/voice/setup-voice-testing/)
