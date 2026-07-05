---
title: Use the Web UI
description: Run Arena's local web interface for live run monitoring, a results dashboard, and browser-based chat.
sidebar:
  order: 20
---

Arena ships with a local web interface that serves an embedded React single-page
app backed by a REST API and Server-Sent Events (SSE). Use it to browse results,
watch runs stream live, kick off runs from the browser, and hold an interactive
chat with a configured agent.

The server binds to **localhost (`127.0.0.1`) only** — it is a local development
tool, not something to expose on a network.

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

The server prints its URL and config path on startup and runs until you press
`Ctrl+C`.

### Choose a Port

The default port is **8080**, so the UI lives at `http://localhost:8080`. Override
it with `-p`/`--port`:

```bash
promptarena serve --port 3000
```

If the port is already in use, Arena exits with a clear error telling you to pick
another with `-p`.

## The Dashboard

Opening the UI lands you on the dashboard. It reads the loaded config and any
previously saved results (Arena hydrates `*.json` files from the config's output
directory on startup) and shows:

- The loaded configuration — providers, scenarios, and regions.
- A results list of completed runs, each opening a detailed results viewer.
- Controls to start a new run and to clear stored results.

:::note[📸 Screenshot needed]
The Arena web dashboard on first load: the config summary, the list of prior run
results, and the "start a run" controls.
:::

## Monitor a Live Run

The UI subscribes to `GET /api/events`, an SSE stream of run events emitted by the
engine as scenarios execute. Turn-by-turn messages, assertion outcomes, and status
updates appear in the browser in real time — no page refresh needed. The stream
stays open for the life of the page and reconnects automatically if dropped.

:::note[📸 Screenshot needed]
A run in progress: the live event stream updating turn by turn with assertion
results as they land.
:::

## Start a Run from the Browser

Use the run controls to select which providers, scenarios, and regions to execute,
then start the run. This issues `POST /api/run`, which launches the run in the
**background** (up to **4 concurrent** runs, each with a **30-minute timeout**) and
returns immediately. Progress streams back over the SSE connection, and each
completed run is persisted to the output directory so it survives a restart.

## The Interactive Chat Tab

The interactive tab lets you talk to a configured agent directly in the browser —
handy for exploring prompt behaviour without authoring a scenario. Pick an agent
and provider, supply any required template variables, and start a session
(`POST /api/interactive/session`). Each message you send goes to
`POST /api/interactive/message`, and the agent's reply renders live over the same
SSE stream. You can optionally enable evals to score the conversation as it runs.

:::note[📸 Screenshot needed]
The interactive chat tab: agent/provider pickers on the left and a live
conversation with a configured agent on the right.
:::

## Audio Monitoring

For voice scenarios, the server can relay **real-time duplex audio** (both the
caller and agent sides) to the browser over SSE for live monitoring and playback.
Audio is opt-in per client — the frontend requests it by connecting to
`GET /api/events?audio=1`.

Control audio capture with the `--audio-monitor` and `--audio-rate` flags:

```bash
promptarena serve --audio-monitor on --audio-rate 24000
```

- `--audio-monitor` — `auto` (default), `on`, or `off`.
- `--audio-rate` — canonical sample rate: `16000`, `24000` (default), or `48000`.

## Free Demos with Mock Providers

Add `--mock-provider` to replace **every** configured provider — the assistant, the
self-play user role, and any TTS path — with mocks, so a demo makes **zero real API
calls** and costs nothing. Pair it with `--mock-config` to serve canned scenario
responses:

```bash
promptarena serve --mock-provider --mock-config mock-responses.yaml --open
```

This is the safest way to show the UI to others: runs execute end to end (tools
still run for real) but no vendor APIs are billed. See
[Use Mock Providers](/arena/how-to/use-mock-providers/) for the response file
format.

## REST API

The SPA is a thin client over a small REST + SSE surface. You can call these
endpoints directly (for scripts or your own tooling):

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/events` | SSE stream of live run events; add `?audio=1` for audio relay |
| `GET` | `/api/config` | Return the loaded Arena config |
| `GET` | `/api/run-options` | List available providers, scenarios, and regions |
| `GET` | `/api/results` | List completed run IDs |
| `GET` | `/api/results/{id}` | Fetch a single run result |
| `DELETE` | `/api/results` | Clear stored results (memory + disk) |
| `GET` | `/api/media/{path...}` | Serve a media artifact (e.g. recorded audio) |
| `POST` | `/api/run` | Start a run in the background |
| `GET` | `/api/interactive/options` | List agents/providers for interactive chat |
| `POST` | `/api/interactive/session` | Open an interactive chat session |
| `POST` | `/api/interactive/message` | Send a message to a session |
| `GET` | `/` | The embedded React SPA |

## See Also

- [CLI Command Reference](/arena/reference/cli-commands/) — full `serve` flag reference
- [Use the TUI](/arena/how-to/interfaces/use-the-tui/) — the terminal interface
- [Run in CI](/arena/how-to/interfaces/run-in-ci/) — headless, non-interactive runs
- [Use Mock Providers](/arena/how-to/use-mock-providers/) — zero-cost demos and tests
