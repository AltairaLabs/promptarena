---
title: Use the Web UI
description: Run Arena's local web interface for live run monitoring, a results dashboard, and browser-based chat.
sidebar:
  order: 20
verified:
  commit: 02ec7c860b978fcce24022d9ccb868d94208c751
  sources:
    - arena/arenaconfig/mock_config_check.go
    - arena/cmd/promptarena/chat.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/engine/engine.go
    - arena/selfplay/registry.go
    - arena/web/embed.go
    - arena/web/event_adapter.go
    - arena/web/frontend/src/App.tsx
    - arena/web/frontend/src/audio/player.ts
    - arena/web/frontend/src/components/HistoricalResults.tsx
    - arena/web/frontend/src/components/InteractiveChat.tsx
    - arena/web/frontend/src/components/arena/CommandStrip.tsx
    - arena/web/frontend/src/components/arena/FieldPicker.tsx
    - arena/web/frontend/src/components/arena/Hero.tsx
    - arena/web/frontend/src/components/arena/InstrumentBand.tsx
    - arena/web/frontend/src/components/arena/Standings.tsx
    - arena/web/frontend/src/components/arena/TopBar.tsx
    - arena/web/frontend/src/components/arena/TrialMatrix.tsx
    - arena/web/frontend/src/hooks/useArenaAPI.ts
    - arena/web/frontend/src/hooks/useArenaEvents.ts
    - arena/web/frontend/src/hooks/useInteractiveChat.ts
    - arena/web/frontend/src/lib/arenaInspectorTabs.tsx
    - arena/web/frontend/src/lib/atlasAdapter.ts
    - arena/web/interactive.go
    - arena/web/server.go
    - arena/web/server_loader_integration.go
    - arena/web/server_picker_integration.go
    - arena/web/spa.go
---

Arena ships with a local web interface that serves an embedded React single-page
app backed by a REST API and Server-Sent Events (SSE). Use it to browse results,
watch runs stream live, kick off runs from the browser, and hold an interactive
chat with a configured agent.

The server binds to the loopback interface only. It is a local development tool,
not something to expose on a network.

## Start the Server

Point `serve` at a directory containing `config.arena.yaml` (or the config file
itself). Add `--open` to launch your browser automatically:

```bash
promptarena serve --open
```

If `config-path` is a directory, Arena looks for `config.arena.yaml` inside it. With
no argument it loads the config from the current directory:

```bash
promptarena serve                 # load config.arena.yaml from the current dir
promptarena serve ./my-scenario   # load from a specific directory
```

The server prints its URL and config path and runs until you press `Ctrl+C`.

### Choose a Port

The default port is **8080**, so the UI lives at `http://localhost:8080`. Override
it with `-p`/`--port`:

```bash
promptarena serve --port 3000
```

If the port is in use, Arena tries the next consecutive port (up to 20 ports) and
prints the one it picked. If none is free, it exits with an error; retry with `-p`.

```text
Port 8080 is in use — serving on 8081 instead
Arena Web UI: http://localhost:8081
```

## The Dashboard

