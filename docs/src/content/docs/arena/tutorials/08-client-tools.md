---
title: 'Tutorial 8: Client-Side Tools'
description: 'Test a client-mode tool in PromptArena with a mock provider, and simulate consent grant and consent deny.'
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - arena/assertions/types.go
    - arena/assertions/validate_types_test.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/consent/hook.go
    - arena/engine/conversation_executor.go
    - arena/turnexecutors/pipeline_stages_integration.go
    - examples/client-tools/mock-responses.yaml
    - examples/client-tools/prompts/chat.yaml
    - examples/client-tools/providers/mock-provider.yaml
    - examples/client-tools/scenarios/consent-grant.scenario.yaml
    - examples/client-tools/tools/get_location.tool.yaml
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/promptconfig.json
    - schemas/v1alpha1/scenario.json
    - schemas/v1alpha1/tool.json
---
We will test a tool that runs on the caller's device, and simulate the user granting and denying consent.

## What we will build

A project with one client-mode tool, `get_location`, two scenarios that exercise it, and a mock provider that replies without an API key. At the end, `promptarena run --simple` passes both scenarios and writes `out/results.md`.

## Prerequisites

- `promptarena` installed ([Tutorial 1](/arena/tutorials/01-first-test/), Step 1)

No provider key is needed.

A client-mode tool is handled outside the runtime and executes on the caller's device; see [Tool authoring](/arena/reference/tool-authoring/) for the details. PromptArena simulates the consent prompt, so we can test both granted and denied permission.

## Step 1: Define a client-mode tool

Create `tools/get_location.tool.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: get_location
spec:
  description: "Get the user's current GPS coordinates"
  mode: client
  timeout_ms: 5000
  input_schema:
    type: object
    properties:
      accuracy:
        type: string
        enum: ["high", "low"]
        description: "Desired accuracy level"
  output_schema:
    type: object
    properties:
      lat:
        type: number
      lng:
        type: number
  mock_result:
    lat: 37.7749
    lng: -122.4194
  client:
    consent:
      required: true
      message: "Allow location access?"
      decline_strategy: error
    categories:
      - location
```

`mode: client` makes this a client-side tool, `mock_result` is what the model receives when the call goes through, and `client.consent` makes the call wait for the user's permission.

## Step 2: Create a prompt

Create `prompts/chat.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: chat
spec:
  task_type: "chat"
  version: "v1.0.0"
  description: "Chat assistant with client-side tool access"

  system_template: |
    You are a helpful assistant with access to the user's device capabilities.

    Available tools:
    - get_location: Get the user's GPS coordinates (requires user consent)

    When the user asks about their location, use the get_location tool.
    If the tool is unavailable or denied, politely inform the user that
    you are unable to access their location and suggest they provide it manually.

  allowed_tools:
    - get_location
```

## Step 3: Write the scenarios

We write two scenarios: one where the user grants consent and one where they deny it.

### Consent granted

Create `scenarios/consent-grant.scenario.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: consent-grant
spec:
  id: consent-grant
  task_type: chat
  description: "Verifies that a client tool executes when consent is granted (default)"

  turns:
    - role: user
      content: "Where am I right now?"
      assertions:
        - type: tool_called
          params:
            tool_names: [get_location]
          message: "Should call get_location tool when user asks for location"

    - role: user
      content: "Tell me more about this area"
      assertions:
        - type: content_matches
          params:
            pattern: "(?i)(san francisco|37\\.77|location)"
          message: "Should reference the location data from the tool result"
```

When no `consent_overrides` are specified, consent defaults to grant. The model calls `get_location`, receives the `mock_result` data, and uses it in its response.

### Consent denied

