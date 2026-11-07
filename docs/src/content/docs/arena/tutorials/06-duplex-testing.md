---
title: 'Tutorial 6: Duplex Voice Testing'
description: Build a duplex voice test that streams audio files to a mock duplex provider and asserts on the replies, with no API key.
verified:
  commit: 5a8cffd8daf00b49cbbb3596ce63750db56c3704
  sources:
    - arena/arenaconfig/types.go
    - arena/arenaconfig/voice.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/engine/duplex_conversation_executor.go
    - arena/engine/duplex_executor_assertions_integration.go
    - arena/engine/duplex_executor_pipeline_integration.go
    - arena/engine/duplex_executor_results_integration.go
    - arena/engine/duplex_executor_turns_integration.go
    - arena/engine/duplex_turn_assertions.go
    - arena/tui/tui.go
    - arena/tui/views/summary.go
    - arena/turnexecutors/audio_file_source.go
    - arena/turnexecutors/media_loader.go
    - examples/duplex-streaming/config.arena.yaml
    - examples/duplex-streaming/prompts/voice-assistant.prompt.yaml
    - examples/duplex-streaming/providers/gemini-2-flash.provider.yaml
    - examples/duplex-streaming/providers/mock-tts.provider.yaml
    - examples/duplex-streaming/providers/openai-alloy.provider.yaml
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/persona.json
    - schemas/v1alpha1/promptconfig.json
    - schemas/v1alpha1/provider.json
    - schemas/v1alpha1/scenario.json
---
In this tutorial we build a duplex voice test: a scenario that streams audio files to a provider and asserts on what comes back.

We run it against a mock duplex provider, so it needs no API key.

## What we will build

A small project with one duplex scenario of two audio turns:

```text
duplex-test/
  config.arena.yaml
  mock-responses.yaml
  audio/greeting.pcm
  audio/question.pcm
  prompts/voice-assistant.prompt.yaml
  providers/mock-duplex.provider.yaml
  scenarios/basic-duplex.scenario.yaml
```

In duplex mode `promptarena` streams the audio in chunks and the provider decides when the turn ends, instead of receiving each file as a single blob.

## Prerequisites

- `promptarena` installed, and [Tutorial 1: Your First Test](/arena/tutorials/01-first-test/) completed.
- `python3`, to create the audio files.

## Step 1: Create the project

```bash
mkdir duplex-test
cd duplex-test
mkdir audio prompts providers scenarios
```

Create `config.arena.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: duplex-test

spec:
  prompt_configs:
    - id: voice-assistant
      file: prompts/voice-assistant.prompt.yaml

  providers:
    - file: providers/mock-duplex.provider.yaml

  scenarios:
    - file: scenarios/basic-duplex.scenario.yaml

  defaults:
    concurrency: 1
    output:
      dir: out
      formats:
        - json
```

We run duplex scenarios one at a time, so `concurrency` is `1`.

## Step 2: Create the prompt

Create `prompts/voice-assistant.prompt.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: voice-assistant

spec:
  task_type: voice-assistant
  version: "1.0.0"
  description: "Voice assistant for duplex conversations"
  system_template: |
    You are Nova, a friendly voice assistant. Keep responses brief and
    conversational, two or three sentences at most.
```

## Step 3: Create the audio files

Duplex audio files are raw PCM: 16 kHz, 16-bit, mono, no header. The mock provider does not decode speech, so we create two files of silence, about two seconds each:

```bash
python3 -c "open('audio/greeting.pcm', 'wb').write(bytes(64000))"
python3 -c "open('audio/question.pcm', 'wb').write(bytes(64000))"
```

## Step 4: Create the mock duplex provider

The mock provider replies to each audio turn with scripted text. Create `providers/mock-duplex.provider.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: mock-duplex

spec:
  id: mock-duplex
  type: mock
  model: mock-duplex-model
  additional_config:
    mock_config: mock-responses.yaml
    auto_respond: true
    duplex_scenario: basic-duplex
```

Create `mock-responses.yaml`, which holds the replies for `basic-duplex`, one per turn:

```yaml
defaultResponse: "I'm here to help! How can I assist you today?"

scenarios:
  basic-duplex:
    defaultResponse: "I'm Nova, your voice assistant."
    turns:
      1:
        text: "Hello! Yes, I can hear you. I'm Nova, your voice assistant."
      2:
        text: "My name is Nova, and I can answer questions and help with everyday tasks."
```

## Step 5: Create the duplex scenario

The `duplex` block turns on duplex mode. With `mode: asm` the provider decides when each turn ends. Each turn sends an audio file instead of text.

Create `scenarios/basic-duplex.scenario.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: basic-duplex

spec:
  id: basic-duplex
  task_type: voice-assistant
  description: "Basic duplex streaming test"

  duplex:
    timeout: "30s"
    turn_detection:
      mode: asm

  streaming: true

  turns:
    - role: user
      parts:
        - type: audio
          media:
            file_path: audio/greeting.pcm
            mime_type: audio/L16
      assertions:
        - type: content_matches
          params:
            pattern: "(?i)(hello|hi|hey)"

    - role: user
      parts:
        - type: audio
          media:
            file_path: audio/question.pcm
            mime_type: audio/L16
      assertions:
        - type: content_includes
          params:
            patterns:
              - "Nova"
```

The assertions run against the provider's reply to each turn. See the [assertions reference](/arena/reference/assertions/) for every type.

## Step 6: Run the test

```bash
promptarena run --provider mock-duplex --scenario basic-duplex --simple
```

```text
Generated 1 run combinations
Starting execution…

Done  [████████████████████████]  1/1

Run Summary
────────────────────────────────────────────────────

Total runs      1
Passed          1 (100.0%)
Failed          0 (0.0%)

Assertions      2 total
Assertions Pass all passed

Total cost      $0.0000
Total tokens    0
Total duration  7.4s
Avg duration    7.4s per run

Providers       mock-duplex (1)
Scenarios       1 scenarios
Regions         default

Saved to        out
```

The scenario passes: both replies matched their assertions. The run wrote its JSON results, the assistant replies and the recorded audio to `out/`. The duration lines in your output differ.

A failing turn assertion fails the run. To see it, change the `content_includes` pattern to `"Max"` and run again. The summary shows `Failed 1`, `Assertions Fail 1` and the failure `turn assertions failed: 1 of 2`. The other turns still run, and each reply in the JSON results carries its own assertion results. With `--ci` the command exits with status 1.

## Next steps

- [Duplex streaming example](/arena/examples/duplex-streaming/): the same scenarios against Gemini Live and OpenAI Realtime, plus self-play, tools and noisy-line scenarios.
- [Duplex configuration reference](/arena/reference/duplex-config/): turn detection modes, VAD thresholds and session resilience.
- [Duplex architecture](/arena/explanation/duplex-architecture/): how duplex sessions stream audio.
- [Assertions reference](/arena/reference/assertions/): every assertion type.
- [CI/CD Integration](/arena/tutorials/05-ci-integration/): run these tests in CI.
