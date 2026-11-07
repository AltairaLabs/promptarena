---
title: Use the TUI
description: Launch Arena's interactive terminal hub to run tests, browse results, chat with agents and deploy.
sidebar:
  order: 10
verified:
  commit: 0dbee6290db4e4de3bff3101f260fd614f81c924
  sources:
    - arena/cmd/promptarena/chat.go
    - arena/cmd/promptarena/chat_interactive.go
    - arena/cmd/promptarena/config_inspect_interactive.go
    - arena/cmd/promptarena/deploy_interactive.go
    - arena/cmd/promptarena/deploy_login_interactive.go
    - arena/cmd/promptarena/main_integration.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/cmd/promptarena/view.go
    - arena/cmd/promptarena/view_interactive.go
    - arena/deploy/flow/config.go
    - arena/deploy/flow/connect.go
    - arena/deploy/flow/login.go
    - arena/deploy/flow/preflight.go
    - arena/deploy/flow/render.go
    - arena/tui/app/app.go
    - arena/tui/app/chatpage.go
    - arena/tui/app/chatpage_voice.go
    - arena/tui/app/config.go
    - arena/tui/app/configswitch.go
    - arena/tui/app/deploypage.go
    - arena/tui/app/home.go
    - arena/tui/app/inspectpage.go
    - arena/tui/app/launch_interactive.go
    - arena/tui/app/menu.go
    - arena/tui/app/runpage.go
    - arena/tui/app/viewpage.go
    - arena/tui/pages/file_browser.go
    - arena/tui/views/deploytable.go
    - arena/tui/views/plandiff.go
    - arena/voice/portaudio/portaudio_discovery.go
    - arena/voice/portaudio/portaudio_io_interactive.go
    - arena/voice/session_audioio_interactive.go
---

Arena ships a full-screen terminal UI (TUI): a hub for running scenarios, browsing
past results, chatting with an agent and deploying. This guide shows how to launch
the hub, move between its pages, and deep-link straight into the page you want.

## Launch the hub

Run `promptarena` with no subcommand from any directory:

```bash
promptarena
```

The hub opens in the alternate screen behind a brief splash (any key dismisses it).
It loads `config.arena.yaml` from the working directory if one exists. No config is
required to launch, but Run, Chat, Inspect and Deploy stay greyed out until one is
loaded. Results default to the `out/` directory in the working directory.

![The TUI hub menu with five entries (View, Run, Chat, Inspect and Deploy), each with a one-line description, and the loaded config shown above.](/screenshots/tui-hub.png)

## Navigate the five pages

The hub menu has five entries:

```text
View     - browse past test results
Run      - run scenarios against providers
Chat     - interactive multi-turn session
Inspect  - explore config and state
Deploy   - deploy this pack to a target environment
```

| Key | Action |
|-----|--------|
| `↑` / `↓` (or `j` / `k`) | Move the menu selection |
| `Enter` | Open the highlighted page |
| `Esc` | Go back one page; quits at the root menu |
| `q` | Quit from the root menu |
| `c` | Open the config switcher (Home page) |
| `Ctrl-C` | Quit from anywhere |

Run, Chat, Inspect and Deploy need a loaded config. Without one, they are greyed out
and `Enter` does nothing; the config line prompts you to press `c` to pick a config or run
`promptarena init`. Switching config rebuilds the engine
so the new config takes effect immediately.

### Inspect config and state

The **Inspect** page shows the resolved config and live state (providers, agents,
scenarios, and workflow/memory state) so you can confirm what Arena will actually
run before you launch it. Scroll with `↑` / `↓` or `j` / `k`, `PgUp` / `PgDn` for half
a page, and `g` / `G` for top and bottom.

![The Inspect page listing the resolved config: prompt configs, scenarios with task and turn counts, the three registered providers with their models and files, and the declared tools.](/screenshots/tui-inspect.png)

## Browse results with `view`

To jump straight to the results browser, use the `view` subcommand:

```bash
promptarena view            # browse ./ (current directory)
promptarena view out/       # browse a specific results directory
```

`view` opens a file browser over `.json` result files. The directory argument
defaults to the current directory and must exist. Navigate with `↑` / `↓`, open with
`Enter`, and go to the parent directory with `←` or `h`.

- `Enter` on `index.json` opens the Results page: a runs table, a log panel and a
  result panel. `Tab` cycles focus between them, and `Enter` on a run opens its
  conversation.
- `Enter` on any other `.json` file opens that run's conversation directly.

The file browser is the root of the navigation stack, so `Esc` or `q` there exits
the program instead of returning to the hub. Deeper in, `Esc` goes back one page.

