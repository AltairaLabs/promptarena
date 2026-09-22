---
title: Build a PromptPack with an AI coding agent
description: Brief Claude Code, Codex, or another coding agent with PromptArena's conventions, then let it author, validate, and test a kit against mocks before you try real providers.
sidebar:
  order: 20
---

A coding agent such as Claude Code can write a PromptArena kit for you: the prompts,
scenarios, assertions, tools, and mock responses. It works best when it knows the
config format and the idioms up front, so it doesn't have to guess field names. PromptArena
ships that knowledge inside the binary and writes it into your project as an **authoring
brief**.

This guide covers briefing the agent, what to ask it for, the build-and-test loop it
runs, and the steps that still need you.

## 1. Brief the agent

### New project

```bash
promptarena init my-kit --quick
```

`init` scaffolds a sample kit and writes the brief next to it. Pass `--template <name>` to
start from a different template (`promptarena examples list` shows the catalog), or
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
The skill and reference files are overwritten each time, so **re-run `promptarena
agent-brief` after upgrading PromptArena** to keep the catalogs in step with the binary
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

Any MCP client can use the same command. This is also the easiest route for agents that
don't read `AGENTS.md` or `.claude/skills/`.

## 3. Open the agent and describe the kit

- **Claude Code** loads `.claude/skills/` and `AGENTS.md` from the project.
- **Codex** and other agents that read `AGENTS.md` pick it up from there.
- For anything else, tell the agent to read `AGENTS.md` first.

Then describe what the agent under test should do, and how you'll know it works. For
example:

> Build a kit for a refund-support assistant. It can look up orders and issue refunds
> under £50; anything larger goes to a human. It must never promise a refund before
> checking the order. Test the happy path, an over-limit refund, and a customer who
> tries to talk it into skipping the check.

The skill tells the agent to settle **measurable success criteria** before writing any
prompts, so expect questions like these:

- **What counts as success?** Each answer becomes an assertion or eval, and those are
  the spec.
- **What should the assistant sound like?** It asks you for identity, tone, and
  guidelines rather than making them up.
- **Where do the tools' data come from?** When you name something that lives in a backend,
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

`validate` checks each file against the JSON schema embedded in the binary and checks that
every assertion type exists. `run --ci` runs every scenario headless and writes results
to `out/`. The agent reads `out/results.md` for the summary and the per-run JSON files for
detail, fixes what failed, and runs again.

Things the agent will set up along the way:

- **A mock provider and `mock-responses.yaml`**, keyed by each scenario's
  `metadata.name`, so every turn has a known reply. Tools, workflow state, and memory
  still execute for real against the mocked model output.
- **Self-play personas** (cooperative, confused, impatient, adversarial) to exercise the
  guardrails. If the adversarial persona doesn't trip an assertion, the assertions are
  too weak.

Prefer a `type: mock` provider in the kit over the `--mock-provider` flag. The flag
replaces every provider with a generic mock that ignores your canned responses, so
assertions that expect specific content will fail.

## 5. Tools: definitions, not implementations

A PromptPack defines tools; it doesn't implement them. A `kind: Tool` config is a
contract (`name`, `description`, `input_schema`, `output_schema`) plus a binding, never
backend code. For each tool, the agent will:

- **mock it** (`mode: mock`) for testing,
- **bind an existing service** (`mode: live` with an `http` block, `mode: mcp`, or
  `mode: exec`), or
- **write a new service** that you deploy separately from the pack.

Whichever you choose, the implementation lives outside the pack.

## 6. Watch it run

The agent can run everything headless. To watch results yourself:

- **Terminal UI**: run `promptarena run` in **your own terminal**. The interactive TUI
  can't run inside an agent session.
- **Web UI**: `promptarena serve` (add `--open` to launch a browser). The agent can
  start it as a background task.
- **Conversation**: `promptarena chat` lets you talk to the agent under test.

## 7. Real providers and deploy

When the kit passes against mocks, add a real provider and run it again. Expect some
assertions to need tuning, because real model output varies more than mocks do.
Then ship it with `promptarena deploy`, or have the agent write a CI workflow that
validates, runs, and deploys the kit.

## See also

- [Use mock providers](/arena/how-to/providers/use-mock-providers/)
- [Configure providers](/arena/how-to/providers/configure-providers/)
