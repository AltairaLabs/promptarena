---
title: Test Voice Personas
description: Have a self-play persona emit bracket-tagged expressive markup that each TTS provider lowers into its native format, and assert the agent stays composed across turns.
verified:
  commit: 0dbee6290db4e4de3bff3101f260fd614f81c924
  sources:
    - arena/arenaconfig/judge_param.go
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/persona.go
    - arena/arenaconfig/types.go
    - arena/assertions/types.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/validate.go
    - arena/engine/builder_integration.go
    - arena/engine/duplex_executor_turns_integration.go
    - arena/selfplay/audio_generator.go
    - arena/selfplay/mock_tts_interactive.go
    - arena/stages/persona_assembly.go
    - examples/voice-refund-demo/config.arena.yaml
    - examples/voice-refund-demo/personas/aggressive-entitled.persona.yaml
    - examples/voice-refund-demo/personas/anxious-recipient.persona.yaml
    - examples/voice-refund-demo/scenarios/aggressive-refund.scenario.yaml
    - schemas/v1alpha1/scenario.json
---

This how-to walks through the expressive-persona path in `examples/voice-refund-demo/`. With the TTS characterization tag dialect, a persona emits tags such as `[shouts]`, `[sighs]` and `[whispers]`, and each TTS provider adapter lowers them into its native API. You write one persona prompt for every provider.

## What it proves

- Personas emit canonical `[tag]` markup. The canonical tags are `whispers`, `shouts`, `laughs`, `sighs`, `excited`, `sad`, `smiles`, `calm` and `pause` (for example `[pause:500ms]`).
- Each TTS provider adapter parses the tags and lowers them into its native format: pass-through for ElevenLabs v3, the `instructions:` field for `gpt-4o-mini-tts`, and a single emotion for Cartesia. Cartesia also raises the volume for shout tags, and drops `whispers`, `calm` and `pause`. Adapters drop tags outside the taxonomy.
- The persona prompt needs no per-provider fork. The rubric the runtime injects depends on the provider: the full taxonomy for ElevenLabs v3 and `gpt-4o-mini-tts`, emotions only for Cartesia, and none for older OpenAI and ElevenLabs models.

## Opt in via the persona

In `examples/voice-refund-demo/personas/aggressive-entitled.persona.yaml`:

```yaml
spec:
  id: aggressive-entitled
  voice: confident-man
  style:
    verbosity: short
    challenge_level: high
    expressive: true   # ← teaches the persona to emit bracket tags
```

When `expressive: true` is set, the runtime prepends a short rubric to the persona's system prompt explaining the tag dialect. The LLM then emits text like:

```text
[shouts] I don't care about your warranty policy! [sighs] I want my money back!
```

The TTS adapter parses the bracket spans, lowers them into its native dialect, and synthesises audio. Only the TTS adapters strip the tags. The persona message and transcript keep them, so text assertions such as `content_matches` and `content_includes` see the brackets.

## Run it

The shipped config binds every persona voice to `mock-tts`, which plays a canned clip and lowers no tags. For the expressive path, bind `confident-man` (the `aggressive-refund` caller) to `cartesia-confident-man` in `config.arena.yaml` and set `CARTESIA_API_KEY`. For an LLM to write the tags, also bind the `selfplay-user` role to `openai-gpt4o-mini-text` and set `OPENAI_API_KEY`.

```bash
cd examples/voice-refund-demo
promptarena serve
```

Pick the `aggressive-refund` scenario. The persona's audio output exercises the characterization path: the agent under test (Gemini Live, OpenAI Realtime) hears an expressive caller and replies in voice.

Headless / CI:

```bash
promptarena run --scenario aggressive-refund --formats json,markdown
```

The Markdown report shows each persona-side turn with the assertion results. For full conversation detail including parsed expressive tags, run `promptarena serve`.

## Persona-side authoring

Two patterns are useful:

- Per-trait personas stick to a narrow emotional band: `aggressive-entitled` shouts and demands; `anxious-recipient` worries and apologises. The expressive rubric encourages the LLM to keep within that band.
- Mood-shifting personas drift across the call (calm → agitated → conciliatory). The system prompt includes per-turn guidance: "turn 1: open friendly; turn 2: get frustrated; turn 3: escalate". The LLM emits different tags per turn, and the agent under test has to handle the modulation.

## Asserting persona consistency

The useful tests are cross-turn: they check that the agent stays in role as the caller modulates. Two patterns:

- `llm_judge_session` with criteria about consistency: "did the agent maintain a professional tone across all turns?"
- `content_includes_any` / `content_excludes` per turn, checking for explicit calmness markers when the persona escalates.

```yaml
conversation_assertions:
  - type: assertion
    params:
      eval_type: llm_judge_session
      eval_params:
        criteria: |
          Did the agent maintain a calm, professional tone across all turns,
          even when the caller escalated? Score 1.0 if yes, 0.0 if the agent
          matched the caller's tone instead of staying composed.
      min_score: 0.8
```

Without a `judge:` key, the default judge scores it. To pick another, set `judge:` in `eval_params` to a name defined in `spec.judges`; an unknown name fails at load.

## What's not covered

`expressive: true` sends persona-side tags to TTS. Arena's audio-side assertions are `audio_duration`, `audio_format` and `audio_emotion`. `audio_emotion` runs speech-emotion recognition through an inference provider such as `hf` and needs `HF_TOKEN`; `aggressive-refund` uses it. Other judgements about the agent's speech (warmth, professionalism) go through the LLM-judged path, operating on the text of the agent's speech.

The reference list of supported tags per provider lives in PromptKit's `runtime/tts/markup/`. See the package doc for the canonical dialect and what each TTS adapter recognizes.

## Switching providers

Persona prompts are provider-agnostic. The persona's `voice` names a `voices:` entry in `config.arena.yaml`, and each entry binds a voice id to a TTS provider. To send the same expressive persona through a different TTS, re-point that entry's provider:

```yaml
# config.arena.yaml
voices:
  - id: confident-man
    provider: elevenlabs-arnold    # was: cartesia-confident-man
```

Pick a provider that takes expressive markup: ElevenLabs `eleven_v3`, OpenAI `gpt-4o-mini-tts` or Cartesia. Older models such as OpenAI `tts-1` get no rubric and strip the tags.

A scenario-level `voices:` list overrides the persona's voice, one voice per trial. The persona's bracket tags reach the new provider, and its adapter lowers them into its native format. The persona prompt does not change.

## CI gate

The expressive path needs real provider keys: the mock TTS plays a canned clip, adds no rubric, and never asks the persona to emit tags. For CI, validate the configs without making provider calls:

```yaml
- name: Validate expressive scenarios
  working-directory: examples/voice-refund-demo
  run: ../../bin/promptarena validate config.arena.yaml
```

For real-provider gated runs, follow the pattern in the [voice customer support how-to](/arena/how-to/voice/voice-customer-support/): a secret-gated `head.repo.full_name == github.repository` job with provider keys in `secrets:`.

## Related pages

- [Test a Voice Customer Support Agent with Self-Play](/arena/how-to/voice/voice-customer-support/)
- [Set Up Voice Testing with Self-Play](/arena/how-to/voice/setup-voice-testing/)
