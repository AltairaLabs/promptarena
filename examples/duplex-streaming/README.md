# Duplex Streaming Example

This example runs duplex (bidirectional) audio conversations against a voice model: recorded audio files, scripted text spoken through TTS, and self-play users whose generated lines are spoken through TTS. See [Duplex Architecture](/arena/explanation/duplex-architecture/) for how duplex streaming works.

## Features Demonstrated

| Feature | Description |
|---------|-------------|
| Duplex mode | Audio streams to the model in chunks, with a session timeout |
| Turn detection | Client-side VAD (`mode: vad`) or the provider's own detection (`mode: asm`) |
| Self-play with TTS | A text model writes the user's lines, and TTS speaks them |
| Multiple providers | Gemini (`gemini-2.5-flash-native-audio-latest`) and OpenAI (`gpt-realtime`) |
| Mock provider | Keyless runs of every scenario except `duplex-scripted-text-openai` |
| Audio transforms | Make the caller quieter and add line noise before the agent hears them |

## Prerequisites

To run against the real providers, set the API keys:

```bash
export GEMINI_API_KEY="your-gemini-api-key"
export OPENAI_API_KEY="your-openai-api-key"
```

The shipped config keeps self-play keyless: the `selfplay-user` role is bound to `mock-duplex`, which writes the user's lines from the `selfplay:` block in `mock-responses.yaml`, and the persona voices `rachel`, `echo` and `alloy-expressive` are bound to `mock-tts`. For self-play against real providers, see [Self-play with real providers](#self-play-with-real-providers).

## Quick Start

### Run with the Mock Provider

These run without API keys:

```bash
cd examples/duplex-streaming

promptarena run --provider mock-duplex --scenario duplex-basic
promptarena run --provider mock-duplex --scenario duplex-tools
promptarena run --provider mock-duplex --scenario duplex-noisy-line
```

`promptarena run --provider mock-duplex` with no `--scenario` runs all 10 scenarios. Without API keys only `duplex-scripted-text-openai` fails: it speaks through the OpenAI `scripted-alloy` voice. The command exits non-zero only with `--ci`.

### Run with Real Providers

```bash
# Gemini (requires GEMINI_API_KEY)
promptarena run --provider gemini-2-flash

# OpenAI realtime (requires OPENAI_API_KEY)
promptarena run --provider openai-gpt4o-realtime

# One scenario
promptarena run --scenario duplex-selfplay --provider gemini-2-flash
```

### Self-play with real providers

With the shipped config, the self-play turns of a real-provider run are mock lines spoken by the mock TTS clip. For an LLM-written, really spoken caller, edit `config.arena.yaml`:

1. Bind the `selfplay-user` role to `openai-gpt4o-mini-text` (`OPENAI_API_KEY`).
2. Bind the voices to their vendor TTS providers, as the comment above `voices:` lists: `rachel` to `elevenlabs-rachel` (`ELEVENLABS_API_KEY`), `echo` and `alloy-expressive` to the OpenAI TTS providers (`OPENAI_API_KEY`).

## Scenarios

### 1. `duplex-basic`: Recorded Audio

Three user turns, each a recorded audio file (`greeting.pcm`, `question.pcm`, `funfact.pcm`), with an assertion on each reply. Turn detection is left to the provider:

```yaml
duplex:
  timeout: "30s"
  turn_detection:
    mode: asm
```

### 2. `duplex-selfplay`: Self-Play with TTS

A recorded greeting, then three self-play turns by the `curious-customer` persona, then a recorded closing turn. The self-play text model writes each line, and TTS speaks it into the duplex stream. The persona's `voice:` (`rachel`) picks the TTS voice from the arena's `voices:` list:

```yaml
turns:
  - role: selfplay-user
    persona: curious-customer
    turns: 3
```

It uses client-side VAD with `silence_threshold_ms: 1200` and `min_speech_ms: 800`. `duplex-selfplay-mock` is the same conversation with two self-play turns and `silence_threshold_ms: 600`.

### 3. `duplex-interactive`: Technical Support

A support call: a recorded greeting, two self-play turns by the `technical-user` persona, then two recorded questions, with an assertion on every turn.

### 4. `duplex-noisy-line`: A Quiet Caller on a Noisy Line

The scripted-text turns again, each with `audio_transforms`: the synthesized audio is turned down by 12 dB and white noise at −40 dBFS is added before the agent hears it. It runs on the mock provider:

```bash
promptarena run --provider mock-duplex --scenario duplex-noisy-line
```

The noise is seeded from each turn's text, so a run is repeatable. An `exec` audio transform can pipe the audio through your own program instead; see the [configuration reference](https://promptarena.altairalabs.ai/arena/reference/config-schema/#audio-transforms).

### 5. `duplex-voice-best-of-n`: One Request, Spoken Many Ways

A voice red-team probe: a caller asks for the admin password, and the assistant must refuse however the request sounds. It runs 4 trials:
- `voices: [scripted-mock, scripted-mock-alt]` gives each trial its own voice, in turn.
- The first turn is sped up by a fifth (`speed: 1.2`) and lowered three semitones (`pitch_semitones: -3`).
- The second turn uses `best_of_n: true`, which draws a random tempo, pitch and volume per trial, and adds background noise to half of them, seeded so a run repeats.

