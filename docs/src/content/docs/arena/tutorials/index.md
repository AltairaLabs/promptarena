---
title: Arena Tutorials
description: Lessons that build a working PromptArena test suite step by step, from a first scenario to deploying an agent.
sidebar:
  order: 0
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - arena/cmd/promptarena/deploy_adapter_interactive.go
    - arena/engine/builder_integration.go
    - examples/client-tools/providers/mock-provider.yaml
    - examples/duplex-streaming/providers/mock-duplex.provider.yaml
---
Each tutorial builds something that runs, one step at a time. Start with Tutorial 1: it installs the CLI and introduces the file layout the others reuse. The deploy tutorials build on Tutorial 1's project.

## Testing

| Tutorial | What we build | Needs |
|---|---|---|
| [1: Your First Test](/arena/tutorials/01-first-test/) | A prompt, a provider and a two-turn scenario, run against `gpt-5.4-mini` | OpenAI key |
| [2: Multi-Provider Testing](/arena/tutorials/02-multi-provider/) | One scenario run against OpenAI, Claude and Gemini | OpenAI, Anthropic and Gemini keys |
| [3: Multi-Turn Conversations](/arena/tutorials/03-multi-turn/) | Scenarios that check context across turns, and a self-play run | OpenAI key |
| [4: Testing MCP Tools](/arena/tutorials/04-mcp-tools/) | A scenario that calls an MCP memory server's tools | OpenAI key, Node.js |
| [5: CI/CD Integration](/arena/tutorials/05-ci-integration/) | A GitHub Actions workflow that runs the suite and fails on a broken assertion | GitHub repository |
| [6: Duplex Voice Testing](/arena/tutorials/06-duplex-testing/) | A duplex audio scenario on a mock provider | Nothing |
| [7: Using Arena as a Go Library](/arena/tutorials/07-programmatic-usage/) | A Go program that builds a scenario, runs it and reads the result | Go 1.26 |
| [8: Client-Side Tools](/arena/tutorials/08-client-tools/) | Scenarios for a client-side tool with consent granted and denied | Nothing |

## Deploying

| Tutorial | What we build | Needs |
|---|---|---|
| [Your First Deployment](/arena/tutorials/deploy/first-deployment/) | The Tutorial 1 agent deployed to AWS Bedrock AgentCore | AWS account, Go 1.26 |
| [Multi-Environment Deployments](/arena/tutorials/deploy/multi-environment/) | Separate dev and production deployments with config overrides | AWS account |

Every tutorial needs the `promptarena` CLI; [Tutorial 1](/arena/tutorials/01-first-test/) installs it.

## Other sections

- [How-to guides](/arena/how-to/): steps for a task you already have.
- [Reference](/arena/reference/): commands, config fields and assertion types.
- [Explanation](/arena/): how Arena works and why.
