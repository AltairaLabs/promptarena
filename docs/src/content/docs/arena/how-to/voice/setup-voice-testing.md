---
title: Set Up Voice Testing with Self-Play
verified:
  commit: c67065389ccbc6e76babad54b4345e1c76bb633e
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/persona.go
    - arena/arenaconfig/types.go
    - arena/arenaconfig/voice.go
    - arena/cmd/promptarena/run.go
    - arena/engine/composite_conversation_executor.go
    - arena/engine/conversation_executor.go
    - arena/engine/duplex_conversation_executor.go
    - arena/engine/duplex_executor_pipeline_integration.go
    - arena/engine/duplex_executor_turns_integration.go
    - arena/selfplay/audio_generator.go
    - arena/selfplay/registry.go
    - arena/selfplay/tts_registry.go
    - examples/duplex-streaming/providers/gemini-2-flash.provider.yaml
    - examples/duplex-streaming/scenarios/duplex-selfplay.scenario.yaml
    - schemas/v1alpha1/provider.json
---
Run automated multi-turn voice tests, where a self-play persona speaks through TTS.

## Prerequisites

- Gemini API key (for duplex streaming)
- OpenAI API key (for TTS and the self-play text model, or use mock providers)
- Audio files in PCM format (16kHz, 16-bit, mono)

## Quick Setup

The steps build one project directory. Every path below is relative to it, and
`config.arena.yaml` loads each file by the path shown in its heading. `examples/duplex-streaming/` holds a
larger project with the same layout.

```text
config.arena.yaml
audio/greeting.pcm
prompts/voice-assistant.prompt.yaml
providers/gemini-live.provider.yaml
providers/openai-gpt4o-mini-text.provider.yaml
providers/openai-alloy.provider.yaml
providers/mock-tts.provider.yaml
personas/test-user.persona.yaml
scenarios/voice-selfplay.scenario.yaml
```

Copy `greeting.pcm` from `examples/duplex-streaming/audio/`, or record your own in the format
above.

### 1. Declare the Arena Config

TTS is configured at the arena level. Declare one or more TTS provider files under
`tts_providers:`, then bind voice IDs in `voices:`. Personas and scenarios reference
those IDs, so one edit to `voices:` swaps between a real vendor and mock TTS for CI.

The `prompt_configs:` entry supplies the prompt whose `task_type` the scenario names, and
`scenarios:` lists the file that `--scenario voice-selfplay` resolves.

```yaml
# config.arena.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: voice-testing
spec:
  prompt_configs:
    - id: voice-assistant
      file: prompts/voice-assistant.prompt.yaml

  providers:
    - file: providers/gemini-live.provider.yaml
    - file: providers/openai-gpt4o-mini-text.provider.yaml  # text LLM for the self-play user

  tts_providers:
    - file: providers/openai-alloy.provider.yaml  # real TTS
    - file: providers/mock-tts.provider.yaml       # for CI

  voices:
    # Real-vendor mode: point to openai-alloy.
    # CI / keyless mode: change provider to mock-tts.
    - id: test-voice
      provider: openai-alloy

  scenarios:
    - file: scenarios/voice-selfplay.scenario.yaml

  self_play:
    personas:
      - file: personas/test-user.persona.yaml
    roles:
      - id: selfplay-user
        provider: openai-gpt4o-mini-text

  defaults:
    temperature: 0.7
    max_tokens: 1000
    concurrency: 1
    output:
      dir: out
      formats:
        - json
        - markdown
```

The `self_play.roles` entry defines the `selfplay-user` role that the scenario in step 5 uses. Its
`provider` must be a text LLM provider from `providers:`.

### 2. Create the Provider Files

The duplex model:

```yaml
# providers/gemini-live.provider.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: gemini-live
spec:
  id: gemini-live
  type: gemini
  model: gemini-2.0-flash-exp
  additional_config:
    response_modalities:
      - AUDIO
```

The text model that writes the self-play user's lines:

```yaml
# providers/openai-gpt4o-mini-text.provider.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai-gpt4o-mini-text
spec:
  id: openai-gpt4o-mini-text
  type: openai
  model: gpt-4o-mini
```

The real TTS vendor:

