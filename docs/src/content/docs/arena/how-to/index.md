---
title: Arena How-To
description: Task-focused guides for installing PromptArena, writing and running scenarios, configuring providers, testing voice agents and deploying packs.
sidebar:
  order: 0
verified:
  commit: 0dbee6290db4e4de3bff3101f260fd614f81c924
  sources:
    - arena/cmd/promptarena/init.go
    - arena/cmd/promptarena/mocks.go
    - arena/cmd/promptarena/redteam_cmd.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/templates/loader.go
    - examples/guardrails-test/config.arena.yaml
    - examples/model-migration/config.arena.yaml
    - examples/pii-guardrail-red-team/config.arena.yaml
    - examples/text-negotiation/config.arena.yaml
    - examples/voice-bake-off/config.arena.yaml
    - examples/voice-guardrails/config.arena.yaml
    - examples/voice-ivr/config.arena.yaml
    - examples/voice-latency-budget/config.arena.yaml
    - examples/voice-refund-demo/config.arena.yaml
    - examples/workflow-order-processing/config.arena.yaml
    - examples/workflow-support/config.arena.yaml
---
Each guide covers one task. Pick the one that matches what you want to do.

## Set up

### [Install PromptArena](/arena/how-to/setup/installation/)
Install the `promptarena` CLI and check that it runs.

### [Configure Shell Completions](/arena/how-to/setup/shell-completions/)
Enable tab completion for commands, flags, scenarios and providers.

### [Use Project Templates](/arena/how-to/setup/use-project-templates/)
Scaffold a project with `promptarena init`. Six templates are built in: quick start, customer support, code assistant, content generation, multimodal and MCP integration.

### [Develop Templates](/arena/how-to/setup/develop-templates/)
Build, test and publish templates for the community repository.

### [Install Skills](/arena/how-to/setup/install-skills/)
Install, list and remove shared skills from Git repositories or local paths.

### [Use Arena as a Go Library](/arena/how-to/setup/use-as-go-library/)
Run Arena from Go code for programmatic testing and generated scenarios.

### [Build a PromptPack with an AI coding agent](/arena/how-to/agents/build-with-an-ai-agent/)
Brief Claude Code, Codex or another coding agent so it can author, validate and test a kit for you.

## Write and run scenarios

### [Write Test Scenarios](/arena/how-to/scenarios/write-scenarios/)
Create and structure test scenarios in the PromptPack format.

### [Validate Outputs](/arena/how-to/scenarios/validate-outputs/)
Check response quality with assertions and validators.

### [Run a Scenario Over a Dataset](/arena/how-to/scenarios/run-a-dataset/)
Load inputs and expected answers from a JSONL or CSV file, and get one result per row.

### [Test how an agent handles tool failures](/arena/how-to/scenarios/inject-tool-failures/)
Use a turn's `chaos:` block to make calls to a named tool fail, then assert that the agent reports the failure instead of inventing a result.

### [Use guardrails as test signals](/arena/how-to/scenarios/guardrails-as-signals/)
Walk through `examples/guardrails-test/`: one validator enforces a policy at run time and is asserted as a test signal.

### [Red-team a support agent for PII leakage](/arena/how-to/scenarios/pii-guardrail-red-team/)
Walk through `examples/pii-guardrail-red-team/`: two scripted probes check that a `pii_leakage` guardrail catches a leak and stays quiet on a routine question, asserted with `guardrail_triggered`.

### [Generate a red-team suite from objectives](/arena/how-to/scenarios/generate-red-team-scenarios/)
Turn a file of objectives, or a public set such as JailbreakBench, into scenarios with `promptarena redteam generate`.

### [Run workflow scenarios as a regression suite](/arena/how-to/scenarios/workflow-regression/)
Walk through `examples/workflow-support/` and `examples/workflow-order-processing/`: drive an agent through a workflow state machine and assert its transitions and end state.

### [Generate regression scenarios from production sessions](/arena/how-to/scenarios/generate-from-production-sessions/)
Turn a session that went wrong in production into a scenario that asserts it does not happen again.

### [Session Recording](/arena/how-to/scenarios/session-recording/)
Record scenario runs to JSONL, then load, replay, annotate and evaluate them.

### [Manage Context](/arena/how-to/scenarios/manage-context/)
Set a token budget and a truncation strategy so long conversations stay within the model's context window.

## Providers

### [Configure LLM Providers](/arena/how-to/providers/configure-providers/)
Add OpenAI, Anthropic, Gemini and other providers, set credentials, and select providers per run.

### [Use Mock Providers](/arena/how-to/providers/use-mock-providers/)
Run scenarios against canned responses instead of a real LLM API.

