---
title: Duplex Configuration Reference
description: Scenario fields, voice catalog, audio input format and pipeline stages for duplex (bidirectional) streaming runs.
verified:
  commit: c67065389ccbc6e76babad54b4345e1c76bb633e
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/persona.go
    - arena/arenaconfig/types.go
    - arena/arenaconfig/voice.go
    - arena/engine/composite_conversation_executor.go
    - arena/engine/conversation_executor.go
    - arena/engine/duplex_conversation_executor.go
    - arena/engine/duplex_executor_assertions_integration.go
    - arena/engine/duplex_executor_pipeline_integration.go
    - arena/engine/duplex_executor_turns_integration.go
    - arena/engine/duplex_executor_types.go
    - arena/selfplay/registry.go
    - arena/turnexecutors/audio_file_source.go
    - arena/turnexecutors/media_validator.go
    - schemas/v1alpha1/provider.json
    - schemas/v1alpha1/scenario.json
---
Complete reference for configuring [duplex](https://promptkit.altairalabs.ai/glossary#duplex) (bidirectional) streaming scenarios in PromptArena.

## Overview

Duplex mode streams audio in chunks and detects turn boundaries dynamically with [VAD](https://promptkit.altairalabs.ai/glossary#vad) or [ASM](https://promptkit.altairalabs.ai/glossary#asm).

Duplex runs need a provider that supports stream input (`providers.StreamInputSupport`). Gemini (type `gemini`), OpenAI realtime models (model name containing `realtime`), the replay provider and the mock provider support it.

---

## Scenario Configuration

The `duplex` field of the scenario spec enables duplex mode:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: voice-assistant-test
spec:
  id: voice-assistant-test
  task_type: voice-assistant
  description: "Voice assistant duplex test"

  duplex:
    timeout: "5m"
    turn_detection:
      mode: asm
    resilience:
      max_retries: 2
      partial_success_min_turns: 2
```

---

## DuplexConfig

The main duplex configuration object.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `timeout` | string | `"10m"` | Maximum session duration (Go duration format) |
| `turn_detection` | [TurnDetectionConfig](#turndetectionconfig) | Omitted: client-side VAD | Turn boundary detection settings |
| `resilience` | [DuplexResilienceConfig](#duplexresilienceconfig) | See below | Error handling and retry behavior |

### Example

```yaml
duplex:
  timeout: "5m30s"
  turn_detection:
    mode: vad
    vad:
      silence_threshold_ms: 600
      min_speech_ms: 200
  resilience:
    max_retries: 2
```

---

## TurnDetectionConfig

Configures how turn boundaries are detected during duplex streaming.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `mode` | string | `"asm"` | Detection mode: `"vad"` or `"asm"` (applies when the `turn_detection` block is present) |
| `vad` | [VADConfig](#vadconfig) | - | Voice activity detection settings (when mode is `vad`) |

### Turn Detection Modes

| Mode | Name | Description |
|------|------|-------------|
| `asm` | Provider-Native | The provider handles turn detection internally with its own automatic speech detection (for example Gemini's `automaticActivityDetection`). The arena adds no `AudioTurnStage`. |
| `vad` | Voice Activity Detection | Client-side VAD with configurable silence thresholds. The arena adds an `AudioTurnStage`. |

Turn detection defaults to client-side VAD when the `turn_detection` block is omitted. A `turn_detection` block whose `mode` is empty or `asm` uses ASM.

```yaml
duplex:
  turn_detection:
    mode: vad
    vad:
      silence_threshold_ms: 600
      min_speech_ms: 200
      max_turn_duration_s: 60
```

See [Duplex Architecture](/arena/explanation/duplex-architecture/) for how each mode works.

---

## VADConfig

Voice Activity Detection configuration (used when `turn_detection.mode` is `"vad"`).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `silence_threshold_ms` | int | `800` | Silence duration (ms) to trigger turn end |
| `min_speech_ms` | int | `200` | Minimum speech duration before silence counts |
| `max_turn_duration_s` | int | `30` | Force turn end after this duration (seconds) |

### Example

```yaml
duplex:
  turn_detection:
    mode: vad
    vad:
      silence_threshold_ms: 800   # Longer silence for natural speech pauses
      min_speech_ms: 300          # Short utterances still count
      max_turn_duration_s: 30     # Limit long turns
```

### Tuning Guidelines

| Scenario | silence_threshold_ms | min_speech_ms |
|----------|---------------------|---------------|
| Quick responses | 400-500 | 150-200 |
| Natural conversation | 600-800 | 200-300 |
| TTS with pauses | 1000-1500 | 500-800 |
| Slow/deliberate speech | 1200-2000 | 800-1000 |

---

## DuplexResilienceConfig

Error handling and retry behavior for duplex sessions.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_retries` | int | `0` | Retry attempts for failed turns |
| `retry_delay_ms` | int | `1000` | Delay between retries (ms) |
| `partial_success_min_turns` | int | `1` | Minimum completed turns for partial success |
| `ignore_last_turn_session_end` | bool | `true` | Treat session end on final turn as success |

### Example

```yaml
duplex:
  resilience:
    max_retries: 2
    retry_delay_ms: 2000
    partial_success_min_turns: 3
    ignore_last_turn_session_end: true
```

### Partial Success

When `partial_success_min_turns` is set, sessions that end unexpectedly after completing at least that many turns are treated as successful:

```yaml
resilience:
  partial_success_min_turns: 2  # Accept if 2+ turns complete
```

### Session End Handling

With `ignore_last_turn_session_end: true` (the default), a session that ends on the final expected turn is treated as success:

```yaml
resilience:
  ignore_last_turn_session_end: true   # Default
```

With `false`, the final turn must complete without session termination.

---

## Voice Catalog

[Text-to-speech (TTS)](https://promptkit.altairalabs.ai/glossary#tts) for self-play audio generation comes from the
arena voice catalog. TTS providers are declared in `tts_providers:` and bound to voice IDs in
`voices:`. Personas and scripted-text scenarios reference those IDs.

### Arena-Level Declaration

```yaml
# config.arena.yaml
spec:
  tts_providers:
    - file: providers/openai-alloy.provider.yaml
    - file: providers/mock-tts.provider.yaml

  voices:
    - id: alloy
      provider: openai-alloy
```

### Persona Voice Assignment

A persona's `voice` field holds a voice ID. Self-play turns using that persona use the corresponding TTS provider.

```yaml
# personas/curious-customer.persona.yaml
spec:
  id: curious-customer
  voice: alloy   # references the voice catalog id above
  system_template: |
    You are a curious customer ...
```

### Scripted-Text Scenario Voice Assignment

Scripted-text duplex scenarios (turns with `content:` instead of audio `parts:`) declare `voice:` at the scenario level:

```yaml
spec:
  id: my-scripted-scenario
  voice: alloy   # references the voice catalog id above
  turns:
    - role: user
      content: "Hello, can you hear me?"
```

### CI vs Recording Mode

The `provider` of a `voices:` entry selects real vendor TTS or mock TTS. Personas and scenarios reference the voice ID only:

```yaml
voices:
  # Real vendor TTS:
  - id: alloy
    provider: openai-alloy

  # Mock TTS:
  # - id: alloy
  #   provider: mock-tts
```

---

## Audio Turn Parts

In duplex scenarios, user turns contain audio parts instead of text:

```yaml
turns:
  - role: user
    parts:
      - type: audio
        media:
          file_path: audio/greeting.pcm
          mime_type: audio/L16
```

### Audio Requirements

| Parameter | Value | Description |
|-----------|-------|-------------|
| Format | `.wav`, `.pcm` or `.raw` | `AudioFileSource` parses the WAV header and extracts the data chunk; raw files carry no header. Other extensions are rejected with `unsupported audio format: ... (supported: .wav, .pcm, .raw)`. |
| Sample Rate | 16000 Hz | Every chunk read from a file, WAV or raw, is labelled 16000 Hz; the WAV header's rate is not used. The Gemini encoder accepts only 16000 Hz |
| Bit Depth | 16-bit | Raw `.pcm`/`.raw` files hold 16-bit samples. WAV files with 24-bit, 32-bit or float32 samples are converted to 16-bit PCM by `AudioFileSource` |
| Channels | Mono | Every chunk is labelled one channel, WAV or raw; audio is not downmixed |
| MIME Type | any `audio/` type | `mime_type` is required by the schema and needs an `audio/` prefix where validated. Decoding follows the file extension; `mime_type` labels the stored audio |

Source files are 16 kHz mono: 16-bit for raw files, 16/24/32-bit or float32 for WAV.

### Converting Audio Files

```bash
# WAV to raw PCM
ffmpeg -i input.wav -f s16le -ar 16000 -ac 1 output.pcm

# MP3 to PCM
ffmpeg -i input.mp3 -f s16le -ar 16000 -ac 1 output.pcm

# Check a WAV file's sample rate, channels and sample format
ffprobe -show_streams input.wav
```

---

## Provider Configuration

A Gemini provider for duplex:

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

  defaults:
    temperature: 0.7
    max_tokens: 1000

  # Gemini-specific configuration
  additional_config:
    response_modalities:
      - AUDIO   # Returns audio + text transcription
```

### Response Modalities

| Modality | Description |
|----------|-------------|
| `AUDIO` | Returns audio response with text transcription |
| `TEXT` | Returns text-only response (no audio) |

The Gemini Live API supports only one modality at a time; requesting both `TEXT` and `AUDIO` returns an error at setup. `AUDIO` mode includes text transcription via `outputAudioTranscription`. `TEXT` is the default when nothing is set.

---

## Complete Scenario Example

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: voice-assistant-comprehensive

spec:
  id: voice-assistant-comprehensive
  task_type: voice-assistant
  description: "Full duplex voice assistant test with self-play"

  duplex:
    timeout: "5m"
    turn_detection:
      mode: vad
      vad:
        silence_threshold_ms: 800
        min_speech_ms: 250
        max_turn_duration_s: 45
    resilience:
      max_retries: 2
      retry_delay_ms: 2000
      partial_success_min_turns: 3
      ignore_last_turn_session_end: true

  turns:
    # Initial audio greeting
    - role: user
      parts:
        - type: audio
          media:
            file_path: audio/greeting.pcm
            mime_type: audio/L16
      assertions:
        - type: content_matches
          params:
            pattern: "(?i)(hello|hi|welcome)"

    # Self-play generates follow-up questions; voice is resolved from the persona
    - role: selfplay-user
      persona: curious-customer
      turns: 3
      assertions:
        - type: content_matches
          params:
            pattern: ".{20,}"  # At least 20 chars

  conversation_assertions:
    - type: content_includes_any
      params:
        patterns:
          - "help"
          - "assist"
          - "support"
```

A self-play turn's `role` is the `id` of an entry in the arena's `self_play.roles` list. This example assumes the arena config declares a role with the id `selfplay-user`, as `examples/voice-refund-demo/config.arena.yaml` does.

---

## Pipeline Stages

The duplex executor builds the pipeline from these stages, in this order. A stage with a condition is added only when that condition holds.

| Stage | Added when | Role |
|-------|------------|------|
| `AudioPacingStage` (`audio-pacing`) | The arena config has no enabled `self_play` section, or an audio monitor is attached | Emits each input audio chunk at the real-time cadence implied by its size and sample rate |
| `MonitorTap` (input) | An audio monitor is attached | Copies input audio to the audio monitor |
| `AudioResampleStage` | Always | Resamples input audio to the provider's preferred sample rate (Gemini 16000 Hz, OpenAI realtime 24000 Hz); passes audio through when the rates match |
| `AudioTurnStage` | Client-side VAD: [`turn_detection.mode`](#turndetectionconfig) is `vad`, or the `turn_detection` block is omitted | Detects turn boundaries with the [VAD settings](#vadconfig) |
| `VariableProviderStage` | Always | Supplies the merged scenario variables |
| `PromptAssemblyStage` | Always | Loads the prompt template into the shared `TurnState` (`Template`, `AllowedTools`, `Validators`) |
| `TemplateStage` | Always | Renders the system prompt into `TurnState.SystemPrompt` |
| `DuplexProviderStage` | Always | Opens a `StreamInputSupport` session, reading `TurnState.SystemPrompt` at session creation |
| `AudioPacingStage` (`audio-pacing-output`) | Same condition as the input `AudioPacingStage` | Emits each response audio chunk at real-time cadence |
| `MonitorTap` (output) | An audio monitor is attached | Copies response audio to the audio monitor |
| `MediaExternalizerStage` | Media storage is set | Writes all media to storage, retained |
| `ArenaStateStoreSaveStage` | A state store is configured | Saves the conversation messages to the state store |

An audio monitor is attached when `promptarena run` or `promptarena serve` has `--audio-monitor on`, or `--audio-monitor auto` (the default) with stdout on a terminal. Runs started by `promptarena run` set media storage to the `media` directory under the output directory, and configure a state store.

---

## Validation Errors

Common configuration errors and solutions:

| Error | Cause | Solution |
|-------|-------|----------|
| `invalid duplex timeout format` | Timeout not in Go duration format | Use format like `"5m"`, `"30s"`, `"1h30m"` |
| `invalid turn detection mode` | Mode not `vad` or `asm` | Use `mode: vad` or `mode: asm` |
| `silence_threshold_ms must be non-negative` | Negative VAD threshold | Use zero or a positive value |
| `voices[<voice id>]: provider id "<provider>" not found in tts_providers` | Voice references an unknown TTS provider ID | Check that `tts_providers:` lists the provider file and the `id:` matches |
| `voices[N]: id is required` | A `voices:` entry has no `id` | Set `id` |
| `voices[<id>]: provider is required` | A `voices:` entry has no `provider` | Set `provider` |
| `persona <name>: voice id "<id>" not found in spec.voices` | Persona references a voice ID not declared in `voices:` | Add the voice binding to the arena `voices:` list |
| `scenario <name>: voice id "X" not found in spec.voices` | Scenario references an undeclared voice ID | Add the voice binding to the arena `voices:` list |
| `voice "X" binds to provider "Y" which is not loaded in spec.tts_providers` | Scenario voice binds to a provider that is not loaded | List the provider file in `tts_providers:` |

---

## See Also

- [Tutorial: Duplex Voice Testing](/arena/tutorials/06-duplex-testing/) - Step-by-step guide
- [Duplex Architecture](/arena/explanation/duplex-architecture/) - How duplex streaming works
- [Assertions Reference](/arena/reference/assertions/) - All assertion types
- [CLI Commands Reference](/arena/reference/cli-commands/) - Command-line options