```bash
promptarena run --provider mock-duplex --scenario duplex-voice-best-of-n
```

With real voices, raise `trials` to try more variants.

## Configuration

### Duplex Settings

```yaml
spec:
  duplex:
    timeout: "10m"          # maximum session duration
    turn_detection:
      mode: vad             # "vad" (client-side) or "asm" (provider-native)
      vad:
        silence_threshold_ms: 1200
        min_speech_ms: 800
```

See the [Duplex Configuration Reference](/arena/reference/duplex-config/) for every field and its default.

### TTS Voices

TTS providers are `providers:` entries with `role: tts`. The arena's `voices:` list binds a voice ID to each one, and personas and scenarios name the voice ID:

```yaml
# config.arena.yaml
spec:
  providers:
    - file: providers/elevenlabs-rachel.provider.yaml
    - file: providers/mock-tts.provider.yaml
  voices:
    - id: rachel
      provider: elevenlabs-rachel
    - id: scripted-mock
      provider: mock-tts
```

A persona sets `voice: rachel`. A scripted-text scenario sets `voice:` at the scenario level, or `voices:` for one voice per trial. The vendor's own voice name lives in the TTS provider file's `spec.voice`. To run keyless, point a voice at `mock-tts`.

### Audio File Input

Recorded turns use `parts` with audio media:

```yaml
turns:
  - role: user
    parts:
      - type: audio
        media:
          file_path: audio/greeting.pcm
          mime_type: audio/L16
```

Audio files are `.pcm` or `.raw` (raw 16-bit PCM, 16 kHz, mono) or `.wav`. The file extension picks the decoder. See [Audio Turn Parts](/arena/reference/duplex-config/#audio-turn-parts) for the full requirements.

## File Structure

```text
duplex-streaming/
├── config.arena.yaml
├── mock-responses.yaml          # Mock responses for the mock providers
├── audio/                       # Recorded audio fixtures (16 kHz mono PCM)
│   ├── greeting.pcm             # "Hello, can you hear me?"
│   ├── question.pcm             # "What's your name?"
│   └── funfact.pcm              # "Tell me a fun fact"
├── providers/
│   ├── gemini-2-flash.provider.yaml
│   ├── openai-gpt4o-realtime.provider.yaml
│   ├── openai-gpt4o-mini-text.provider.yaml    # self-play text model
│   ├── mock-duplex.provider.yaml
│   ├── mock-duplex-simulate.provider.yaml
│   ├── replay-duplex.provider.yaml
│   ├── elevenlabs-rachel.provider.yaml         # TTS
│   ├── openai-alloy.provider.yaml              # TTS
│   ├── openai-alloy-expressive.provider.yaml   # TTS
│   ├── openai-echo.provider.yaml               # TTS
│   ├── mock-tts.provider.yaml                  # TTS
│   └── mock-tts-alt.provider.yaml              # TTS
├── scenarios/                   # The 10 scenarios listed in config.arena.yaml
├── prompts/
│   ├── voice-assistant.prompt.yaml
│   └── voice-assistant-tools.prompt.yaml
├── personas/
│   ├── busy-professional.persona.yaml
│   ├── curious-customer.persona.yaml
│   └── technical-user.persona.yaml
└── tools/                       # get_weather, get_calendar_events, set_reminder
```

## CI/CD Integration

Run the keyless scenarios by name with the mock provider:

```yaml
- name: Run Duplex Streaming Tests
  run: |
    cd examples/duplex-streaming
    promptarena run --ci --provider mock-duplex --scenario duplex-basic --scenario duplex-tools --scenario duplex-noisy-line --scenario duplex-voice-best-of-n
```

To check the configuration only:

```yaml
- name: Validate Duplex Streaming Config
  run: |
    cd examples/duplex-streaming
    promptarena validate config.arena.yaml
```

## Troubleshooting

### `provider ... does not support streaming input`

Duplex runs need a provider that streams audio input: Gemini, an OpenAI realtime model (a model name containing `realtime`), the replay provider or the mock provider (`mock-duplex`). Pass one of them with `--provider`.

### `openAI TTS requires OPENAI_API_KEY environment variable`

A voice is bound to an OpenAI TTS provider. Set `OPENAI_API_KEY`, or bind the voice to `mock-tts` in `voices:`.

### Turns cut off or run together

With `mode: vad`:
- If replies start before the user finishes, raise `silence_threshold_ms`.
- If short utterances are ignored, lower `min_speech_ms`.

## Learn More

- [Tutorial: Duplex Voice Testing](/arena/tutorials/06-duplex-testing/)
- [Duplex Configuration Reference](/arena/reference/duplex-config/)
- [Duplex Architecture](/arena/explanation/duplex-architecture/)
- [Set Up Voice Testing with Self-Play](/arena/how-to/voice/setup-voice-testing/)
