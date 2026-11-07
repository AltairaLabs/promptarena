---
title: Session Recording
description: Record Arena scenario runs to JSONL, then load, replay, annotate and evaluate them.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - .github/workflows/ci.yml
    - arena/arenaconfig/helpers.go
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/engine/builder_integration.go
    - arena/engine/conversation_executor.go
    - arena/engine/duplex_executor_pipeline_integration.go
    - arena/engine/engine.go
    - arena/engine/execution.go
    - arena/stages/duplex_recording.go
    - arena/turnexecutors/pipeline_stages_integration.go
    - examples/eval-test/config.arena.yaml
    - examples/session-replay/demo/main.go
    - examples/session-replay/replay-player/main.go
    - schemas/v1alpha1/eval.json
---
Capture a recording of each Arena run, then load it for replay, annotation and evaluation.

See [Session Recording Architecture](/arena/explanation/session-recording/) for how recordings work.

## Prerequisites

- A PromptArena project with a `config.arena.yaml` (`promptarena init` creates one).
- Text and duplex scenarios. A duplex recording holds the user and assistant messages and the provider's audio chunks.
- Go, to load, replay or annotate a recording with the `recording` package.

## Enable Session Recording

Add the recording configuration to your arena config:

```yaml
# arena.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: my-arena

spec:
  defaults:
    output:
      dir: out
      formats: [json, markdown]
      recording:
        enabled: true           # Enable recording
        dir: recordings         # Subdirectory for recordings
```

Run your tests:

```bash
promptarena run --scenario my-test
```

Recordings are saved to `<output directory>/recordings/` as JSONL files: `out/recordings/` here, or under `--out` when you pass it.

## Recording Format

