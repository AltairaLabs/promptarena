---
title: 'Deploy: Target Capabilities'
description: What the Omnia, AgentCore and Vertex deploy adapters support, cell by cell
sidebar:
  order: 5
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/adapter_registry.json
---

The adapter registry lists four first-party adapters: `agentcore`, `omnia`,
`foundry` (Azure AI Foundry hosted agents) and `vertex`. This page compares
Omnia, AgentCore and Vertex. They deploy the same prompt pack to different
substrates and are not feature-equivalent.

Every cell was checked against adapter source. Cells marked *unverified* could
not be checked.

## At a glance

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Substrate | Omnia Kubernetes workspace | AWS Bedrock AgentCore | Google Agent Runtime |
| Unit of deployment | CRDs in a workspace | AgentCore runtimes + supporting resources | One engine per agent |

## Adapter lifecycle

| Method | Omnia | AgentCore | Vertex |
|---|---|---|---|
| `get_provider_info` | Yes | Yes | Yes |
| `validate_config` | Yes | Yes | Yes |
| `plan` | Yes | Yes | Yes |
| `apply` | Yes | Yes | Yes |
| `destroy` | Yes | Yes | Yes |
| `status` | Yes | Yes | Yes |
| `import` | No | No | No |

All three reject `import` with an error saying it is unsupported. Vertex
`destroy` deletes every engine recorded in state, continues past individual
failures and treats already-deleted engines as success. Engines recorded
mid-creation have no resource name and are reported as needing manual removal.
Vertex `status` looks each engine up live and reports `deployed`,
`not_deployed` or `degraded`, with per-engine `healthy`, `unhealthy` or
`missing`.

## What gets created

| Resource | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Agent runtime | `agent_runtime` | `agent_runtime` | `agent_runtime` |
| Pack | `prompt_pack` CRD | Injected as env | Injected as env, or `pack_object` in GCS |
| Tools | `tool_registry` CRD | `tool_gateway` (MCP targets) | Injected as env |
| Policy | `agent_policy` CRD | `cedar_policy` | — |
| Memory | Config on the runtime | `memory` | — |
| Evals | Config on the runtime | `evaluator`, `online_eval_config` | Run in-process from the pack |
| A2A | Via the platform *(unverified)* | `a2a_endpoint` | Agent cards attached to each engine |

Omnia and AgentCore model the agent's capabilities as platform resources.
Vertex ships everything inside the container as environment.

## Tools

