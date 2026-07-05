---
title: Use the TUI
description: Launch Arena's interactive terminal hub to run tests, browse results, and chat with agents.
sidebar:
  order: 10
---

Arena ships a full-screen terminal UI (TUI) — an interactive hub for running
scenarios, browsing past results, and chatting with an agent, all without leaving
the terminal. This guide shows how to launch the hub, move between its pages, and
deep-link straight into the page you want.

## Launch the hub

Run `promptarena` with **no subcommand** from any directory:

```bash
promptarena
```

The hub opens in the alternate screen. On startup it looks for an Arena config
in the working directory (for example `config.arena.yaml`) and loads it if found.
No config is required to launch — the hub still opens, but pages that need one
(Run, Chat, Inspect) stay disabled until a config is present. Results default to
the `out/` directory beside the config.

:::note[📸 Screenshot needed]
The TUI hub menu (View / Run / Chat / Inspect)
:::

## Navigate the four pages

The hub menu has four entries:

```text
View     — browse past test results
Run      — run scenarios against providers
Chat     — interactive multi-turn session
Inspect  — explore config and state
```

Navigation keys are consistent across the whole hub:

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move the menu selection |
| `Enter` | Open the highlighted page |
| `Esc` | Go back one page (pops the navigation stack) |
| `q` | Quit — only at the root menu; inside a page, use `Esc` to go back |
| `Ctrl-C` | Quit from anywhere |

**View** is always available — it only needs a results directory. **Run**, **Chat**,
and **Inspect** require a loaded config; if none was discovered, opening one shows
a notice explaining why. Switching the active config from within the hub rebuilds
the engine so the new config takes effect immediately.

### Inspect config and state

The **Inspect** page shows the resolved config and live state (providers, agents,
scenarios, and workflow/memory state) so you can confirm what Arena will actually
run before you launch it.

:::note[📸 Screenshot needed]
The Inspect page showing resolved config and state
:::

## Browse results with `view`

To jump straight to the results browser without going through the menu, use the
`view` subcommand:

```bash
promptarena view            # browse ./ (current directory)
promptarena view out/       # browse a specific results directory
```

`view` opens a file browser over past JSON result files. Select a run to preview
its metadata, then drill into the full conversation transcript for any completed
test. The directory argument is optional and defaults to the current directory;
the path must exist and be a directory or the command errors out.

Because `view` deep-links with the results page as the root of the navigation
stack, `Esc` or `q` **exits the program** rather than returning to the hub menu.

:::note[📸 Screenshot needed]
The results browser previewing a JSON result and its conversation transcript
:::

## Chat with an agent

The `chat` subcommand opens an interactive, multi-turn session with an agent
declared in a config:

```bash
promptarena chat                              # uses config.arena.yaml
promptarena chat --config config.arena.yaml   # explicit config path
```

Type a message and press `Enter` to send; the agent replies turn by turn, keeping
conversation state between turns. Add `-v` / `--verbose` to tee raw provider events
and transcription to `promptarena.log` for debugging. As with `view`, the chat page
is the stack root, so `Esc` / `q` exits back to the shell.

:::note[📸 Screenshot needed]
An interactive chat session with an agent, several turns deep
:::

### Voice mode

Add `--voice` for a hands-free voice conversation. This requires **PortAudio**
installed on the host (loaded on demand only when a voice session starts):

```bash
promptarena chat --voice --config config.arena.yaml
```

Speak naturally — the agent responds with synthesized audio. If PortAudio is
missing, only voice is unavailable; the command prints an install hint and every
other feature keeps working. For turn-detection modes, echo guarding, and STT/TTS
flags, see [Use the Voice Console](/arena/how-to/voice-console/).

## Watch a run live

The `run` command shows a **live progress TUI by default** — you do not need bare
`promptarena` to get it:

```bash
promptarena run --config config.arena.yaml
```

While scenarios execute you get a live feed of runs as they progress, a log panel,
and results as they complete. It updates in place until every combination finishes.

:::note[📸 Screenshot needed]
A live run in progress — the run feed, log panel, and completed results
:::

### Disable the live TUI

For non-interactive environments (CI, log capture, piping output), turn the TUI
off with `--ci` (or its alias `--simple`) to fall back to plain streaming logs:

```bash
promptarena run --ci --config config.arena.yaml
```

In `--ci` mode Arena also fails the process with a non-zero exit code when any run
errors or an assertion fails, which is what you want in a pipeline. See
[Run Arena in CI](/arena/how-to/interfaces/run-in-ci/) for the full CI workflow.

## Recover a wedged terminal

If a session ever hangs, `Ctrl-C` starts a 5-second watchdog and prints a hint.
Press `Ctrl-C` again — or wait out the 5 seconds — and Arena force-exits, restoring
the cursor, primary screen buffer, and terminal attributes on the way out. If the
terminal is still off afterward, `reset` or `stty sane` will finish the cleanup.

## See also

- [CLI Command Reference](/arena/reference/cli-commands/) — every command and flag
- [Use the Web UI](/arena/how-to/interfaces/use-the-web-ui/) — the browser-based dashboard
- [Run Arena in CI](/arena/how-to/interfaces/run-in-ci/) — headless runs and exit codes
- [Use the Voice Console](/arena/how-to/voice-console/) — live voice conversations
