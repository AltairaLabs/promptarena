---
title: Use Mock Providers
---
Learn how to use mock providers for fast, cost-free testing without calling real LLM APIs.

## Why Use Mock Providers?

- **Fast iteration**: Test configuration changes instantly
- **Zero cost**: No API charges during development
- **Deterministic**: Predictable responses for debugging
- **Offline testing**: Work without internet connection
- **CI/CD testing**: Fast pipeline validation without API dependencies

## Basic Usage

Replace all providers with mock:

```bash
# Use mock provider instead of real APIs
promptarena run --mock-provider
```

This replaces all configured providers with a mock that returns predefined responses.

## Mock Configuration File

A mock config file supplies canned responses keyed by scenario and turn. The
mock provider simulates only the LLM's decision-making — any tools it "calls"
still execute for real through the tool loop. Pass the file with `--mock-config`,
or reference it from a mock provider (see [Mock Provider Config](#mock-provider-config)).

### Mock file format

```yaml
# mock-responses.yaml
scenarios:
  rental-negotiation:        # MUST match the scenario's metadata.name
    turns:
      1: "Thanks for your interest. We're listing at twenty-six hundred a month."
      2: "I can come down to twenty-five hundred on a twelve-month lease."

# Fallback used when no scenario/turn matches
default_response: "This is a mock response from the configured provider."
```

Scenario keys **must match the scenario's `metadata.name`** (not `spec.id`) — the
mock repository looks up responses by metadata name. Turn keys are turn numbers,
and each value is either a plain string (a text response) or a mapping with
`response` and/or `tool_calls`.

### Run with a mock config

```bash
promptarena run --mock-provider --mock-config mock-responses.yaml
```

## Mock Provider Config

Instead of `--mock-provider`, you can define a provider of `type: mock` that
points at its own mock file. This lets you keep a real provider and a mock side
by side in the same arena config:

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

A turn with `tool_calls` triggers real tool execution. The mock decides which
tools to call; the tools run for real and their results feed back into the
conversation:

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

Turn numbers account for each round of the tool loop — a tool-calling turn and
the follow-up text turn are counted separately:

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

A scenario's `default_response` covers turns with no explicit entry; the
top-level `default_response` covers scenarios with no match at all:

```yaml
scenarios:
  quick-question:
    default_response: "The answer to your question is: 42."
    turns:
      1: "Tell me more about what you need."

default_response: "This is a mock response from the configured provider."
```

## Provider Comparison Testing

Compare mock vs. real providers by listing both in your arena config. Every
scenario runs against each provider, so you get a side-by-side matrix:

```yaml
# config.arena.yaml
spec:
  providers:
    - file: providers/openai.yaml
    - file: providers/mock-landlord.provider.yaml

  scenarios:
    - file: scenarios/customer-support.yaml
```

Run side-by-side:

```bash
# Compare real vs mock across the provider matrix
promptarena run --scenario customer-support
```

## Substituting a Single Provider

`--mock-provider` replaces *every* provider with a generic mock. When you need to
swap just one configured provider for another at run time — without editing config —
use `--override-provider from=to` (repeatable). It rewrites the `from` provider with
the spec of the `to` provider, so every reference to `from` (candidate selection,
self-play roles, **and judges**) picks up the new implementation.

```bash
# Tiered CI: mock judge in the cheap gate, real judge in the drift check —
# same config, different invocation.
promptarena run                                         # cheap tier: judge uses its mock provider
promptarena run --override-provider mock-judge=claude   # drift tier: judge runs for real

# Swap a self-play user-simulation model, or isolate one provider behind a mock
promptarena run --override-provider mock-user=claude
promptarena run --override-provider claude=mock         # requires a defined `mock` provider
```

Both `from` and `to` must be providers defined in `spec.providers`. A typo
hard-errors rather than silently leaving the original provider in place:

```
--override-provider mock-judge=nonexistent: unknown target provider "nonexistent" (must be defined in spec.providers)
```

## Development Workflow

### Phase 1: Build with Mocks

```bash
# Fast iteration with mock responses
promptarena run --mock-provider --mock-config dev-mocks.yaml
```

### Phase 2: Validate Structure

```bash
# Verify assertions work with mock data
promptarena run --mock-provider --format junit
```

### Phase 3: Real Provider Testing

```bash
# Test with actual providers
promptarena run --provider openai-gpt4
```

## CI/CD Integration

Use mocks for configuration validation:

```yaml
# .github/workflows/test.yml
name: Arena Tests

on: [push, pull_request]

jobs:
  mock-validation:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Validate Scenarios (Mock)
        run: |
          promptarena run --mock-provider --ci --format junit
      
      - name: Publish Results
        uses: dorny/test-reporter@v1
        with:
          name: Mock Tests
          path: out/junit.xml
          reporter: java-junit
  
  real-provider-tests:
    runs-on: ubuntu-latest
    needs: mock-validation  # Run after mock validation
    steps:
      - uses: actions/checkout@v3
      
      - name: Run Real Provider Tests
        env:
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
        run: |
          promptarena run --provider openai-gpt4 --ci
```

## Complete Mock Example

```yaml
# mock-customer-support.yaml
scenarios:
  # A conversational scenario, keyed by its metadata.name
  customer-support:
    turns:
      1: "Hello! How can I assist you today?"
      2: "We're open Monday-Friday, 9 AM to 5 PM EST."

  # A tool-driven scenario: the mock calls a real tool, then answers
  account-lookup:
    turns:
      1:
        response: "Let me pull up your account."
        tool_calls:
          - name: get_customer_info
            arguments:
              email: customer@example.com
      2: "I found your account. Your current balance is $42.00."

# Fallback for any scenario/turn not covered above
default_response: "I'm here to help. Can you provide more details?"
```

Use this mock:

```bash
promptarena run --mock-provider --mock-config mock-customer-support.yaml
```

## Combining Mocks and Real Providers

Keep a mock provider and real providers in the same arena config. Fast
structure-validation runs can target the mock, while quality runs use the real
LLMs:

```yaml
# config.arena.yaml
spec:
  providers:
    - file: providers/mock-landlord.provider.yaml
    - file: providers/openai.yaml
    - file: providers/claude.yaml
```

Select which provider a run uses with `--provider`, or swap one provider for
another at run time with `--override-provider` (see below).

## Next Steps

- **[Validate Outputs](/arena/how-to/scenarios/validate-outputs/)** - Add assertions to mock responses
- **[Integrate CI/CD](/arena/how-to/interfaces/run-in-ci/)** - Use mocks in pipelines
- **[Tutorial: First Test](/arena/tutorials/01-first-test/)** - Complete walkthrough

## Examples

See complete mock configurations:
- `examples/mock-config-example.yaml` - Full mock setup
- `examples/customer-support/` - Mock vs real provider comparison
