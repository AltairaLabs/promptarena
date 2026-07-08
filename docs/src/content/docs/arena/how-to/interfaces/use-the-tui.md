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
The TUI hub menu (View / Run / Chat / Inspect / Deploy)
:::

## Navigate the five pages

The hub menu has five entries:

```text
View     — browse past test results
Run      — run scenarios against providers
Chat     — interactive multi-turn session
Inspect  — explore config and state
Deploy   — walk through preflight, plan, confirm, apply, and status
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
a notice explaining why. **Deploy** additionally requires the loaded config to have
a `deploy:` section — see [Deploy from the TUI](#deploy-from-the-tui) below. Switching
the active config from within the hub rebuilds the engine so the new config takes
effect immediately.

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

## Deploy from the TUI

The **Deploy** entry walks you through the same deploy lifecycle as
`promptarena deploy` on the CLI, but as a guided wizard: preflight, plan, confirm,
apply, and status. It's enabled once the loaded config has a `deploy:` section —
see [Configure Deploy](/arena/how-to/deploy/configure/) to set one up.

:::note[📸 Screenshot needed]
The Deploy entry on the hub menu, alongside View / Run / Chat / Inspect
:::

### Preflight

Opening Deploy starts on a readiness gate, before anything is planned or applied:

- **Target provider** and **environment** — the environment name is shown loudly,
  so a production target is visually unmistakable before you go any further.
- **Adapter presence** — `✓ <provider> vX.Y.Z` if the adapter binary is installed,
  `✗ not installed` if it isn't.
- **Auth** — `✓` or `✗` depending on whether the adapter has a valid token.

If the adapter isn't installed, preflight blocks and prints the command to run:

```text
promptarena deploy adapter install <provider>
```

Installing, listing, and removing adapters is CLI-only in v1 — the TUI detects the
gap and points you at the CLI rather than installing anything itself. See
[Install Adapters](/arena/how-to/deploy/install-adapters/).

### Log in, if needed

When the adapter supports login and you aren't authenticated yet, preflight offers
a **Login** step with `[l]`. It runs the same in-TUI loopback OAuth flow as
`promptarena deploy login`: authenticate in your browser, and the config and token
are written for you. See [Deploy: Log In](/arena/how-to/deploy/login/) for exactly
what gets written and where.

### Plan

Press `[p]` to render the plan — a colored, Terraform-style diff of what will
change:

| Key | Action |
|-----|--------|
| `[space]` | Expand the collapsed "no change" section |
| `[a]` | Accept the plan and move to Confirm |

Drifted resources are badged `⚠ N drifted` so they stand out from ordinary
creates, updates, and deletes. The diff uses the same `+` / `~` / `-` / `!`
vocabulary as the CLI's plan output — see
[Deploy: Plan and Apply](/arena/how-to/deploy/plan-and-apply/#reading-the-plan)
for how to read it.

:::note[📸 Screenshot needed]
The plan page showing a colored diff with a drifted resource
:::

### Confirm

Confirm is the wrong-environment guardrail:

- **Default environment** — a simple `[y/N]` prompt.
- **Any non-default environment** (including production) — you must **type the
  environment name exactly** to proceed. There's no single keypress that can
  apply to a non-default environment by accident.

### Apply

Once confirmed (`[y]`, or the typed environment name), Deploy acquires a deploy
lock and applies, showing live resource progress and logs as it runs — the
in-TUI equivalent of `promptarena deploy`'s streaming output.

### Status

Press `[s]` to check status, after applying or on its own. Each managed resource
shows its health: `✓` healthy, `✗` unhealthy, `?` unknown.

### What's still CLI-only

`destroy`, `import`, `refresh`, and `adapter install` / `list` / `remove` aren't
part of the wizard in v1. Reach for the CLI for these — see
[Deploy: Plan and Apply](/arena/how-to/deploy/plan-and-apply/) for destroy,
import, and refresh, and [Install Adapters](/arena/how-to/deploy/install-adapters/)
for adapter management.

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
- [Configure Deploy](/arena/how-to/deploy/configure/) — set up the `deploy:` section the Deploy page needs
- [Deploy: Plan and Apply](/arena/how-to/deploy/plan-and-apply/) — the CLI equivalent of plan, apply, and status
