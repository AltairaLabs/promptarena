# PromptArena

[![CI](https://github.com/AltairaLabs/promptarena/actions/workflows/ci.yml/badge.svg)](https://github.com/AltairaLabs/promptarena/actions/workflows/ci.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=AltairaLabs_promptarena&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=AltairaLabs_promptarena)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=AltairaLabs_promptarena&metric=coverage)](https://sonarcloud.io/summary/new_code?id=AltairaLabs_promptarena)
[![Go Report Card](https://goreportcard.com/badge/github.com/AltairaLabs/promptarena)](https://goreportcard.com/report/github.com/AltairaLabs/promptarena)
[![Go Reference](https://pkg.go.dev/badge/github.com/AltairaLabs/promptarena.svg)](https://pkg.go.dev/github.com/AltairaLabs/promptarena)
[![npm (promptarena)](https://img.shields.io/npm/v/@altairalabs/promptarena?label=%40altairalabs%2Fpromptarena)](https://www.npmjs.com/package/@altairalabs/promptarena)
[![npm (packc)](https://img.shields.io/npm/v/@altairalabs/packc?label=%40altairalabs%2Fpackc)](https://www.npmjs.com/package/@altairalabs/packc)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

> **Test prompts before they fail in production.** Multi-provider prompt testing, red-teaming, and evaluation — from the command line.

PromptArena is the testing and evaluation framework for [**PromptPack**](https://github.com/AltairaLabs/promptpack-spec): run scenarios and red-team simulations across Claude, OpenAI, Gemini, Azure, and local models, score them with assertions and evals in CI, and compile portable packs for runtime. It is built on the [**PromptKit**](https://github.com/AltairaLabs/PromptKit) runtime.

## How it fits together

```
PromptPack  ── open spec for portable prompts (JSON, vendor-neutral)
    │
    └── PromptKit  ── runtime + SDK (providers, pipeline, tools, workflows, a2a)
         │
         └── PromptArena  ── this repo
              ├── promptarena  ── test, red-team, evaluate (CLI)
              └── packc        ── compile config → portable pack
```

## Install

```bash
npm install -g @altairalabs/promptarena @altairalabs/packc
```

Or with Go:

```bash
go install github.com/AltairaLabs/promptarena/arena/cmd/promptarena@latest
go install github.com/AltairaLabs/promptarena/packc@latest
```

Building from source: see [CONTRIBUTING.md](./CONTRIBUTING.md).

## Quick Start

```bash
# 1. Create a project from a template
promptarena init my-project --template iot-maintenance-demo
cd my-project

# 2. Inspect configuration
promptarena config-inspect

# 3. Run a test scenario
promptarena run --scenario scenarios/hardware-faults.scenario.yaml

# 4. Red-team security testing
promptarena run --scenario scenarios/redteam-selfplay.scenario.yaml

# 5. Review results
promptarena view

# 6. Compile prompts to a portable pack for your app
packc compile -c config.arena.yaml -o app.pack.json
```

## Voice-agent self-play

You can't unit-test a voice agent — so PromptArena has AI personas *call it*. Synthetic, personality-driven callers (hostile, evasive, anxious) are driven through TTS into your realtime agent (Gemini Live, OpenAI Realtime), and structured assertions score whether it holds policy under pressure — never issuing an unauthorized refund, escalating when it should. It even checks the caller *sounds* angry (speech-emotion recognition), not just says angry words.

Try it in one command — keyless, runs green out of the box:

```bash
promptarena init my-refund-demo --template voice-refund-demo
cd my-refund-demo
promptarena run --provider mock-duplex --ci   # no API keys needed
```

Swap in `--provider gemini-2-flash` or `openai-gpt4o-realtime` (plus TTS keys) to run it against a live voice agent — pass rates vary, and that variation is the test.

## Features

| Feature | Description |
|---------|-------------|
| **Multi-Provider** | OpenAI, Anthropic, Google Gemini, Azure OpenAI, Ollama, vLLM |
| **Self-Play Testing** | AI personas for adversarial and user simulation |
| **Voice Self-Play** | Adversarial TTS personas stress-test realtime voice agents (Gemini Live, OpenAI Realtime), scored on behavior + speech-emotion |
| **Red-Team** | Security testing with prompt injection detection |
| **Assertions & Evals** | Pluggable assertion handlers and eval primitives (RAG, safety, quality) |
| **Workflows** | Event-driven state machines with orchestration modes and context carry-forward |
| **MCP Integration** | Native Model Context Protocol for real tool execution |
| **Tool Validation** | Mock or live tool call verification with three-level scoping |
| **Skills & A2A** | AgentSkills.io support and Agent-to-Agent orchestration (via the PromptKit runtime) |
| **Pack Compilation** | `packc` compiles config into portable packs for production (run via the PromptKit SDK) |

## GitHub Actions

PromptArena ships GitHub Actions for CI/CD. Run prompt tests:

```yaml
- name: Run prompt tests
  uses: AltairaLabs/promptarena/.github/actions/promptarena-action@v1
  with:
    config-file: config.arena.yaml
  env:
    OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
```

Compile and publish packs to OCI registries:

```yaml
- name: Build and publish pack
  uses: AltairaLabs/promptarena/.github/actions/packc-action@v1
  with:
    config-file: config.arena.yaml
    registry: ghcr.io
    repository: ${{ github.repository }}/prompts
    username: ${{ github.actor }}
    password: ${{ secrets.GITHUB_TOKEN }}
```

> Note: the action definitions are being migrated from PromptKit; until then reference `AltairaLabs/PromptKit/.github/actions/*@v1`.

## Repository Structure

```
promptarena/
├── arena/     # PromptArena CLI + engine (testing, red-team, self-play, deploy)
├── packc/     # Pack Compiler CLI
├── npm/       # npm distribution wrappers (@altairalabs/promptarena, @altairalabs/packc)
└── schemas/   # JSON schema snapshots for config validation
```

Built on the [PromptKit](https://github.com/AltairaLabs/PromptKit) runtime + SDK (`github.com/AltairaLabs/PromptKit/{runtime,pkg,sdk}`).

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

Apache 2.0 — See [LICENSE](./LICENSE).

---

Built by [AltairaLabs.ai](https://altairalabs.ai)
