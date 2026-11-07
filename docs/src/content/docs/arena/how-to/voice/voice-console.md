---
title: Use the Voice Console
description: Hold a live, hands-free voice conversation with an Arena agent from the terminal with promptarena chat --voice.
verified:
  commit: 3df344ffbc8c604e1a8935929c28499d778d19f0
  sources:
    - .goreleaser.yml
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - arena/arenaconfig/voice.go
    - arena/cmd/promptarena/chat.go
    - arena/cmd/promptarena/chat_interactive.go
    - arena/engine/duplex_executor_pipeline_integration.go
    - arena/engine/duplex_interactive.go
    - arena/tui/app/app.go
    - arena/tui/app/chatpage.go
    - arena/tui/app/chatpage_voice.go
    - arena/tui/app/launch_interactive.go
    - arena/tui/app/page.go
    - arena/voice/driver.go
    - arena/voice/echo_guard.go
    - arena/voice/portaudio/portaudio_discovery.go
    - arena/voice/portaudio/portaudio_dlopen_windows.go
    - arena/voice/portaudio/portaudio_io_interactive.go
    - arena/voice/session_audioio_interactive.go
    - examples/voice-console-asm/providers/gemini-realtime.provider.yaml
    - npm/promptarena/package.json
---
Run a live, hands-free voice conversation with an Arena agent from the terminal.

## Prerequisites

- The standard `promptarena` binary. Voice is built in; there is no separate voice build. See [Install PromptArena](/arena/how-to/setup/installation/).
- PortAudio installed on the host (see [Install PortAudio](#install-portaudio)).
- Headphones, or `--echo-guard` if you use laptop speakers (see [Echo guard](#echo-guard)). The mic stays open for the whole session, and without either one the speaker audio feeds back into the mic.
- An Arena config with at least one provider and one agent (a `prompt_configs` entry).

## Getting the binary

`promptarena` loads PortAudio at runtime, and only when a voice session starts. Install the binary the usual way (`npm install -g @altairalabs/promptarena`, a release download, or `go build -o bin/promptarena ./arena/cmd/promptarena`), then [install PortAudio](#install-portaudio).

If PortAudio is missing, every other feature works. The voice console shows an `error: open audio device: ...` line with an install hint.

## Install PortAudio

| Platform | Command |
|----------|---------|
| macOS | `brew install portaudio` |
| Debian / Ubuntu | `sudo apt install libportaudio2` |
| Fedora / RHEL | `sudo dnf install portaudio` |
| Windows | place `portaudio.dll` on your `PATH` |

## Start a voice session

```bash
promptarena chat --voice --config config.arena.yaml
```

If the config declares more than one agent or provider, the console first shows a numbered picker. It then opens in full-screen TUI mode. Speak; the agent replies with synthesized audio. Press `q`, `Esc` or `Ctrl-C` to exit.

`chat` also starts a voice session without `--voice` when a scenario in the config has a `duplex:` block or a provider sets `additional_config.realtime: true`. A session ends after 30 minutes idle.

## Mode selection: ASM vs VAD

The console picks how it detects the end of each turn when the session starts:

1. If you pass `--voice-stt`, it uses VAD mode, whatever the provider.
2. Otherwise, if the provider supports streaming audio input, it uses ASM mode.
3. Otherwise it stops with `voice over a text agent requires an STT provider (--voice-stt)`.

### ASM mode (realtime providers, default)

Realtime providers such as OpenAI Realtime and Gemini Live detect turns inside the connection. In ASM (Automatic Speech Mode) the console streams raw PCM from the microphone straight into the provider connection, and the provider signals when a turn ends. No extra flags are needed.

### VAD mode (text / REST providers)

For chat-completion providers without streaming audio input, the console uses client-side VAD (Voice Activity Detection). It records the mic until it detects silence, transcribes the audio with an STT provider, and sends the text as a turn. A TTS provider turns the agent's reply back into audio.

Pass the STT provider id and the TTS voice id:

```bash
promptarena chat --voice \
  --voice-stt openai-whisper \
  --voice-output-voice alloy \
  --config config.arena.yaml
```

`--voice-stt` must match a provider declared under `stt_providers:` in the Arena config. Any other id stops `chat` before the console opens, with an error that lists the STT provider ids the config loads. `--voice-output-voice` must match a voice id declared under `voices:`, and VAD mode needs it. The value is a voice binding id (the `id:` field of a `voices:` entry), which maps to the synthesis voice of the configured TTS provider. Use the binding id, not the vendor's voice name, unless the two are the same.

## Echo guard

Without headphones (for example on laptop speakers), add `--echo-guard`:

```bash
promptarena chat --voice --echo-guard --config config.arena.yaml
```

While the agent speaks, the echo guard drops mic frames quieter than a threshold that follows the playback level: a frame passes only if its RMS level is at least the larger of `0.02` and half the smoothed playback level. The gate stays engaged for 200 ms after the last played audio, and it resets when playback is flushed on a barge-in.

Speaker bleed and quiet speech during playback are dropped. Speech louder than that threshold still gets through, so you can interrupt the agent when `--barge-in` is also set. The guard adds no latency. It is a level gate, not acoustic echo cancellation.

Echo guard is off by default, which assumes headphones. Prefer headphones for the lowest latency and cleanest transcription.

## Barge-in

To interrupt the agent mid-reply, add `--barge-in`:

```bash
promptarena chat --voice --barge-in --config config.arena.yaml
```

Barge-in is off by default. On a realtime provider, the console flushes the agent's queued audio when the provider signals that you started speaking. Use it with headphones or `--echo-guard`; otherwise the agent's own audio can interrupt it.

## Full flag reference

| Flag | Default | Description |
|------|---------|-------------|
| `--voice` | `false` | Hands-free voice mode (requires PortAudio installed) |
| `--voice-stt <id>` | none | STT provider id; selects VAD mode for a text agent |
| `--voice-output-voice <id>` | none | TTS voice binding id the agent speaks in (VAD mode) |
| `--echo-guard` | `false` | Gate the mic while the agent speaks (laptop speakers) |
| `--barge-in` | `false` | Allow interrupting the agent mid-reply |
| `--config <path>` | `config.arena.yaml` | Path to the Arena config file |
| `--mock-provider` | `false` | Replace all providers with a generic mock |
| `--mock-config <path>` | none | Mock response file, used with `--mock-provider` |
| `-v`, `--verbose` | `false` | Debug logging (raw provider events, transcription) |

See [CLI commands](/arena/reference/cli-commands/) for every `chat` flag.

## Related pages

- [Set Up Voice Testing with Self-Play](/arena/how-to/voice/setup-voice-testing/): automated voice testing without a live mic, and declaring TTS providers and `voices:`
- [Install PromptArena](/arena/how-to/setup/installation/): standard installation