Each line of a recording is one event, `{"seq":N,"event":{...}}`. See [Storage Format](/arena/explanation/session-recording/#storage-format) for the file formats.

```jsonl
{"seq":1,"event":{"type":"message.created","timestamp":"2024-01-15T10:30:00.500Z","session_id":"run-123","conversation_id":"...","data_type":"...","data":{...}}}
```

## Captured Events

An Arena recording holds these event types:

| Event Type | Description |
|------------|-------------|
| `message.created` | One event per message: user, assistant and tool messages, with multimodal content parts |
| `stream.interrupted` | A stream interruption, with the reason |
| `audio.output` | One event per assistant audio chunk in a duplex run: inline PCM (`audio/pcm`) in `payload.inline_data`, with sample rate, channels, encoding and duration in `metadata` |

A recording holds no stage, pipeline, tool or provider events.

In a duplex run, the recorder sits after the pack guardrails, so a recorded assistant message holds the enforced content, not what the agent tried to say.

Tool calls appear inside `message.created` events. Assistant messages carry `ToolCalls` (id, name, args) and tool messages carry `ToolResult`.

### Multimodal Content in Events

The `message.created` event includes a `Parts` field (`[]ContentPart`) for multimodal messages. Each `ContentPart` has a `Type` field indicating the content kind:

- `text`: text content with a `Text` field
- `image`: image content with a `Media` field containing MIME type, URL, or inline data
- `audio`: audio content with a `Media` field
- `video`: video content with a `Media` field

## Working with Recordings

### Load and Inspect a Recording

```go
package main

import (
    "fmt"
    "github.com/AltairaLabs/PromptKit/runtime/v2/recording"
)

func main() {
    // Load recording
    rec, err := recording.Load("out/recordings/run-123.jsonl")
    if err != nil {
        panic(err)
    }

    // Print metadata
    fmt.Printf("Session: %s\n", rec.Metadata.SessionID)
    fmt.Printf("Duration: %v\n", rec.Metadata.Duration)
    fmt.Printf("Events: %d\n", rec.Metadata.EventCount)
    fmt.Printf("Provider: %s\n", rec.Metadata.ProviderName)
    fmt.Printf("Model: %s\n", rec.Metadata.Model)

    // Iterate events
    for _, event := range rec.Events {
        fmt.Printf("[%v] %s\n", event.Offset, event.Type)
    }
}
```

### Synchronized Playback

Use the ReplayPlayer for time-synchronized event access:

```go
player, _ := recording.NewReplayPlayer(rec)

// Seek to specific position
player.Seek(2 * time.Second)

// Get state at current position
state := player.GetState()
fmt.Printf("Position: %s\n", player.FormatPosition())
fmt.Printf("Current events: %d\n", len(state.CurrentEvents))
fmt.Printf("Messages so far: %d\n", len(state.Messages))
fmt.Printf("Audio input active: %v\n", state.AudioInputActive)
fmt.Printf("Audio output active: %v\n", state.AudioOutputActive)

// Advance through recording
for {
    events := player.Advance(100 * time.Millisecond)
    if player.Position() >= player.Duration() {
        break
    }
    for _, e := range events {
        fmt.Printf("[%v] %s\n", e.Offset, e.Type)
    }
}
```

To export the assistant audio from a duplex recording as a WAV file, build a media timeline and export the output track. A recording holds no user audio, so there is no input track:

```go
timeline, err := rec.ToMediaTimeline(nil)
if err != nil {
    panic(err)
}
if err := timeline.ExportAudioToWAV(events.TrackAudioOutput, "assistant.wav"); err != nil {
    panic(err)
}
```

`GetState()` and `GetStateAt()` return a state with these fields:

| Field | Description |
|-------|-------------|
| `Position` | Current offset from session start |
| `Timestamp` | Absolute timestamp |
| `CurrentEvents` | Events within 50ms of position |
| `RecentEvents` | Events in last 2 seconds |
| `Messages` | Accumulated conversation |
| `AudioInputActive` | User audio present |
| `AudioOutputActive` | Assistant audio present |
| `ActiveAnnotations` | Annotations in scope |

### Add Annotations

Attach annotations to recordings for review and analysis:

```go
import "github.com/AltairaLabs/PromptKit/runtime/v2/annotations"

// Create annotations
anns := []*annotations.Annotation{
    {
        ID:        "quality-1",
        Type:      annotations.TypeScore,
        SessionID: rec.Metadata.SessionID,
        Target:    annotations.ForSession(),
        Key:       "overall_quality",
        Value:     annotations.NewScoreValue(0.92),
    },
    {
        ID:        "highlight-1",
        Type:      annotations.TypeComment,
        SessionID: rec.Metadata.SessionID,
        Target:    annotations.InTimeRange(startTime, endTime),
        Key:       "observation",
        Value:     annotations.NewCommentValue("Good response latency"),
    },
}

// Attach to player
player.SetAnnotations(anns)

// Query active annotations at any position
state := player.GetStateAt(1500 * time.Millisecond)
for _, ann := range state.ActiveAnnotations {
    fmt.Printf("Annotation: %s = %v\n", ann.Key, ann.Value)
}
```

## Replay Provider

Use recordings for deterministic test replay:

```go
import "github.com/AltairaLabs/PromptKit/runtime/v2/providers/replay"

// Create replay provider from recording (nil config uses defaults)
provider, err := replay.NewProvider(rec, nil)
if err != nil {
    panic(err)
}

// Use like any other provider - returns recorded responses
response, err := provider.Predict(ctx, req) // req is a providers.PredictionRequest
```

`replay.NewProviderFromFile(path, cfg)` builds the provider directly from a recording file. See [Session Recording Architecture](/arena/explanation/session-recording/#replay-provider) for the use cases.

## Evaluating Recorded Conversations

Use the **Eval** configuration type to validate and test saved conversations with assertions and LLM judges:

```yaml
# evals/validate-recording.eval.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Eval
metadata:
  name: validate-support-session

spec:
  id: support-validation
  description: Validate customer support conversation quality

  recording:
    path: out/recordings/session-abc123.jsonl
    type: session

  conversation_assertions:
    - type: llm_judge
      params:
        criteria: "Did the agent provide helpful and accurate information?"

    - type: llm_judge
      params:
        criteria: "Was the conversation tone professional and empathetic?"
```

Declare the judge in your arena configuration and reference the eval. A judge is a `{name, provider}` entry in `spec.judges`, where `provider` is the ID of a provider listed in `spec.providers`:

```yaml
# arena.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: evaluation-suite

spec:
  providers:
    - file: providers/openai-gpt4o.provider.yaml

  defaults:
    output:
      dir: out
      formats: [json]

  judges:
    - name: default
      provider: openai-gpt4o

  evals:
    - file: evals/validate-recording.eval.yaml
```

Run evaluations:

```bash
promptarena run --config arena.yaml
```

See the [Eval Configuration Reference](/arena/reference/config-schema#eval-saved-conversation-evaluation) for complete documentation.

## Complete Example

See `examples/session-replay/` for a full working example. `demo` records events, loads them, and plays them back with annotations. `replay-player` loads a recording and takes an optional path:

```bash
cd examples/session-replay
go run ./demo

# Or with your own recording:
go run ./replay-player path/to/recording.jsonl
```

## Recording Storage

### File Organization

```text
out/
└── recordings/
    ├── run-abc123.jsonl     # One file per test run
    ├── run-def456.jsonl
    └── run-ghi789.jsonl
```

There is no retention policy. To keep recordings small:

- Skip recording for quick CI validation runs: set `defaults.output.recording.enabled` to `false`.
- Compress archived recordings. JSONL compresses well.
- Delete old files from `<out>/recordings/` yourself.

See [Session Recording Architecture](/arena/explanation/session-recording/#performance-considerations) for the cost of recording.

### CI/CD Integration

Save recordings as artifacts for debugging failed tests:

```yaml
# .github/workflows/test.yml
- name: Run Arena Tests
  run: promptarena run --ci

- name: Upload Recordings
  if: failure()
  uses: actions/upload-artifact@v4
  with:
    name: session-recordings
    path: out/recordings/
    retention-days: 7
```

## What's Captured vs. What's Not

### Captured in Recordings
- Message content and metadata, including multimodal content parts
- Tool calls with arguments (inside assistant messages) and tool results (inside tool messages)
- Stream interruptions
- Assistant audio chunks from duplex runs

### Not Captured (stored separately)
- User audio (the user's `message.created` event keeps the audio as a content part)
- Stage, pipeline, tool and provider events
- Post-run human feedback (UserFeedback)
- Session tags added after completion
- External annotations (stored in separate files)

## Related pages

- [Duplex/Voice Testing](/arena/how-to/voice/setup-voice-testing/)
- [Session Recording Architecture](/arena/explanation/session-recording/)
- [Configuration Schema](/arena/reference/config-schema/)