All three adapters read the arena's `tool_specs`.

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Where a tool runs | Platform, via a `tool_registry` CRD | `tool_gateway` for tools with a Lambda ARN, API Gateway, OpenAPI or Smithy target; mock and plain HTTP (`url` + `method`) tools run in the runtime container | In the agent's own container |
| Dispatches on the spec's `mode` | No | No (mode is inferred from the `lambda_arn`, `http` and mock fields) | Yes |
| HTTP transports | HTTP with full request/response mapping | HTTP, Lambda ARN, API Gateway, OpenAPI, Smithy | HTTP `url`, `method`, `headers`, `headers_from_env`, `timeout_ms` and `redact`; no request/response mapping |
| Additional handler types | `openapi`, `grpc`, `mcp`, `client` via its own `tools:` block | MCP via the gateway *(unverified)* | — (`mcp`, `exec` and `client` tools are logged once and left unregistered) |
| Credential providers | Headers from env, static headers, redaction | `GATEWAY_IAM_ROLE`, `OAUTH`, `API_KEY` for Gateway targets | Static `headers` and `headers_from_env` (`Header=ENV_VAR`, read in the runtime at call time; env set via the deploy config's `env` block) |
| `mock_result` / `mock_template` forwarded | No | Yes (`PROMPTPACK_TOOL_SPECS`; the runtime builds mock tools, `mock_template` wins over `mock_result`) | Yes |

Mock tools run on AgentCore and Vertex. Omnia carries neither `mock_result`
nor `mock_template`; it gives mock tools a placeholder handler and a plan
warning.

AgentCore's plain-HTTP live path forwards `url` and `method` only. Its
authentication is the Gateway credential provider for Gateway targets.

On Vertex, if `tool_specs` never reaches the adapter, tools are advertised to
the model with nothing able to run them.

## Invocation

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Unary request/response | Yes | `POST /invocations` | `POST /api/reasoning_engine` |
| Server-streamed output | Platform-dependent *(unverified)* | SSE on `/invocations` | ndjson on `/api/stream_reasoning_engine` |
| Persistent bidirectional socket | Websocket facade (type `websocket`, handler `runtime`) on every AgentRuntime | `GET /ws` upgrade, text messages | No |
| Server-side sessions | Via the platform *(unverified)* | `X-Amzn-Bedrock-AgentCore-Runtime-Session-Id` header | `session_id` (also `sessionId`, `session`) in the request body |
| Duplex voice | *(unverified)* | No | No |

AgentCore's `/ws` upgrade holds the connection open. `processWSMessage`
unmarshals each frame as a text request with `prompt` or `input` and writes one
response back. No frame carries audio, and no adapter runtime calls PromptKit's
`OpenDuplex()`.

PromptKit provides `sdk.OpenDuplex()` alongside `Open()`, with `SendChunk`,
`SendFrame`, `SendVideoChunk`, VAD and barge-in handling. No deploy target
exposes it. The `reasoningEngines` REST surface has `query`, `streamQuery` and
`asyncQuery`, all request/response, and no bidirectional method exists in the
v1beta1 API *(unverified)*.

Whether the AWS and Google control planes proxy a WebSocket upgrade through to
a custom container is *unverified* in both cases.

## Sessions and conversation state

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Multi-turn context | Platform-side *(unverified)* | Session-id header, mapped to the A2A `contextId` | Agent Runtime sessions, when the request includes a session id |

The Vertex runtime opens a fresh PromptKit conversation per request. With a
session id it loads state from, and appends to, an Agent Runtime session
(`GetSession`, `ListEvents`, `AppendEvent`, `CreateSession`). A request without
a session id is stateless. Sessions require `GOOGLE_CLOUD_AGENT_ENGINE_ID`,
which Agent Runtime injects.

## Observability

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Config surface | Cluster-provided | `observability.tracing_enabled`, `otlp_endpoint` (optional, full URL), `cloudwatch_log_group` | `observability.tracing_enabled` + `otlp_endpoint` (required when tracing is enabled) |
| Traces | Cluster tooling | CloudWatch *(unverified)* | Any OTLP collector |
| Metrics | Cluster tooling | CloudWatch metrics from evals | None |
| Eval scores in telemetry | *(unverified)* | Via `online_eval_config` | `gen_ai.evaluation.score` spans |
| Guardrail firings | *(unverified)* | *(unverified)* | Logged only, never traced |

A `validators` entry runs through the guardrail hook adapter, which emits no
event, so a firing guardrail rewrites the response and leaves no trace
attribute *(unverified)*. Declare an `evals` section to get scores in
telemetry.

## Authentication

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Adapter → control plane | `OMNIA_API_TOKEN` | AWS credentials | Application Default Credentials |
| Agent → model provider | Provider CRD reference | AWS IAM | ADC as the engine's service account |
| API keys in config | Avoidable, use the env var | No | No: none exist |

## Multi-agent

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| One deployment per agent | Yes | Yes | Yes |
| Routing between agents | Via the platform *(unverified)* | `a2a_endpoint` wiring | None |

A multi-agent pack on Vertex becomes several independent engines that cannot
call each other. The adapter generates agent cards and attaches them by
patching `spec.agentCard` over REST on create and update. A failure is logged
and does not fail the apply. The generated Go client has no field for
`spec.agentCard`, which is absent from the published protobufs
*(unverified)*. The plan warns for `a2a__` tools.
