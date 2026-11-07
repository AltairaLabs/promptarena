---
title: Test Voice Tool Calls
description: Assert that a voice agent calls the right tools, with the right arguments, in the right order, during a duplex audio conversation.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - arena/assertions/validate_params.go
    - arena/cmd/promptarena/run.go
    - arena/engine/mock_default_audio.go
    - examples/duplex-streaming/config.arena.yaml
    - examples/duplex-streaming/mock-responses.yaml
    - examples/duplex-streaming/prompts/voice-assistant-tools.prompt.yaml
    - examples/duplex-streaming/providers/gemini-2-flash.provider.yaml
    - examples/duplex-streaming/providers/mock-duplex.provider.yaml
    - examples/duplex-streaming/scenarios/duplex-tools.scenario.yaml
    - examples/duplex-streaming/tools/set_reminder.tool.yaml
    - examples/voice-ivr/mock-responses.yaml
---

This how-to walks through the tool-calling scenarios in `examples/duplex-streaming/`. The agent calls tools during a real-time voice conversation, and conversation-level assertions check the calls across the whole conversation.

## What it proves

A voice agent that calls tools can fail in two ways: it never reaches for the tool when it should, or it calls the tool with the wrong arguments or stalls when the result comes back during ongoing audio output. One scenario covers both:

- A scripted persona drives a sustained voice conversation via self-play + TTS.
- The voice agent under test (OpenAI Realtime, Gemini Live) handles audio and decides when to call tools.
- The tool registry executes real handlers: mock-backed for the demo, real services for production.
- Conversation-level assertions check the tool-call pattern: which tools fired, with what args, in what order, with what results.

## Run it

```bash
cd examples/duplex-streaming
promptarena serve
```

`serve` loads all duplex-streaming scenarios. The tool-call scenario is `duplex-tools`: a busy-professional persona that asks about weather, calendar, and reminders, exercising three tools in a single conversation.

For headless / CI use:

```bash
# Real provider (requires GEMINI_API_KEY)
promptarena run --scenario duplex-tools --provider gemini-2-flash --ci

# Mock provider (keyless; see "Mock mode" below)
promptarena run --scenario duplex-tools --provider mock-duplex --ci
```

The shipped `config.arena.yaml` binds the `selfplay-user` role to `mock-duplex` and the persona's `alloy-expressive` voice to `mock-tts`, so the mock command needs no keys. For an LLM-written, really spoken caller on the real-provider run, bind the role to `openai-gpt4o-mini-text` and the voice to `openai-alloy-expressive` (both `OPENAI_API_KEY`), as the config comments say.

## The assertion shape

`examples/duplex-streaming/scenarios/duplex-tools.scenario.yaml`:

```yaml
turns:
  - role: user
    parts:
      - type: audio
        media:
          file_path: audio/greeting.pcm
          mime_type: audio/L16
    assertions:
      - type: content_matches
        message: "Assistant should greet or offer help"
        params:
          pattern: "(?i)(hello|hi|help|assist|Nova)"

  - role: selfplay-user
    persona: busy-professional
    turns: 4

conversation_assertions:
  - type: tools_called
    params:
      tool_names: [get_weather]
      min_calls: 1
  - type: tools_called
    params:
      tool_names: [get_calendar_events]
      min_calls: 1
  - type: tools_called
    params:
      tool_names: [set_reminder]
      min_calls: 1
```

The headline checks live at the conversation level: each of the three tools fired at least once over the four-turn self-play conversation. Add `tool_calls_with_args` or `tool_call_sequence` to tighten the contract:

```yaml
- type: tool_calls_with_args
  params:
    tool_name: set_reminder
    expected_args:
      time: "in 30 minutes"
- type: tool_call_sequence
  params:
    sequence: [get_calendar_events, set_reminder]
```

`tool_calls_with_args` compares argument values exactly. In the example, the scripted `set_reminder` call passes `time: "in 30 minutes"`.

## CI gate

See [Run in CI](/arena/how-to/interfaces/run-in-ci/) for the general setup.

```yaml
# .github/workflows/voice-tool-calls.yml
name: Voice tool calls

on:
  pull_request:
    paths:
      - 'examples/duplex-streaming/**'

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Validate duplex configs
        working-directory: examples/duplex-streaming
        run: ../../bin/promptarena validate config.arena.yaml

  run-against-gemini:
    runs-on: ubuntu-latest
    if: github.event.pull_request.head.repo.full_name == github.repository
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run duplex-tools scenario
        working-directory: examples/duplex-streaming
        env:
          GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
        run: ../../bin/promptarena run --scenario duplex-tools --provider gemini-2-flash --ci --formats json,markdown
```

The fork-aware `if:` check keeps the secret-bearing job from running on PRs from external forks.

## Mock mode

Arena passes the running scenario's ID to the mock session, so `mock-duplex` replies from the `duplex-tools` block of `mock-responses.yaml`, scripted `tool_calls` included. The tools execute for real and the three conversation-level `tools_called` assertions pass. The run proves the wiring (the duplex session, the tool loop, the assertions), not how a real model chooses tools.

## Extending it

- To add a tool, drop a new `.tool.yaml` into `tools/`, reference it in `config.arena.yaml`, and mention it in the system prompt at `prompts/voice-assistant-tools.prompt.yaml`.
- To check the values the agent passes, replace `tools_called` with `tool_calls_with_args` and list them under `expected_args`.
- To assert call order, add `tool_call_sequence`.
- To catch tools the agent should not invoke in a given path (for example `set_reminder` on a read-only query), add `tools_not_called`.

## Related pages

- [Test a Voice Customer Support Agent with Self-Play](/arena/how-to/voice/voice-customer-support/)
- [Set Up Voice Testing with Self-Play](/arena/how-to/voice/setup-voice-testing/)