The UI opens on the dashboard. It reads the loaded config and any saved results
(Arena hydrates `*.json` files from the config's output directory on startup)
and shows:

- The loaded configuration: providers and scenarios.
- A results list of completed runs, each opening a detailed results viewer.
- Controls to start a new run and to clear stored results.

![The Arena dashboard: a scenario picker with the run cost and duration estimate, an overall pass-rate dial, trial/spend/latency/token tiles, a provider standings table, and a trial matrix of scenario by provider with per-cell pass rate, cost and duration.](/screenshots/web-dashboard.png)

Select a trial to open the detailed results viewer for that run: the transcript
with any tool calls and their results inline, per-turn token, cost and latency
figures, and a scrubber that replays the conversation turn by turn.

![A run detail view for one scenario and provider, showing message, tool-call, token, cost and latency totals along the top, a transcript containing a get_order_status tool call with its JSON result and the assistant reply, and a replay timeline along the bottom.](/screenshots/web-run-detail.png)

## Monitor a Live Run

The UI subscribes to `GET /api/events`, an SSE stream of run events emitted by the
engine as scenarios execute. Turn-by-turn messages (with token and cost figures)
and provider-call errors appear in the browser as they happen. The stream
reconnects automatically if dropped. See [Interfaces](/arena/explanation/interfaces/)
for how the stream works.

The stream also carries `stage.started`, `stage.completed` and `stage.failed`
pipeline events. Open a run to see them on its **Trace** tab: one row per stage
execution, in progress until the stage finishes, then with its duration, and a
failed stage with its error. The tab appears only for runs whose stage events
this browser tab received live; a result loaded from disk after a reload has
none.

Scripts can read the same stream (`curl -N http://localhost:8080/api/events`).
Stage events carry the payload `{name, stageType, duration, error}`.

![The dashboard mid-run: the status badge in the header reads Live, and the trial count, spend, latency and pass-rate figures climb as results arrive.](/screenshots/web-run-live.png)

The header status badge reads Online when connected, Live while a run is in
flight and Offline when the stream is disconnected. The summary tiles update as
each trial lands.

## Start a Run from the Browser

Use the run controls to select the providers and scenarios to execute and a
repeat count (`Run the field xN`, 1 to 25), then start the run. This issues
`POST /api/run`, which launches the run in the background (up to 4 concurrent
runs, with a 30-minute timeout per sweep) and returns immediately. Progress streams back over the SSE connection, and each
completed run is persisted to the output directory so it survives a restart.

## The Interactive Chat Tab

The interactive tab lets you talk to a configured agent directly in the browser,
without authoring a scenario. Pick an agent
and provider, supply any required template variables, and start a session
(`POST /api/interactive/session`). Each message you send goes to
`POST /api/interactive/message`, and the agent's reply renders live over the same
SSE stream. You can optionally enable evals to score the conversation as it runs.

The active agent, provider and connection status sit in a bar above the
transcript rather than in a side panel. Selecting any message opens an inspector
on the right with Overview, Checks, Raw and Tools tabs. `Raw`
shows the exact message object, including the resolved tool descriptors sent to
the provider.

The chat tab also supports a voice call over the WebSocket at
`GET /api/interactive/voice`.

![The Interactive Chat tab: a header bar naming the agent, provider and connection status, the system prompt and a user/assistant exchange in the transcript, and a message inspector open on the right showing the raw JSON for the system message.](/screenshots/web-chat.png)

## Audio Monitoring

For voice scenarios, the server can relay real-time duplex audio (both the
caller and agent sides) to the browser over SSE for live monitoring and playback.
Audio is opt-in per client: the frontend requests it by connecting to
`GET /api/events?audio=1`.

Control audio capture with `--audio-monitor` and `--audio-rate`:

```bash
promptarena serve --audio-monitor on --audio-rate 24000
```

See the [`serve` flags](/arena/reference/cli-commands/) for the accepted values and defaults.

## Free Demos with Mock Providers

Add `--mock-provider` to replace the LLM provider and the self-play role providers
with mocks, so the assistant and self-play turns make no real model calls. TTS and
STT providers are not replaced; bind voices to a `mock` TTS provider to keep audio
free too. Pair it with `--mock-config` to serve canned scenario
responses:

```bash
promptarena serve --mock-provider --mock-config mock-responses.yaml --open
```

`serve` exits at startup if the `--mock-config` file has an unknown key. Tools still run for real. See [Use Mock Providers](/arena/how-to/providers/use-mock-providers/) for the
response file format.

## REST API

The SPA is a thin client over these endpoints, which you can also call from
scripts:

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/events` | SSE stream of live run events; add `?audio=1` for audio relay |
| `GET` | `/api/config` | Loaded Arena config |
| `GET` | `/api/run-options` | Available providers and scenarios |
| `GET` | `/api/workflow` | Workflow topology graph |
| `GET` | `/api/results` | Completed runs as `{run_id, scenario_id, provider_id}` locators |
| `GET` | `/api/results/{id}` | A single run result |
| `DELETE` | `/api/results` | Clear stored results (memory and disk) |
| `GET` | `/api/media/{path...}` | A media artifact, such as recorded audio |
| `POST` | `/api/run` | Start a run; body takes `providers`, `scenarios`, `regions`, `runs`; returns 202, or 400 if nothing matches |
| `GET` | `/api/interactive/options` | Agents and providers for interactive chat |
| `POST` | `/api/interactive/session` | Open a chat session |
| `POST` | `/api/interactive/message` | Send a message to a session |
| `GET` | `/api/interactive/voice` | WebSocket for the chat voice call |
| `GET` | `/` | The embedded React SPA |

## See Also

- [CLI Command Reference](/arena/reference/cli-commands/): full `serve` flag reference
- [Use the TUI](/arena/how-to/interfaces/use-the-tui/): the terminal interface
- [Run in CI](/arena/how-to/interfaces/run-in-ci/): headless, non-interactive runs
- [Use Mock Providers](/arena/how-to/providers/use-mock-providers/): zero-cost demos and tests
