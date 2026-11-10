# Provider Capability Matrix

This document tracks expected capabilities for each provider/model based on official documentation.

## Capability Legend

| Capability | Description |
|------------|-------------|
| text | Basic text completion |
| streaming | Streaming responses |
| vision | Image input understanding |
| audio | Audio input understanding |
| video | Video input understanding |
| tools | Function/tool calling |
| json | JSON mode / structured output |

## What the tables show

Each row is a provider in `config.arena.yaml`. ✅ means the model passed that
scenario in the matrix, and ❌ that it ran and failed. — means the scenario is not run against the model,
because the provider does not declare the capability. ? means the model has not
completed a run yet.

## OpenAI Models

| Model ID | Text | Stream | Vision | Audio | Video | Tools | JSON | Notes |
|----------|------|--------|--------|-------|-------|-------|------|-------|
| gpt-6.1-terra | ✅ | ✅ | ✅ | — | — | ✅ | ✅ | Current frontier; sol is the same-generation sibling |
| gpt-6-sol | ✅ | ✅ | ✅ | — | — | ✅ | ✅ | luna and astra are the same-generation siblings |
| gpt-5.6-luna | ✅ | ✅ | ✅ | — | — | ✅ | ✅ | sol and terra are the same-generation siblings |
| gpt-5.5 | ✅ | ✅ | ✅ | — | — | ✅ | ✅ |  |
| gpt-5.5-pro | ✅ | ✅ | ✅ | — | — | ✅ | ✅ | Responses API only |
| gpt-5.4 | ✅ | ✅ | ✅ | — | — | ✅ | ✅ |  |
| gpt-5.4-mini | ✅ | ✅ | — | — | — | ✅ | ✅ | Vision not declared |
| gpt-5.4-nano | ✅ | ✅ | — | — | — | ✅ | ✅ | Vision not declared |
| gpt-5.2 | ✅ | ✅ | ✅ | — | — | ✅ | ✅ |  |
| gpt-5.2-pro | ✅ | ✅ | ✅ | — | — | ✅ | ✅ | Responses API only |
| o3 | ✅ | ✅ | — | — | — | ✅ | ✅ | Reasoning model; no temperature or top_p |

Retired by OpenAI and removed: o1, o1-pro, o3-mini, o4-mini, gpt-4.1-nano,
gpt-4o and gpt-4o-mini. Superseded and removed: gpt-4.1, gpt-4.1-mini, gpt-5,
gpt-5-mini, gpt-5-nano, gpt-5-pro and gpt-5.1.

OpenAI audio models (`gpt-audio-1.5`, `gpt-audio-mini`) are not in the matrix;
see "Partially Implemented" below.

## Anthropic/Claude Models

| Model ID | Text | Stream | Vision | Audio | Video | Tools | JSON | Notes |
|----------|------|--------|--------|-------|-------|-------|------|-------|
| claude-opus-5-5 | ✅ | ✅ | ✅ | — | — | ✅ | ✅ | Latest Opus |
| claude-sonnet-5-5 | ✅ | ✅ | ✅ | — | — | ✅ | ✅ | Latest Sonnet |
| claude-haiku-5-5 | ✅ | ✅ | ✅ | — | — | ✅ | ✅ | Latest Haiku |
| claude-fable-5-2 | ✅ | ✅ | ✅ | — | — | ✅ | ✅ | Latest Fable |
| claude-opus-5 | ✅ | ✅ | ✅ | — | — | ✅ | ✅ |  |
| claude-sonnet-5 | ✅ | ✅ | ✅ | — | — | ✅ | ✅ |  |
| claude-opus-4-8 | ✅ | ✅ | ✅ | — | — | ✅ | ✅ |  |
| claude-sonnet-4-6 | ✅ | ✅ | ✅ | — | — | ✅ | ✅ |  |

Superseded and removed: Opus 4.5, 4.6 and 4.7, Sonnet 4.5 and Haiku 4.5.

**Note:** Claude API does not support native audio input. Voice features in consumer apps use external STT/TTS.

## Google Gemini Models

| Model ID | Text | Stream | Vision | Audio | Video | Tools | JSON | Notes |
|----------|------|--------|--------|-------|-------|-------|------|-------|
| gemini-4-argon | ? | ? | ? | ? | ? | ? | ? | Added 2026-11-10; not yet run |
| gemini-3.8-flash | ? | ? | ? | ? | ? | ? | ? | Added 2026-11-10; not yet run |
| gemini-3.7-flash | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |  |
| gemini-3.5-flash | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |  |
| gemini-3.5-flash-lite | ? | ? | ? | ? | ? | ? | ? | Added 2026-11-10; not yet run |
| gemini-3.1-pro-preview | ✅ | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ | Vision failed its content assertion on 2026-10-18 |
| gemini-3.1-flash-lite | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |  |

Gemini 2.5 (pro, flash, flash-lite) was deprecated on 2026-10-16 and removed.
The ✅ rows are from the last complete Gemini run, on 2026-10-18.

**Audio formats supported:** wav, mp3, aiff, aac, ogg, flac (up to 9.5 hours, 20MB inline)

## Implementation Status

### Fully Implemented
- **Text, Streaming, Vision, Tools, JSON** - All providers
- **Audio Input** - Gemini only (inline data up to 20MB)
- **Video Input** - Gemini only

### Partially Implemented
- **OpenAI Audio Models** (`gpt-audio-1.5`, `gpt-audio-mini`)
  - The earlier `gpt-4o-audio-preview` / `gpt-4o-mini-audio-preview` models were retired 2026-05-07
  - Implemented via Chat Completions API with `modalities: ["text", "audio"]` parameter
  - Requires `api_mode: completions` in provider config (Responses API doesn't support audio)
  - Supports WAV and MP3 formats only
  - **Not included in the capability matrix.** The matrix's audio scenario is audio-in / text-out with text assertions; `gpt-audio-mini` requires an audio *output* modality, and `gpt-audio-1.5` returns an intermittent 400 (`requires that either input content or output modality contain audio`). Audio capability is exercised via the Gemini providers instead. Revisit if an output-modality-aware audio scenario is added.
  - See: https://platform.openai.com/docs/guides/audio

### Not Yet Implemented
- **OpenAI Realtime API** (WebSocket-based live audio)
  - Different API endpoint and protocol entirely
  - Not applicable for batch testing

### Known Limitations
- Claude/Anthropic has no native audio input support in the API
- OpenAI chat models in the matrix don't declare audio input
- Audio input requires specific model variants or Gemini

## Sources

- [OpenAI Models](https://platform.openai.com/docs/models)
- [OpenAI Audio Guide](https://platform.openai.com/docs/guides/audio)
- [Anthropic Models](https://docs.anthropic.com/en/docs/about-claude/models)
- [Gemini Audio Understanding](https://ai.google.dev/gemini-api/docs/audio)
- [GPT-5 Announcement](https://openai.com/index/introducing-gpt-5/)

## Last Updated

2026-11-10
