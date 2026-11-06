---
title: Pack Evals
description: Fields of an eval definition in a pack's evals array, with the triggers, groups and metric types it accepts.
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - arena/engine/builder_integration.go
    - arena/engine/eval_conversation_executor.go
---

An eval is an entry in the `evals` array of a pack file, or of a prompt inside the pack. The runtime reads it as an `EvalDef`, which is generated from the PromptPack schema. This page lists the fields the runtime acts on, the trigger values that fire, the automatic groups and the metric definition.

## Eval definition

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `id` | string | required | Identifier, unique within the pack or within a prompt's evals. A prompt eval with the same `id` overrides the pack eval. |
| `description` | string | none | Human-readable description of what the eval measures. |
| `type` | string | required | Check type, from the [Checks Reference](https://promptkit.altairalabs.ai/reference/checks/). |
| `trigger` | string | required | When the eval fires. See [Triggers](#triggers). |
| `sample_percentage` | number, 0 to 100 | 5 | Percentage of turns or sessions to evaluate. Read only by `sample_turns` and `sample_sessions`. |
| `enabled` | boolean | `true` | Whether the eval runs. |
| `params` | object | none | Parameters for the check type. |
| `metric` | object | auto-generated gauge | Prometheus metric definition. See [Metric definition](#metric-definition). |
| `when` | object | none | Conditions on the turn's tool calls. When they are not met, the eval is skipped. |
| `groups` | array of strings | by `type` | Groups that filters select on. Setting it replaces the automatic classification. See [Groups](#groups). |

An eval with no `metric` records a gauge named after its `id`. To gate on a score, use the `assertion` or `guardrail` check type with `min_score` and `max_score` in its `params`; a plain eval records a score and never fails.

## Triggers

| Value | Fires |
|-------|-------|
| `every_turn` | After each assistant response. |
| `sample_turns` | On a hash-selected percentage of turns. |
| `on_session_complete` | When the session closes. |
| `sample_sessions` | On a hash-selected percentage of sessions, at session close. |
| `on_conversation_complete` | When a conversation ends. Runs in Arena and in `sdk.Evaluate` with `Trigger` set, and does not fire in a live SDK conversation. |

Sampling uses an FNV-1a hash of the session ID and turn index, so the same inputs always give the same decision. For `sample_sessions` the decision depends on the session ID only.

## Groups

An eval with no explicit `groups` gets groups from its `type`. `sdk.EvaluateOpts.EvalGroups` selects which groups run, and an empty list runs every eval.

| Group | Assigned to |
|-------|-------------|
| `default` | Every eval with no explicit groups. |
| `fast-running` | Every type not listed under `long-running` or `external`, such as `contains`, `regex`, `json_valid` and `tools_called`. |
| `long-running` | `llm_judge`, `llm_judge_session`, `llm_judge_tool_calls`, `cosine_similarity`, `outcome_equivalent`, `a2a_eval`, `a2a_eval_session`, `rest_eval`, `rest_eval_session` and exec handlers. |
| `external` | `llm_judge`, `llm_judge_session`, `llm_judge_tool_calls`, `a2a_eval`, `a2a_eval_session`, `rest_eval`, `rest_eval_session` and exec handlers. |

## Metric definition

The `metric` object takes these fields. The schema allows additional properties, and `labels` is one the runtime reads.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | Metric name, matching `^[a-zA-Z_:][a-zA-Z0-9_:]*$`. The collector prefixes it with `{namespace}_eval_` unless it already starts with that prefix. |
| `type` | string | required | One of the metric types below. |
| `labels` | map of string to string | none | Labels added to the metric. Label names are not checked when the pack loads; a name Prometheus rejects fails metric registration and logs a warning. |

| Metric type | Behavior |
|-------------|----------|
| `gauge` | Set to the eval's value, which is the score unless the eval supplies another. |
| `counter` | Incremented on each execution. |
| `histogram` | Observes the value into `prometheus.DefBuckets`. The buckets are not configurable. |
| `boolean` | 1.0 if the value is at least 1.0, otherwise 0.0. |

An eval with no `metric` gets a gauge named after its `id` with no pack-author labels. The `id` is used as given, so an `id` containing a hyphen produces a metric name that the pack schema's `name` pattern does not allow.

## Example

```json
{
  "id": "quality_check",
  "type": "contains",
  "trigger": "every_turn",
  "params": { "patterns": ["thank you"] },
  "enabled": true,
  "metric": {
    "name": "response_quality",
    "type": "gauge",
    "labels": { "category": "tone" }
  }
}
```

## Related pages

- [Eval Framework](/arena/explanation/eval-framework/): how evals run and where the results go
- [Checks Reference](https://promptkit.altairalabs.ai/reference/checks/): all check types and parameters
- [Assertions Reference](/arena/reference/assertions/): test-time checks
- [Metrics Reference](https://promptkit.altairalabs.ai/runtime/reference/metrics/): all emitted metrics
