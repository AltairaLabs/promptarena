---
title: Build a PromptPack with an AI coding agent
description: Brief Claude Code, Codex, or another coding agent with PromptArena's conventions, then let it author, validate, and test a kit against mocks before you try real providers.
sidebar:
  order: 20
verified:
  commit: 3df344ffbc8c604e1a8935929c28499d778d19f0
  sources:
    - arena/agentkb/catalog.yaml
    - arena/agentkb/skill.go
    - arena/agentmcp/server.go
    - arena/agentmcp/tools.go
    - arena/cmd/promptarena/agent_brief.go
    - arena/cmd/promptarena/chat.go
    - arena/cmd/promptarena/deploy_adapter_interactive.go
    - arena/cmd/promptarena/init.go
    - arena/cmd/promptarena/mcp_cmd.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/cmd/promptarena/validate.go
    - arena/cmd/promptarena/validate_json.go
    - arena/configschema/configschema.go
    - arena/engine/engine.go
    - arena/tui/tui.go
    - examples/rag-agent/config.arena.yaml
    - schemas/v1alpha1/tool.json
---

Have a coding agent such as Claude Code write a PromptArena kit (prompts, scenarios, assertions, tools and mock responses) and test it against mocks. PromptArena ships the config format and its idioms inside the binary, and writes them into your project as an authoring brief so the agent does not guess field names.

## Prerequisites

- The `promptarena` binary.
- A coding agent, such as Claude Code or Codex.

## 1. Brief the agent

### New project

```bash
promptarena init my-kit --quick
```

`init` scaffolds a sample kit and writes the brief next to it. Pass `--template <name>` to
start from a different built-in template (`promptarena examples list` shows them), or
`--no-agent` to skip the brief.

### Existing project

```bash
promptarena agent-brief            # current directory
promptarena agent-brief ./my-kit   # another directory
```

`agent-brief` writes only the brief, with no sample kit, so it is safe to run in a
project you already have.

### What gets written

| File | Purpose |
| --- | --- |
| `AGENTS.md` | Short pointer to the skill, the workflow, and the common mistakes. |
| `.claude/skills/promptarena-authoring/SKILL.md` | The authoring skill: a step-by-step workflow and minimal valid skeletons for each config kind. |
| `.claude/skills/promptarena-authoring/reference/` | Catalogs generated from the binary: `evals-and-assertions.md`, `config-fields.md`, `mock-responses.md`, and `cli.md`. |

If `AGENTS.md` already exists, the brief is appended once and your own content is kept.
The skill and reference files are overwritten each time. Re-run `promptarena
agent-brief` after you upgrade PromptArena, so the catalogs match the binary
that validates your configs.

## 2. Connect the agent (optional MCP server)

The brief is enough for most work. If you also want the agent to query PromptArena's
knowledge base directly, `promptarena mcp` runs a stdio MCP server with four tools:
`explain`, `get_schema`, `list_examples`, and `show_example`.

For Claude Code, add it to `.mcp.json` in the project:

```json
{
  "mcpServers": {
    "promptarena": { "command": "promptarena", "args": ["mcp"] }
  }
}
```

Any MCP client can use the same command. Use it for agents that do not read `AGENTS.md` or `.claude/skills/`.

## 3. Open the agent and describe the kit

- Claude Code loads `.claude/skills/` and `AGENTS.md` from the project.
- Codex and other agents that read `AGENTS.md` pick it up from there.
- For anything else, tell the agent to read `AGENTS.md` first.

Then describe what the agent under test should do, and how you'll know it works. For
example:

> Build a kit for a refund-support assistant. It can look up orders and issue refunds
> under £50; anything larger goes to a human. It must never promise a refund before
> checking the order. Test the happy path, an over-limit refund, and a customer who
> tries to talk it into skipping the check.

The skill tells the agent to settle measurable success criteria before it writes any
prompts. Expect it to ask:

- What counts as success. Each answer becomes an assertion or eval.
- What the assistant should sound like: identity, tone and guidelines.
- Where the tools' data comes from. When you name something that lives in a backend,
  such as an order or a customer record, it asks for an MCP server, an API or OpenAPI
  spec, or sample payloads so the tool schemas match the real system.

In a non-interactive run (`claude -p`, CI) nobody can answer, so the agent writes
`mode: mock` tools and lists the contracts it had to invent as assumptions for you to
check.

## 4. The build-and-test loop

The agent works against mock providers first, so the loop is fast and costs nothing. A
typical pass:

```bash
promptarena validate config.arena.yaml
promptarena run --ci --formats json,markdown
```

`validate` checks each file against the JSON schema embedded in the binary, checks that every assertion type exists, then builds the pack in memory and runs every check `packc` runs. A config that passes `validate` compiles with `packc`. It reports every problem in one pass. `validate --json` gives the same report as JSON; each problem names the stage that found it, and names the file to edit when that differs from the file being validated. `run --ci` runs every scenario headless, writes results to `--out`, else `defaults.output.dir`, else `out/`, and exits non-zero when a run errors or an assertion fails. The agent reads `out/results.md` for the summary and the per-run JSON files for
detail, fixes what failed, and runs again.

Along the way the agent sets up:

- A mock provider and `mock-responses.yaml`, keyed by each scenario's
  `metadata.name`, so every turn has a known reply. Tools, workflow state and memory
  still execute for real against the mocked model output.
- Self-play personas (cooperative, confused, impatient, adversarial) to exercise the
  guardrails. If the adversarial persona does not trip an assertion, strengthen the assertions.

Give the kit a `type: mock` provider that declares `additional_config.mock_config`. The `--mock-provider` flag replaces each provider with a mock: a provider that declares `mock_config` keeps its canned responses, and `--mock-config` overrides them for all providers. A provider that declares neither gets a generic reply, so assertions that expect specific content fail.

## 5. Define the tools

A `kind: Tool` config is a contract (`name`, `description`, `input_schema`, `output_schema`) plus a binding; the implementation lives outside the pack. See [Tool authoring](/arena/reference/tool-authoring/). For each tool, the agent either:

- mocks it (`mode: mock`) for testing,
- binds an existing service (`mode: live` with an `http` block, `mode: mcp`, or
  `mode: exec`), or
- writes a new service that you deploy separately from the pack.

## 6. Watch it run

The agent can run everything headless. To watch results yourself:

- Terminal UI: run `promptarena run` in your own terminal. Inside an agent session, with no TTY, `run` falls back to plain output.
- Web UI: `promptarena serve` (add `--open` to launch a browser). The agent can
  start it as a background task.
- Conversation: `promptarena chat` lets you talk to the agent under test.

## 7. Real providers and deploy

When the kit passes against mocks, add a real provider and run it again. Real model output varies more than mock output, so expect to tune some assertions.
Then ship it with `promptarena deploy`, or have the agent write a CI workflow that
validates, runs, and deploys the kit.

## Related pages

- [Use mock providers](/arena/how-to/providers/use-mock-providers/)
- [Configure providers](/arena/how-to/providers/configure-providers/)