```yaml
# providers/openai-alloy.provider.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai-alloy
spec:
  id: openai-alloy
  type: openai
  role: tts
  voice: alloy
  sample_rate: 24000
```

The mock TTS for CI:

```yaml
# providers/mock-tts.provider.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: mock-tts
spec:
  id: mock-tts
  type: mock
  role: tts
  sample_rate: 24000
```

### 3. Create the Prompt Configuration

The scenario's `task_type` must match the `task_type` of a loaded prompt config.

```yaml
# prompts/voice-assistant.prompt.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: voice-assistant
spec:
  task_type: voice-assistant
  version: "1.0.0"
  description: "Voice assistant"
  system_template: |
    You are a helpful voice assistant. Keep answers to one to three
    short sentences, suitable for speech.
```

### 4. Create a Persona for Self-Play

Assign a voice ID from the catalog to the persona. The runtime resolves it to the
correct TTS provider at run time.

```yaml
# personas/test-user.persona.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Persona
metadata:
  name: test-user
spec:
  id: test-user
  voice: test-voice
  description: "Curious user asking follow-up questions"
  system_template: |
    You are testing a voice assistant. Ask natural follow-up
    questions based on the assistant's responses. Keep questions
    brief and conversational.
```

### 5. Create the Self-Play Scenario

The scenario references the persona by ID. No inline `tts:` block is needed: the
voice is resolved through the catalog.

```yaml
# scenarios/voice-selfplay.scenario.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: voice-selfplay
spec:
  id: voice-selfplay
  task_type: voice-assistant
  description: "Voice assistant tested by a self-play persona"
  duplex:
    timeout: "5m"
    turn_detection:
      mode: vad
      vad:
        silence_threshold_ms: 1200
        min_speech_ms: 500
    resilience:
      partial_success_min_turns: 2
      ignore_last_turn_session_end: true
  turns:
    - role: user
      parts:
        - type: audio
          media:
            file_path: audio/greeting.pcm
            mime_type: audio/L16
    - role: selfplay-user
      persona: test-user
      turns: 3
```

### 6. Run the Test

```bash
export GEMINI_API_KEY="your-key"
export OPENAI_API_KEY="your-key"
promptarena validate config.arena.yaml
promptarena run --scenario voice-selfplay --provider gemini-live
```

`promptarena validate` checks the config and its files before you spend API calls.

### 7. Check the Result

The run writes to `out/`, the `defaults.output.dir` from step 1. Open `out/results.md` for the
markdown report, and `out/index.json` for the machine-readable results. Look for the
`voice-selfplay` run against `gemini-live` and check that its assertions passed.

## CI vs Recording Mode

Voice IDs are declared in one place (`voices:` in the arena config), so switching
between real TTS and a mock is a one-line change:

```yaml
voices:
  # Recording mode (requires OPENAI_API_KEY):
  - id: test-voice
    provider: openai-alloy

  # CI / keyless mode, swap to:
  # - id: test-voice
  #   provider: mock-tts
```

## Tuning Turn Detection

If turns are cutting off early or late, adjust VAD settings:

| Issue | Solution |
|-------|----------|
| Cuts off mid-sentence | Increase `silence_threshold_ms` to 1500-2000 |
| Long pauses before response | Decrease `silence_threshold_ms` to 800-1000 |
| Short utterances ignored | Decrease `min_speech_ms` to 200-300 |

## Adding Assertions

Add `assertions:` to the self-play turn in `scenarios/voice-selfplay.scenario.yaml`:

```yaml
    - role: selfplay-user
      persona: test-user
      turns: 3
      assertions:
        - type: content_matches
          params:
            pattern: ".{10,}"   # regex; the turn output must contain 10 or more characters
```

`content_includes` takes `params.patterns`, a list in which every entry must appear. See
[Assertions](/arena/reference/assertions/) for every assertion type.

## See Also

- [Tutorial 6: Duplex Voice Testing](/arena/tutorials/06-duplex-testing/) - Complete learning path
- [Duplex Configuration Reference](/arena/reference/duplex-config/) - All configuration options
- [Duplex Architecture](/arena/explanation/duplex-architecture/) - How duplex streaming works
