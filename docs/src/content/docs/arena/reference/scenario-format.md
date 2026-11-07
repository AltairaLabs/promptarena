---
title: Test Scenario Format
description: The structure of a Scenario file, its main spec and turn fields, and examples of multimodal, workflow and self-play turns.
verified:
  commit: 95b220864e8df93d5a36b38ad288897bf70a4c5b
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - arena/arenaconfig/validator.go
    - arena/arenaconfig/validator_tools.go
    - arena/assertions/validate_types.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/engine/builder_integration.go
    - arena/engine/conversation_executor.go
    - arena/engine/eval_orchestrator.go
    - arena/engine/execution_workflow_integration.go
    - arena/engine/workflow_state_resolver.go
    - arena/engine/workflow_tool_executor.go
    - arena/turnexecutors/media_loader.go
    - arena/turnexecutors/pipeline_stages_integration.go
    - examples/arena-media-test/config.arena.yaml
    - examples/assertions-test/config.arena.yaml
    - examples/document-analysis/scenarios/pdf-summary.scenario.yaml
    - packc/schema_validator.go
    - packc/schema_validator_test.go
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/persona.json
    - schemas/v1alpha1/promptconfig.json
    - schemas/v1alpha1/provider.json
    - schemas/v1alpha1/scenario.json
    - schemas/v1alpha1/tool.json
