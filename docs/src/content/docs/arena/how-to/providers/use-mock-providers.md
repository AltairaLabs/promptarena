---
title: Use Mock Providers
description: "Run scenarios against canned responses with --mock-provider, --mock-config or a type: mock provider, with no API calls or cost."
verified:
  commit: 66ec692303a313b1f7ece7ce55dd5f73859efe29
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/mock_config_check.go
    - arena/cmd/promptarena/provider_override.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/engine/builder_integration.go
    - arena/engine/conversation_executor.go
    - arena/engine/engine.go
    - arena/selfplay/registry.go
    - arena/stages/mock_scenario_context.go
    - arena/turnexecutors/pipeline_stages_integration.go
    - examples/customer-support-integrated/tools/check-subscription-status.tool.yaml
    - examples/customer-support-integrated/tools/get-customer-info.tool.yaml
    - examples/customer-support/config.arena.yaml
    - examples/customer-support/mock-responses.yaml
    - examples/customer-support/scenarios/support-conversations.scenario.yaml
    - examples/guardrails-test/mock-responses.yaml
    - examples/guardrails-test/providers/mock-provider.yaml
    - examples/mock-config-example.yaml
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/provider.json
---
Run scenarios against canned responses instead of a real LLM API.

Mock providers make no network calls, so runs are fast, free, deterministic and work offline, which suits development and CI. See [Testing philosophy](/arena/explanation/testing-philosophy/) for when to use them.

## Basic Usage

Replace every configured provider with a mock:

```bash
promptarena run --mock-provider
```

Each provider keeps its ID and model but runs as a mock. That includes the providers behind self-play roles, so a self-play turn makes no API call either. Responses resolve in this order: the `--mock-config` file if you pass one, then the provider's own `additional_config.mock_config`, then a generic reply ("Mock response from provider"). A provider that already declares `mock_config` keeps its declared responses.

## Mock Configuration File

A mock config file supplies canned responses keyed by scenario and turn. The mock provider simulates only the LLM's decision-making. Any tools it "calls" still execute for real through the tool loop. Pass the file with `--mock-config`, or reference it from a mock provider (see [Mock Provider Config](#mock-provider-config)).

To generate a file from a real run, see [Generate mock responses from an arena run](/arena/how-to/scenarios/generate-mock-responses-from-arena/).

### Mock file format

```yaml
# mock-responses.yaml
scenarios:
  rental-negotiation:        # must match the scenario's metadata.name
    turns:
      1: "Thanks for your interest. We're listing at twenty-six hundred a month."
      2: "I can come down to twenty-five hundred on a twelve-month lease."

# Fallback used when no scenario/turn matches
defaultResponse: "This is a mock response from the configured provider."
```

Scenario keys must match the scenario's `metadata.name`, not `spec.id`. Turn keys are turn numbers. Each value is either a plain string (a text response) or a mapping with `response` and/or `tool_calls`.

The key is `defaultResponse` (camelCase). Arena checks a mock file's keys when it loads it, so a misspelled or snake_case key such as `default_response` is an error rather than a silent fall back to the generic "Mock response for provider ..." reply. The check also covers the keys inside a turn (`response`, `tool_calls`, `parts` and the audio keys), a tool call (`name`, `arguments`) and a content part. `promptarena validate` checks the `mock_config` file of each `type: mock` provider; `promptarena run` also checks the file `--mock-config` names.

### Run with a mock config

```bash
promptarena run --mock-provider --mock-config mock-responses.yaml
```

## Mock Provider Config

Instead of `--mock-provider`, define a provider of `type: mock` that points at its own mock file. This keeps a real provider and a mock side by side in the same arena config:

```yaml
# providers/mock-landlord.provider.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: mock-landlord
spec:
  id: mock-landlord
  type: mock
  model: mock-model
  additional_config:
    mock_config: mock-responses.yaml   # relative to the arena config directory
```

## Advanced Mock Patterns

### Tool call simulation

A turn with `tool_calls` triggers real tool execution. The mock decides which tools to call. The tools run for real and their results feed back into the conversation:

```yaml
scenarios:
  tool-test:
    turns:
      1:
        response: "Let me check your account."
        tool_calls:
          - name: check_subscription_status
            arguments:
              email: jane.smith@acme.com
      2: "You have an active Pro subscription that renews on the 15th."
```