### [Generate Mock Responses from Arena Results](/arena/how-to/scenarios/generate-mock-responses-from-arena/)
Turn recorded Arena runs into mock provider YAML with `promptarena mocks generate`.

### [Enable Model Reasoning](/arena/how-to/providers/enable-reasoning/)
Turn on extended thinking for Gemini, Claude and OpenAI models, and capture the trace separately from the answer.

### [Gate model migrations on a regression suite](/arena/how-to/providers/model-migration/)
Walk through `examples/model-migration/`: run the same scenarios against the old and new model side by side.

## Agents and tools

### [Test MCP Tools](/arena/how-to/agents/test-mcp-tools/)
Configure MCP servers in Arena with tool filtering, timeouts and environment variables.

### [Provision an MCP Sandbox per Scenario](/arena/how-to/agents/provision-mcp-sandbox/)
Start an MCP server on demand at run, scenario or session scope.

### [Test A2A Agents](/arena/how-to/agents/test-a2a-agents/)
Test agent-to-agent delegation with mock or remote A2A agents.

### [Test Multi-Agent A2A Delegation](/arena/how-to/agents/a2a-multi-agent/)
Assert which A2A agents a supervisor calls for which requests.

### [Test RAG Agents](/arena/how-to/agents/rag-agent/)
Gate a RAG agent on faithfulness, relevancy, context and hallucination assertions.

### [Test agent negotiation](/arena/how-to/agents/text-negotiation/)
Walk through `examples/text-negotiation/`: a multi-turn rental-price negotiation with conversation-outcome assertions, scripted or self-play.

## Voice

### [Set Up Voice Testing with Self-Play](/arena/how-to/voice/setup-voice-testing/)
Configure voice testing with duplex streaming and self-play over TTS.

### [Use the Voice Console](/arena/how-to/voice/voice-console/)
Hold a live, hands-free voice conversation with an Arena agent from the terminal.

### [Test a Voice Customer Support Agent](/arena/how-to/voice/voice-customer-support/)
Walk through `examples/voice-refund-demo/`: scripted personas drive a refund agent under voice, with assertions on the tools it must and must not call.

### [Test a Voice IVR with a Workflow State Machine](/arena/how-to/voice/voice-ivr/)
Walk through `examples/voice-ivr/`: a workflow-driven bank IVR that routes callers to self-service or a human.

### [Assert per-turn latency budgets](/arena/how-to/voice/voice-latency-budget/)
Walk through `examples/voice-latency-budget/`: gate every turn on a `latency_budget` assertion.

### [Test voice agents that call tools mid-conversation](/arena/how-to/voice/voice-tool-calls/)
Assert that a voice agent calls the right tools, with the right arguments, in the right order.

### [Add voice PII guardrails](/arena/how-to/voice/voice-guardrails/)
Walk through `examples/voice-guardrails/`: a `pii_leakage` guardrail redacts PII before TTS, and `guardrail_triggered` asserts that it fired.

### [Run a voice provider bake-off](/arena/how-to/voice/voice-bake-off/)
Walk through `examples/voice-bake-off/`: one scenario, several providers, one side-by-side report.

### [Test expressive voice personas](/arena/how-to/voice/voice-characterization/)
Drive TTS providers with bracket tags such as `[shouts]` and `[whispers]` from personas that set `expressive: true`.

## Interfaces and CI

### [Use the TUI](/arena/how-to/interfaces/use-the-tui/)
Run tests, browse results and chat with agents from the terminal.

### [Use the Web UI](/arena/how-to/interfaces/use-the-web-ui/)
Start `promptarena serve` to watch runs live, browse results and chat in the browser.

### [Run in CI](/arena/how-to/interfaces/run-in-ci/)
Run `promptarena run --ci` in GitHub Actions or another pipeline, write reports and gate merges.

## Deploy

### [Install Adapters](/arena/how-to/deploy/install-adapters/)
Install, list and remove deploy adapter plugins.

### [Log In](/arena/how-to/deploy/login/)
Authenticate in your browser and let the adapter write the deploy config.

### [Configure](/arena/how-to/deploy/configure/)
Set up the `deploy` section in `arena.yaml` to target a cloud provider.

### [Plan and Apply](/arena/how-to/deploy/plan-and-apply/)
Preview changes, apply them, check status and tear down resources.

### [CI/CD Integration](/arena/how-to/deploy/ci-cd/)
Automate deployments with GitHub Actions.

## Other sections

- [Tutorials](/arena/tutorials/): step-by-step learning paths.
- [Reference](/arena/reference/): commands, config fields and assertion types.
- [Explanation](/arena/): concepts and design decisions.