---
A scenario file declares the conversation `promptarena run` plays against your providers: the turns to send, the assertions to check and the variables to inject. Scenarios are Kubernetes-style YAML resources that `promptarena validate` checks against `schemas/v1alpha1/scenario.json`. PromptArena and `packc` validate packs against the embedded schema of the open [PromptPack](https://promptpack.org) specification, version 1.9.0.

## File structure

| Key | Type | Description |
|---|---|---|
| `apiVersion` | string | `promptkit.altairalabs.ai/v1alpha1`. |
| `kind` | string | `Scenario`. |
| `metadata` | object | `name` and optional `labels`. Labels are copied onto the scenario for stratified reporting. |
| `spec` | object | The scenario body. See [Spec fields](#spec-fields). |

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: order-tracking
spec:
  id: order-tracking
  task_type: support
  description: "Track an order"
  turns:
    - role: user
      content: "Where is order #12345?"
```

## Spec fields

| Field | Type | Description |
|---|---|---|
| `id` | string | Scenario identifier. |
| `task_type` | string | Prompt task type the scenario runs against. |
| `description` | string | What the scenario tests. Required. |
| `turns` | list | Conversation turns. See [Turn fields](#turn-fields). |
| `variables` | map | Template variables injected into the pack. |
| `conversation_assertions` | list | Assertions evaluated after the whole conversation completes. |
| `providers` | list | Provider ids to run against. Empty means the providers in `provider_group` (default `default`), or every provider when none is in that group. |
| `provider_group` | string | Provider group to run against. |
| `required_capabilities` | list | Keeps only providers that support every listed capability: `text`, `streaming`, `vision`, `tools`, `json`, `audio`, `video`, `documents`. |
| `streaming` | bool | Streams every turn by default. A turn's `streaming` overrides it. |
| `trials` | int | Number of times to run the scenario. Above 1, assertion results aggregate across trials as pass rates and flakiness scores. |
| `trial_budget` | object | Stops trials early once one achieves every objective, or once they have cost a set amount. |
| `data` | object | Runs the scenario once per row of a JSONL or CSV file, substituting columns as `{column}`. |
| `context_policy` | object | Context management for long conversations. |
| `context_metadata` | object | `domain`, `user_role` and `project_stage`. |
| `context`, `constraints` | map | Free-form scenario context and constraints. |
| `tool_policy` | object | Tool-call limits for the scenario. |
| `tool_result_transforms` | list | Rewrites named tools' results before the agent sees them. |
| `seed_memories` | list | Memories written to the memory store before the first turn. Each has `content`, and optionally `type`, `confidence` and `metadata`. |
| `duplex` | object | Bidirectional streaming for voice and audio scenarios. |
| `voice`, `voices` | string, list | Arena voice ids that synthesize scripted user turns. `voices` cycles one voice per trial and cannot be set with `voice`. |

Every field, including nested objects, is documented in the [Scenario reference](/arena/reference/config-schema/#scenario).

## Turn fields

| Field | Type | Description |
|---|---|---|
| `role` | string | `user`, or the ID of a self-play role from `spec.self_play.roles` (such as `gemini-user`). The assistant's reply is never scripted: `promptarena validate` rejects any other role, `assistant` included. Required. |
| `content` | string | Turn text. |
| `parts` | list | Multimodal content parts. Takes precedence over `content` when non-empty. |
| `assertions` | list | Assertions evaluated on this turn. |
| `persona` | string | Persona id for a self-play turn. |
| `turns` | int | Self-play exchange count. The exact count when `max_turns` is absent. |
| `max_turns` | int | Self-play upper bound. Enables natural termination when above `turns`. |
| `assistant_temp`, `user_temp` | float | Temperature overrides for the assistant and the self-play user. |
| `seed` | int | Overrides the self-play persona's seed. |
| `streaming` | bool | Overrides the scenario's `streaming` for this turn. |
| `consent_overrides` | map | Consent simulation for client-side tools. Keys are tool names; values are `grant`, `deny` or `timeout`. |
| `perturbations` | map | Placeholder name to list of variants. Each variant runs as its own trial with `{name}` substituted in `content`. |
| `chaos` | object | Fault injection for resilience testing. |
| `objective` | object | Goal for a self-play turn, checked after every exchange. |
| `transforms` | list | Rewrites each persona message before the agent receives it. |
| `audio_transforms` | list | Rewrites the audio of a spoken user message. Duplex scenarios only. |

### Content parts

| Field | Type | Description |
|---|---|---|
| `type` | string | `text`, `image`, `audio` or `video`. |
| `text` | string | Text, for `type: text`. |
| `media` | object | Media for the other types. |

`media` takes one source (`file_path`, `url`, `data` or `storage_reference`) and a required `mime_type`. `detail` (`low`, `high` or `auto`) and `caption` are optional.

## Examples

### Multimodal turn

A `parts` list mixes text with images from a file path or a URL:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: describe-image
spec:
  id: describe-image
  task_type: media-analysis
  description: "Describe two images"
  turns:
    - role: user
      parts:
        - type: text
          text: "What is in this image?"
        - type: image
          media:
            file_path: media/photo.jpg
            mime_type: image/jpeg
            detail: high
        - type: image
          media:
            url: https://example.com/chart.png
            mime_type: image/png
      assertions:
        - type: content_includes
          params:
            patterns: ["image"]
```

The prompt's `spec.media` block enables media types and sets size limits per type.

### Workflow scenario

A workflow scenario is a `kind: Scenario` with ordinary `turns`. The state machine is declared in the arena config under `spec.workflow`, which registers the `workflow__transition` tool and selects the prompt per turn. The LLM initiates each transition by calling that tool with an `event` and a `context` string, and the context is available as `{{workflow_context}}` in the new state's system prompt. Turn assertions `state_is`, `transitioned_to` and `workflow_complete` check the state:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: support-escalation
spec:
  id: support-escalation
  task_type: intake              # entry state of the workflow
  description: "Test customer support escalation flow"
  variables:
    company_name: "Acme Corp"
  turns:
    - role: user
      content: "I need help with my billing"
      assertions:
        - type: state_is
          params:
            state: "intake"
          message: "Should be in intake state"
    - role: user
      content: "My invoice shows a duplicate charge of $49.99"
      assertions:
        - type: transitioned_to
          params:
            state: "specialist"
          message: "Should have transitioned to specialist"
    - role: user
      content: "Thank you, the refund looks correct!"
      assertions:
        - type: workflow_complete
          message: "Workflow should be complete"
```

| Assertion | Checks |
|---|---|
| `state_is` | The current workflow state. |
| `transitioned_to` | That a state appears in the transition history. |
| `workflow_complete` | That the workflow reached a terminal state. |

### Self-play turn

A turn whose `role` is a provider selector generates the user message from a persona:

```yaml
turns:
  - role: user
    content: "My order hasn't arrived yet."
  - role: gemini-user
    persona: frustrated-customer
    turns: 3
    max_turns: 8
```

See [Persona (Self-Play)](/arena/reference/config-schema/#persona-self-play) for the persona resource.

## Related pages

- [Scenario reference](/arena/reference/config-schema/#scenario): every scenario field, with nested objects.
- [Assertions](/arena/reference/assertions/): assertion types and parameters, including the workflow assertions.
- [Write scenarios](/arena/how-to/scenarios/write-scenarios/): author a scenario step by step.
- [Workflow regression](/arena/how-to/scenarios/workflow-regression/): test workflow-based packs.
- [Mock providers](/arena/how-to/providers/use-mock-providers/): run scenarios without API costs.
- [PromptPack.org](https://promptpack.org): the PromptPack specification.
