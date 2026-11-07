# CLI Reference

Generated from the command tree. Run `promptarena <cmd> --help` for full detail.

## promptarena agent-brief

Write the authoring brief (AGENTS.md + .claude/skills) into a project

## promptarena calibrate

Score a check, such as an LLM judge, against examples a person has labeled

Flags: `--check`, `--config`, `--examples`, `--min-agreement`, `--min-kappa`, `--out`

## promptarena chat

Chat interactively with an agent defined in an Arena config

Flags: `--barge-in`, `--config`, `--echo-guard`, `--mock-config`, `--mock-provider`, `--verbose`, `--voice-output-voice`, `--voice-stt`, `--voice`

## promptarena config-inspect

Inspect and validate prompt arena configuration

Flags: `--config`, `--format`, `--section`, `--short`, `--stats`, `--verbose`

## promptarena debug

Debug configuration and prompt loading

Flags: `--config`

## promptarena deploy

Deploy the arena pack to a target environment

- `promptarena deploy adapter` — Manage deploy adapter binaries
- `promptarena deploy apply` — Apply the deployment
- `promptarena deploy config` — Manage the deploy configuration
- `promptarena deploy destroy` — Tear down a deployment
- `promptarena deploy import` — Import a pre-existing resource into deployment state
- `promptarena deploy login` — Authenticate in the browser and write the deploy config
- `promptarena deploy plan` — Preview the deployment plan
- `promptarena deploy refresh` — Refresh local state from the live environment
- `promptarena deploy status` — Show deployment status

## promptarena examples

Discover example PromptArena kits

- `promptarena examples list` — List example kits from the embedded catalog
- `promptarena examples show` — Print an example kit's files

## promptarena explain

Explain a PromptArena authoring concept

Flags: `--list`

## promptarena export

Export arena config as a compiled PromptPack

Flags: `--config`, `--id`, `--output`

## promptarena generate

Generate Arena scenarios from recorded sessions

Flags: `--config`, `--dedup`, `--expect`, `--filter-eval-type`, `--filter-passed`, `--from-recordings`, `--output`, `--source`, `--task-type`, `--workspace`

## promptarena init

Initialize a new Arena test project

Flags: `--force`, `--no-agent`, `--no-env`, `--output`, `--provider`, `--quick`, `--repo-config`, `--template-cache`, `--template-index`, `--template`, `--verbose`

## promptarena mcp

Run an MCP server exposing PromptArena authoring knowledge over stdio

## promptarena mocks

Manage mock responses for PromptArena

- `promptarena mocks generate` — Generate mock provider responses from Arena JSON results

## promptarena personas

Copy built-in self-play personas into a project

- `promptarena personas add` — Copy a built-in persona into the project
- `promptarena personas list` — List the built-in personas
- `promptarena personas show` — Print a built-in persona's YAML

## promptarena prompt-debug

Debug and test prompt generation

Flags: `--config`, `--context`, `--domain`, `--json`, `--list`, `--persona`, `--region`, `--scenario`, `--show-meta`, `--show-prompt`, `--show-stats`, `--task-type`, `--user`, `--verbose`

## promptarena redteam

Generate red-team scenarios from objective sets

- `promptarena redteam datasets` — List the public objective sets generate can fetch
- `promptarena redteam generate` — Write red-team scenarios for a set of objectives

## promptarena run

Run conversation simulations

Flags: `--audio-monitor`, `--audio-rate`, `--ci`, `--concurrency`, `--config`, `--eval-types`, `--eval`, `--format`, `--formats`, `--junit-file`, `--markdown-file`, `--max-tokens`, `--mock-config`, `--mock-provider`, `--objectives-group-by`, `--out`, `--override-provider`, `--provider`, `--region`, `--scenario`, `--simple`, `--skip-pack-evals`, `--verbose`

## promptarena schema

Print the embedded JSON schema for a config type

Flags: `--list`

## promptarena serve

Start the Arena web UI with live run streaming

Flags: `--audio-monitor`, `--audio-rate`, `--mock-config`, `--mock-provider`, `--open`, `--port`

## promptarena skill

Manage AgentSkills.io skills

- `promptarena skill install` — Install a skill from Git or local path
- `promptarena skill list` — List installed skills
- `promptarena skill remove` — Remove an installed skill

## promptarena templates

Manage PromptArena templates (list, fetch, render, update, repo)

- `promptarena templates fetch` — Fetch a template from an index into cache
- `promptarena templates list` — List built-in templates and the templates in an index
- `promptarena templates render` — Render a cached template to an output directory (dry-run only)
- `promptarena templates repo` — Manage template repositories
- `promptarena templates update` — Update all templates from an index into cache

## promptarena validate

Validate configuration files, and that an arena config builds a valid pack

Flags: `--json`, `--schema-only`, `--type`, `--verbose`

## promptarena view

Browse and view past Arena test results

