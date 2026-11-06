---
title: Set Up Voice Testing with Self-Play
verified:
  commit: 3df344ffbc8c604e1a8935929c28499d778d19f0
  sources:
    - arena/arenaconfig/helpers.go
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/persona.go
    - arena/arenaconfig/types.go
    - arena/arenaconfig/validator.go
    - arena/arenaconfig/voice.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/cmd/promptarena/validate.go
    - arena/engine/conversation_executor.go
    - arena/engine/duplex_conversation_executor.go
    - arena/engine/duplex_executor_pipeline_integration.go
    - arena/engine/duplex_executor_turns_integration.go
    - arena/engine/engine.go
    - arena/engine/types.go
    - arena/results/markdown/markdown_repository.go
    - arena/selfplay/audio_generator.go
    - arena/selfplay/generator.go
    - arena/selfplay/mock_tts_interactive.go
    - arena/selfplay/registry.go
    - arena/selfplay/tts_registry.go
    - arena/turnexecutors/audio_file_source.go
    - examples/duplex-streaming/config.arena.yaml
    - examples/duplex-streaming/providers/gemini-2-flash.provider.yaml
    - examples/duplex-streaming/providers/mock-duplex.provider.yaml
    - examples/duplex-streaming/providers/mock-tts.provider.yaml
    - examples/duplex-streaming/providers/openai-gpt4o-mini-text.provider.yaml
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/promptconfig.json
    - schemas/v1alpha1/provider.json
    - schemas/v1alpha1/scenario.json
---
Run automated multi-turn voice tests, where a self-play persona speaks through TTS.

## Prerequisites

- For a run against real providers: a Gemini API key (the duplex model) and an OpenAI API key
  (the self-play text model and TTS)
- For a keyless run, for example in CI: no API keys; the project includes mock providers
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
providers/mock-duplex.provider.yaml
personas/test-user.persona.yaml
scenarios/voice-selfplay.scenario.yaml
```

### Prepare the Audio

Copy `greeting.pcm` from `examples/duplex-streaming/audio/`, or convert your own recording
to raw 16 kHz mono 16-bit PCM:

```bash
# WAV or MP3 to raw PCM
ffmpeg -i input.wav -f s16le -ar 16000 -ac 1 audio/greeting.pcm

# Check a WAV file's sample rate, channels and sample format
ffprobe -show_streams input.wav
```

See [Audio Requirements](/arena/reference/duplex-config/#audio-requirements) for the formats a
turn accepts.

### 1. Declare the Arena Config

TTS is configured at the arena level. Declare one or more TTS provider files under
`tts_providers:`, then bind voice IDs in `voices:`. Personas and scenarios reference
those IDs, so one edit to `voices:` swaps between a real vendor and mock TTS.

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
    - file: providers/mock-duplex.provider.yaml              # keyless runs

  tts_providers:
    - file: providers/openai-alloy.provider.yaml  # real TTS
    - file: providers/mock-tts.provider.yaml       # keyless runs

  voices:
    # Keyless runs: change provider to mock-tts (step 6).
    - id: test-voice
      provider: openai-alloy

  scenarios:
    - file: scenarios/voice-selfplay.scenario.yaml

  self_play:
    personas:
      - file: personas/test-user.persona.yaml
    roles:
      - id: selfplay-user
        provider: openai-gpt4o-mini-text  # keyless runs: mock-duplex (step 6)

  defaults:
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

The mock TTS and the mock duplex model for keyless runs. With `auto_respond: true`, the mock
duplex model answers each user turn with `response_text`:

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

```yaml
# providers/mock-duplex.provider.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: mock-duplex
spec:
  id: mock-duplex
  type: mock
  model: mock-duplex-model
  additional_config:
    auto_respond: true
    response_text: "Hello, I'm your voice assistant. How can I help you today?"
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
    short sentences, suitable for speech. End each answer by asking
    what else you can help with.
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
voice is resolved through the catalog. The `conversation_assertions:` entry is the check
step 7 reads: `content_includes` passes when the assistant's final reply contains `help`,
which the prompt in step 3 asks for.

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
  conversation_assertions:
    - type: content_includes
      params:
        patterns: ["help"]
      message: "The final reply should offer more help"
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
promptarena run --scenario voice-selfplay --provider gemini-live --ci
```

Run the commands from the project directory. `promptarena validate` checks the config and its
files before you spend API calls. `--ci` prints plain log output and makes a failed assertion
exit non-zero.

To run without API keys, for example in CI, make two edits to `config.arena.yaml`:

- under `voices:`, change `test-voice` to `provider: mock-tts`
- under `self_play.roles`, change `selfplay-user` to `provider: mock-duplex`

Then run against the mock duplex model:

```bash
promptarena run --scenario voice-selfplay --provider mock-duplex --ci
```

Use these mock provider files rather than `--mock-provider`. That flag's mocks do not
answer duplex audio, so the run waits until `duplex.timeout` and fails.

### 7. Check the Result

The run writes its results to `defaults.output.dir`, `out/` here, relative to the current directory
(`-o` sets another directory). Open `out/results.md`: the `voice-selfplay` row should show `Pass`, and its Conversation
Assertions table should show `content_includes` passed. The per-run JSON file in `out/` holds
the same result under `conversation_assertions`. With `--ci`, a failed assertion makes
`promptarena run` exit non-zero.

To add more checks, append entries to `conversation_assertions:` in the scenario. See
[Assertions](/arena/reference/assertions/) for common types, and the
[Checks Reference](https://promptkit.altairalabs.ai/reference/checks/) for every type.

## Tuning Turn Detection

If turns are cutting off early or late, adjust VAD settings:

| Issue | Solution |
|-------|----------|
| Cuts off mid-sentence | Increase `silence_threshold_ms` to 1500-2000 |
| Long pauses before response | Decrease `silence_threshold_ms` to 800-1000 |
| Short utterances ignored | Decrease `min_speech_ms` to 200-300 |

Starting values by speech style:

| Speech style | `silence_threshold_ms` | `min_speech_ms` |
|--------------|------------------------|-----------------|
| Quick responses | 400-500 | 150-200 |
| Natural conversation | 600-800 | 200-300 |
| TTS with pauses | 1000-1500 | 500-800 |
| Slow or deliberate speech | 1200-2000 | 800-1000 |

## See Also

- [Tutorial 6: Duplex Voice Testing](/arena/tutorials/06-duplex-testing/)
- [Duplex Configuration Reference](/arena/reference/duplex-config/) - Every configuration field
- [Duplex Architecture](/arena/explanation/duplex-architecture/) - How duplex streaming works