### Multi-turn tool loops

Turn numbers count each round of the tool loop. A tool-calling turn and the follow-up text turn are counted separately:

```yaml
scenarios:
  billing-question:
    turns:
      1:
        tool_calls:
          - name: get_customer_info
            arguments:
              email: customer@email.com
      2: "I can see your account. Let me check your recent billing activity."
      3:
        tool_calls:
          - name: create_support_ticket
            arguments:
              email: customer@email.com
              issue_type: billing
              priority: high
      4: "I've created ticket TICKET-98765 for your billing issue."
```

### Per-scenario defaults

A scenario's `defaultResponse` covers turns with no explicit entry. The top-level `defaultResponse` covers scenarios with no match at all:

```yaml
scenarios:
  quick-question:
    defaultResponse: "The answer to your question is: 42."
    turns:
      1: "Tell me more about what you need."

defaultResponse: "This is a mock response from the configured provider."
```

## Provider Comparison Testing

List a mock and a real provider in your arena config. Every scenario runs against each provider, which gives a side-by-side matrix:

```yaml
# config.arena.yaml
spec:
  providers:
    - file: providers/openai.yaml
    - file: providers/mock-landlord.provider.yaml

  scenarios:
    - file: scenarios/customer-support.yaml
```

Run a scenario across the matrix:

```bash
promptarena run --scenario customer-support
```

Select which provider a run uses with `--provider`.

## Substituting a Single Provider

`--mock-provider` turns every provider into a mock. To swap one configured provider for another at run time without editing config, use `--override-provider from=to` (repeatable). It rewrites the `from` provider with the spec of the `to` provider, so every reference to `from` (candidate selection, self-play roles and judges) picks up the new implementation.

```bash
# Tiered CI: mock judge in the cheap gate, real judge in the drift check,
# same config, different invocation.
promptarena run                                         # cheap tier: judge uses its mock provider
promptarena run --override-provider mock-judge=claude   # drift tier: judge runs for real

# Swap a self-play user-simulation model, or isolate one provider behind a mock
promptarena run --override-provider mock-user=claude
promptarena run --override-provider claude=mock         # requires a defined `mock` provider
```

Both `from` and `to` must be providers defined in `spec.providers`. A typo is a hard error:

```text
--override-provider mock-judge=nonexistent: unknown target provider "nonexistent" (must be defined in spec.providers)
```

## Development Workflow

### Step 1: Build with Mocks

```bash
promptarena run --mock-provider --mock-config dev-mocks.yaml
```

### Step 2: Validate Structure

```bash
# Verify assertions work with mock data
promptarena run --mock-provider --format junit
```

### Step 3: Real Provider Testing

```bash
promptarena run --provider openai-gpt4
```

## CI/CD Integration

Use mocks for configuration validation, then run real providers after the mock job passes:

```yaml
# .github/workflows/test.yml
name: Arena Tests

on: [push, pull_request]

jobs:
  mock-validation:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - name: Validate Scenarios (Mock)
        run: |
          promptarena run --mock-provider --ci --format junit

      - name: Publish Results
        uses: dorny/test-reporter@v3
        with:
          name: Mock Tests
          path: out/junit.xml
          reporter: java-junit

  real-provider-tests:
    runs-on: ubuntu-latest
    needs: mock-validation  # Run after mock validation
    steps:
      - uses: actions/checkout@v7

      - name: Run Real Provider Tests
        env:
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
        run: |
          promptarena run --provider openai-gpt4 --ci
```

## Next Steps

- [Generate mock responses from an arena run](/arena/how-to/scenarios/generate-mock-responses-from-arena/): Build a mock file from a real run
- [Validate Outputs](/arena/how-to/scenarios/validate-outputs/): Add assertions to mock responses
- [Integrate CI/CD](/arena/how-to/interfaces/run-in-ci/): Use mocks in pipelines
- [Tutorial: First Test](/arena/tutorials/01-first-test/): Complete walkthrough

## Examples

- `examples/mock-config-example.yaml`: full mock config
- `examples/guardrails-test/`: a `type: mock` provider beside an OpenAI provider, with `mock-responses.yaml`
- `examples/customer-support/`: `mock-responses.yaml` for use with `--mock-provider --mock-config`