Create `scenarios/consent-deny.scenario.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: consent-deny
spec:
  id: consent-deny
  task_type: chat
  description: "Verifies that the LLM handles consent denial gracefully"

  turns:
    - role: user
      content: "Where am I right now?"
      consent_overrides:
        get_location: deny
      assertions:
        - type: tool_call_count
          params:
            tool: get_location
            min: 1
          message: "Should attempt to call get_location tool"

    - role: user
      content: "Can you try again?"
      assertions:
        - type: content_matches
          params:
            pattern: "(?i)(unable|denied|provide|manually|sorry)"
          message: "Should explain that location access was denied and suggest alternatives"
```

`consent_overrides: { get_location: deny }` simulates the user declining the consent prompt on that turn.

The first assertion is `tool_call_count` because `tool_called` counts only calls that succeed, and a denied call does not.

## Step 4: Add a mock provider

The mock provider replays scripted replies, so the run needs no API key. Create `providers/mock-provider.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: mock-client-tools
spec:
  id: mock-client-tools
  type: mock
  model: mock-model
  additional_config:
    mock_config: mock-responses.yaml
```

Create `mock-responses.yaml` in the project root. Each scenario's replies are keyed by the scenario's `metadata.name`. Turn 1 asks for the tool, and turn 2 is plain text:

```yaml
defaultResponse: "I can help you with that."

scenarios:
  consent-grant:
    defaultResponse: "Here is your location information."
    turns:
      1:
        tool_calls:
          - name: "get_location"
            arguments:
              accuracy: "high"
      2: "Based on your location at latitude 37.7749, longitude -122.4194, you appear to be in San Francisco, California."

  consent-deny:
    defaultResponse: "I'm unable to access your location."
    turns:
      1:
        tool_calls:
          - name: "get_location"
            arguments:
              accuracy: "high"
      2: "I'm sorry, I was unable to access your location because the request was denied. Could you please provide your location manually so I can assist you?"
```

## Step 5: Create the arena configuration

Create `config.arena.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: client-tools-consent
spec:
  prompt_configs:
    - id: chat
      file: prompts/chat.yaml

  providers:
    - file: providers/mock-provider.yaml

  scenarios:
    - file: scenarios/consent-grant.scenario.yaml
    - file: scenarios/consent-deny.scenario.yaml

  tools:
    - file: tools/get_location.tool.yaml

  defaults:
    temperature: 0.0
    max_tokens: 500
    seed: 42
    output:
      dir: out
      formats: ["json", "markdown"]
```

## Step 6: Run the tests

```bash
promptarena run --simple
```

The run ends with a summary. Under `--ci` the log also shows a `tool hook denied` warning, which is the consent-deny scenario declining the call; `--simple` leaves it out.

```text
Run Summary
────────────────────────────────────────────────────

Total runs      2
Passed          2 (100.0%)
Failed          0 (0.0%)

Assertions      4 total
Assertions Pass all passed

Total cost      $0.0029
Total tokens    285
Total duration  13ms
Avg duration    6ms per run

Providers       mock-client-tools (2)
Scenarios       2 scenarios
Regions         default

Saved to        out
```

Open `out/results.md`. It has one row per scenario, `consent-grant` and `consent-deny`, each with a pass status, two passing assertions and one tool call.

To browse the results in the web UI, run `promptarena serve` from the project directory.

## What we built

1. A client-mode tool with `mode: client` and a `mock_result`.
2. Consent simulation through `client.consent` on the tool and `consent_overrides` on a turn.
3. A grant scenario and a deny scenario, run against a mock provider.

The mock provider scripts the replies, so the run shows that the tool call is made and that the assertions match.

For a complete working example, see the [`examples/client-tools/`](https://github.com/AltairaLabs/promptarena/tree/main/examples/client-tools) directory in the PromptArena repository.

## Next steps

- [Tutorial 5: CI Integration](/arena/tutorials/05-ci-integration/) automates client-tool tests in your CI/CD pipeline.
- [Tutorial 4: MCP Tools](/arena/tutorials/04-mcp-tools/) covers server-side tool testing for comparison.
