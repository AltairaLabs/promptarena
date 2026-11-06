---
title: Write Test Scenarios
description: Create a scenario file, add turns and assertions, and choose which providers it runs against.
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/arenaconfig/types.go
    - arena/assertions/types.go
    - arena/cmd/promptarena/init.go
    - arena/cmd/promptarena/run.go
    - arena/engine/execution.go
    - arena/turnexecutors/pipeline_stages_integration.go
---

Write a scenario: the scripted conversation Arena sends to a provider, with the assertions each response must pass.

## Prerequisites

- The `promptarena` CLI.
- An arena config with a prompt and a provider, or a new project from `promptarena init`.

## Steps

1. Start from a generated project:

   ```bash
   promptarena init my-test --quick --provider mock
   ```

   This writes a working scenario to `my-test/scenarios/basic-test.scenario.yaml`, listed under `spec.scenarios` in `my-test/config.arena.yaml`. Edit it, or add your own file next to it.

2. Write the scenario file. `spec.task_type` selects the prompt config with the same `task_type`:

   ```yaml
   apiVersion: promptkit.altairalabs.ai/v1alpha1
   kind: Scenario
   metadata:
     name: basic-customer-inquiry
     labels:
       category: customer-service
       priority: basic

   spec:
     task_type: support
     description: "Tests customer support responses"

     turns:
       - role: user
         content: "What are your business hours?"
         assertions:
           - type: content_includes
             params:
               patterns: ["Monday"]
             message: "Should mention business days"

       - role: user
         content: "Do you offer weekend support?"
         assertions:
           - type: content_includes
             params:
               patterns: ["Saturday"]
             message: "Should mention weekend availability"
   ```

   Each turn is one user message. The turns run in order in one conversation, so later turns test context retention. `message` sits beside `params`, not inside it.

3. Add the scenario to the arena config:

   ```yaml
   spec:
     scenarios:
       - file: scenarios/basic-customer-inquiry.yaml
   ```

4. Assert on tool use where the turn should call a tool:

   ```yaml
   turns:
     - role: user
       content: "What's the weather in San Francisco?"
       assertions:
         - type: tools_called
           params:
             tools: ["get_weather"]
           message: "Should call weather tool"
         - type: content_includes
           params:
             patterns: ["temperature"]
           message: "Should mention temperature"
   ```

   To run an assertion only when a condition holds, add `when`. The assertion is skipped, not failed, when the condition is not met:

   ```yaml
         - type: content_includes
           params:
             patterns: ["shipped"]
           when:
             tool_called: lookup_order
   ```

   `when` takes `tool_called`, `tool_called_pattern`, `any_tool_called` and `min_tool_calls`.

5. Prefer loose assertions to exact ones. An exact sentence breaks on any rewording; a phrase plus a format check does not:

   ```yaml
   assertions:
     - type: content_includes
       params:
         patterns: ["account balance"]
       message: "Should mention balance"
     - type: content_matches
       params:
         pattern: '\$\d+\.\d{2}'
       message: "Should include dollar amount"
   ```

6. If the scenario needs a feature only some providers have, list it in `required_capabilities`. The scenario then runs only against providers whose `capabilities` list contains every entry:

   ```yaml
   spec:
     task_type: capability-test
     description: "Test vision with streaming"
     required_capabilities:
       - vision
       - streaming
     streaming: true

     turns:
       - role: user
         parts:
           - type: text
             text: "Describe this image"
           - type: image
             media:
               url: "https://example.com/image.png"
               mime_type: "image/png"
   ```

   Capability names: `text`, `streaming`, `vision`, `tools`, `json`, `audio`, `video`, `documents`.

7. To run the scenario against named providers only, list their IDs in the scenario's `providers`. Without it, the scenario runs against every provider in its provider group:

   ```yaml
   spec:
     providers: [openai-gpt4, claude-sonnet, gemini-pro]
   ```

8. To control tool calling, set `tool_policy`:

   ```yaml
   spec:
     tool_policy:
       tool_choice: required
       max_tool_calls_per_turn: 3
   ```

   `tool_choice` is `auto`, `required`, `none` or a tool name. The scenario's `tool_policy` is merged with the `tool_policy` of the prompt under test, and the merge only tightens:

   - `max_rounds` and `max_tool_calls_per_turn`: the lower of the scenario's and the prompt's value applies. With neither set, `max_rounds` defaults to 50.
   - `blocklist`: the scenario's and the prompt's lists are combined.
   - `tool_choice`: the scenario's value is used when it sets one, otherwise the prompt's, otherwise `auto`.

   A scenario can lower the prompt's limits, but cannot raise them.

## Verify

Run the scenario by its `metadata.name`:

```bash
promptarena run --scenario basic-customer-inquiry
```

The run summary lists one result per provider, and `out/results.md` shows each assertion's outcome and `message`. `--scenario` matches scenario names only. Labels in `metadata.labels` appear in the results, and `promptarena run --objectives-group-by label.<key>` groups the objectives report by them.

## Related pages

- [Scenario Format Reference](/arena/reference/scenario-format/)
- [Configuration schema: Scenario](/arena/reference/config-schema/#scenario), for every field
- [Assertions Reference](/arena/reference/assertions/)
- [Validate Outputs](/arena/how-to/scenarios/validate-outputs/)
- [Run a Scenario Over a Dataset](/arena/how-to/scenarios/run-a-dataset/)
- [Configure Providers](/arena/how-to/providers/configure-providers/)
- [Examples](/arena/examples/)