![The results browser with the conversation transcript open: a numbered turn list on the left showing alternating user and assistant roles, and the selected turn's full message rendered on the right.](/screenshots/tui-results.png)

## Chat with an agent

The `chat` subcommand opens an interactive, multi-turn session with an agent
declared in a config:

```bash
promptarena chat                              # uses config.arena.yaml
promptarena chat --config config.arena.yaml   # explicit config path
```

Chat opens on an agent picker if the config has more than one agent, then a
provider picker if it has more than one provider. Choose with the number key, since
the pickers read digits as selections. With one of each, chat starts straight away.
It then asks for any required variables, and whether to run evals if the config
defines them (`y` / `n`).

Type a message and press `Enter` to send; the agent replies turn by turn, keeping
conversation state. `Tab` moves focus between the conversation and the input, and
`Ctrl-L` toggles a runtime-log overlay. Add `-v` / `--verbose` to tee raw provider
events and transcription to `promptarena.log` for debugging. The chat page is the
stack root, so `Esc` exits back to the shell. While the input box has focus, `q`
is typed into the message; `Tab` to the conversation first to quit with `q`.

![A chat session seven turns deep: the numbered turn list shows the system prompt, two user messages, the assistant's replies, a get_order_status tool call and its result, and the final answer rendered on the right with token and cost totals.](/screenshots/tui-chat.png)

### Voice mode

Add `--voice` for a hands-free voice conversation. This requires PortAudio on the
host, loaded only when a voice session starts:

```bash
promptarena chat --voice --config config.arena.yaml
```

If PortAudio is missing, the command prints an install hint and every other feature
keeps working. For turn-detection modes, echo guarding and STT/TTS flags, see
[Use the Voice Console](/arena/how-to/voice/voice-console/).

## Watch a run live

The `run` command shows a live progress TUI by default, so you do not need bare
`promptarena` to get it:

```bash
promptarena run --config config.arena.yaml
```

The display shows runs as they progress, a log panel, and results as they complete.
The log panel shows pipeline stage events: each stage logs `Stage started` and
`Stage completed` lines at DEBUG, and a failing stage logs `Stage failed` at ERROR.

![A run in progress: an Active Runs table listing each provider with status, scenario, region and duration, a live log panel streaming turn and provider-call events, and a results pane awaiting selection.](/screenshots/tui-run.png)

### Disable the live TUI

For CI, log capture or piped output, turn the TUI off with `--ci`:

```bash
promptarena run --ci --config config.arena.yaml
```

`--ci` prints deterministic plain output and exits non-zero when any run errors or
an assertion fails. `--simple` also disables the TUI, with human-friendly linear
output, but does not set the failure exit codes. See
[Run Arena in CI](/arena/how-to/interfaces/run-in-ci/) for the full CI workflow.

## Deploy from the TUI

The **Deploy** entry walks the deploy lifecycle as a wizard: preflight, plan,
confirm, apply and status. It is selectable with any loaded config. Without a
`deploy:` section it opens a "Deploy failed" screen; see
[Configure Deploy](/arena/how-to/deploy/configure/) to set one up. The wizard always
targets the `default` environment. For another environment, use
`promptarena deploy --env <name>`.

### Preflight

Opening Deploy starts on a readiness gate that shows:

- The target provider and environment.
- Adapter presence: `✓ <provider> vX.Y.Z` if installed, `✗ not installed` if not.
- Auth: `✓ authenticated` or `✗ not authenticated`.

If the adapter is missing, preflight blocks and prints the command to run:

```text
promptarena deploy adapter install <provider>
```

Adapter management is CLI-only. See
[Install Adapters](/arena/how-to/deploy/install-adapters/).

### Log in, if needed

When the adapter supports login and you are not authenticated, press `l` to log in.
It runs the same loopback OAuth flow as `promptarena deploy login`. See
[Deploy: Log In](/arena/how-to/deploy/login/) for what gets written and where.

### Plan

Press `p` to render the plan, a Terraform-style diff of what will change. Resources
already matching the config are hidden by default.

| Key | Action |
|-----|--------|
| `space` | Show or hide the "no change" section |
| `a` | Accept the plan and move to Confirm |

Drifted resources are badged `⚠ N drifted`. The diff uses the same `+` / `~` / `-` /
`!` symbols as the CLI; see
[Deploy: Plan and Apply](/arena/how-to/deploy/plan-and-apply/#reading-the-plan).

![The deploy plan page: a "0 to create, 2 to update, 0 to delete" summary, two adapter warnings, and the changed resources listed with ~ markers — a PromptPack and an AgentRuntime that have drifted from the declared config.](/screenshots/tui-deploy-plan.png)

### Confirm

Confirm shows a `[y/N]` prompt. Press `y` to apply; any other key returns to the plan.

### Apply

Deploy acquires a deploy lock and applies. The screen shows a spinner and
`Applying...` until resource events arrive, then the resource table and log panel.
The result screen reads `Apply completed successfully`, `with N failure(s)` or
`with errors`.

### Status

From the apply-result screen, press `s` to check status. Each managed resource shows
its health: `✓` healthy, `✗` unhealthy, `?` missing. `Esc` or `q` goes back.

### CLI-only operations

`destroy`, `import`, `refresh` and `adapter install` / `list` / `remove` are not
part of the wizard. See [Deploy: Plan and Apply](/arena/how-to/deploy/plan-and-apply/)
for destroy, import and refresh, and [Install Adapters](/arena/how-to/deploy/install-adapters/)
for adapters.

## Recover a wedged terminal

If a session hangs, `Ctrl-C` starts a 5-second watchdog and prints a hint. Press
`Ctrl-C` again (or wait) and Arena force-exits, restoring the cursor, screen buffer
and terminal attributes. If the terminal is still off, run `reset` or `stty sane`.

## See also

- [CLI Command Reference](/arena/reference/cli-commands/): every command and flag
- [Use the Web UI](/arena/how-to/interfaces/use-the-web-ui/): the browser-based dashboard
- [Run Arena in CI](/arena/how-to/interfaces/run-in-ci/): headless runs and exit codes
- [Use the Voice Console](/arena/how-to/voice/voice-console/): live voice conversations
- [Configure Deploy](/arena/how-to/deploy/configure/): set up the `deploy:` section
- [Deploy: Plan and Apply](/arena/how-to/deploy/plan-and-apply/): the CLI equivalent of plan, apply and status
