---
title: Arena Reference
description: Index of the PromptArena reference pages, with summary tables of commands, common assertions and output formats.
sidebar:
  order: 0
verified:
  commit: 0dbee6290db4e4de3bff3101f260fd614f81c924
  sources:
    - arena/assertions/types.go
    - arena/cmd/promptarena/config_inspect_interactive.go
    - arena/cmd/promptarena/debug.go
    - arena/cmd/promptarena/deploy_interactive.go
    - arena/cmd/promptarena/init.go
    - arena/cmd/promptarena/prompt-debug_interactive.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/cmd/promptarena/validate.go
    - arena/cmd/promptarena/view.go
    - arena/engine/builder_integration.go
    - schemas/v1alpha1/tool.json
---

The PromptArena reference pages describe the `promptarena` commands, configuration files, check types and outputs. For task-oriented guides, see the [How-To Guides](/arena/how-to/); to learn by building, see the [Tutorials](/arena/tutorials/).

## Pages

| Page | Covers |
|------|--------|
| [CLI Commands](/arena/reference/cli-commands/) | Every `promptarena` command with its flags and defaults |
| [Configuration Schema](/arena/reference/config-schema/) | Every field of the arena, scenario, provider, prompt, tool and persona files |
| [Scenario Format](/arena/reference/scenario-format/) | The structure of a scenario file |
| [Assertions](/arena/reference/assertions/) | Assertion types for checking responses and conversations |
| [Guardrails](/arena/reference/validators/) | Guardrails that check or block responses at runtime |
| [Output Formats](/arena/reference/output-formats/) | Report formats and the web UI |
| [Duplex Configuration](/arena/reference/duplex-config/) | Duplex (bidirectional streaming) scenarios for voice testing |
| [Tool Authoring](/arena/reference/tool-authoring/) | The tool `mode` values (`mock`, `live`, `mcp`, `client`) and how to write each |
| [Go API Reference](/arena/reference/api-reference/) | Using Arena as a Go library |
| [Deploy: CLI Commands](/arena/reference/deploy/cli-commands/) | The `promptarena deploy` commands |
| [Deploy: Target Capabilities](/arena/reference/deploy/target-capabilities/) | What each deploy target supports |
| [Deploy: Protocol](/arena/reference/deploy/protocol/) | The protocol between `promptarena deploy` and an adapter |
| [Deploy: Adapter SDK](/arena/reference/deploy/adapter-sdk/) | Writing a deploy adapter |

## Commands

| Command | Purpose |
|---------|---------|
| `promptarena run` | Run scenarios against providers |
| `promptarena init` | Create a new Arena project |
| `promptarena validate` | Validate config files, and that an arena config builds a valid pack |
| `promptarena config-inspect` | Inspect and validate an arena configuration |
| `promptarena debug` | Debug configuration and prompt loading |
| `promptarena prompt-debug` | Test prompt rendering |
| `promptarena serve` | Start the web UI with live run streaming |
| `promptarena view` | Browse past results in the terminal |
| `promptarena deploy` | Deploy the arena pack to a target environment |

See [CLI Commands](/arena/reference/cli-commands/) for every command.

## Common assertions

| Assertion | Passes when |
|-----------|-------------|
| `content_includes` | The response contains every pattern |
| `content_matches` | The response matches a regular expression |
| `tools_called` | The named tools were called |
| `is_valid_json` | The response is valid JSON |
| `json_schema` | The response matches a JSON schema |
| `llm_judge` | A judge model scores the response against criteria |

## Output formats

| Format | `--format` value | Use |
|--------|------------------|-----|
| JSON | `json` | Machine processing |
| JUnit XML | `junit` | CI test reporting |
| Markdown | `markdown` | Human-readable reports |
| CSV | `csv` | Spreadsheets |
| Objectives | `objectives` | Objective pass rates, grouped with `--objectives-group-by` |

## Getting help

- [Explanation](/arena/): how Arena works
- [GitHub Issues](https://github.com/AltairaLabs/promptarena/issues)
