---
title: 'Deploy: Target Capabilities'
description: What each deploy target supports, so you can pick one before writing config
sidebar:
  order: 5
---

The three first-party adapters deploy the same prompt pack to very different
substrates, and they are not feature-equivalent. This page is the comparison to
read **before** choosing, not after.

Every cell below was checked against adapter source, not adapter marketing.
Cells marked *unverified* are stated as such rather than guessed.

## At a glance

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Substrate | Omnia Kubernetes workspace | AWS Bedrock AgentCore | Google Agent Runtime |
| Unit of deployment | CRDs in a workspace | AgentCore runtimes + supporting resources | One engine per agent |
| Maturity | Most complete | Feature-rich, older | Newest, narrowest |

## Adapter lifecycle

| Method | Omnia | AgentCore | Vertex |
|---|---|---|---|
| `get_provider_info` | Yes | Yes | Yes |
| `validate_config` | Yes | Yes | Yes |
| `plan` | Yes | Yes | Yes |
| `apply` | Yes | Yes | Yes |
| `destroy` | Yes | Yes | **No** |
| `status` | Yes | Yes | **No** |
| `import` | No | No | No |

All three return an explicit "not yet supported" error for `import`. Vertex is
the only one where tearing a deployment down is a manual step.

## What gets created

| Resource | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Agent runtime | `agent_runtime` | `agent_runtime` | `agent_runtime` |
| Pack | `prompt_pack` CRD | Injected as env | Injected as env, or `pack_object` in GCS |
| Tools | `tool_registry` CRD | `tool_gateway` (MCP targets) | Injected as env |
| Policy | `agent_policy` CRD | `cedar_policy` | — |
| Memory | Config on the runtime | `memory` | — |
| Evals | Config on the runtime | `evaluator`, `online_eval_config` | Run in-process from the pack |
| A2A | Via the platform | `a2a_endpoint` | — |

The shape of that column tells you most of what you need. Omnia and AgentCore
model the agent's capabilities as **platform resources**; Vertex ships
everything inside the container as environment. That makes Vertex simpler to
reason about and gives it far fewer knobs.

## Tools

All three adapters read the arena's `tool_specs`. What they *do* with them
differs more than the shared field name suggests.

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Where a tool runs | Platform, via a `tool_registry` CRD | Platform, via `tool_gateway` targets | **In the agent's own container** |
| Dispatches on the spec's `mode` | No | No | Yes |
| HTTP transports | HTTP with full request/response mapping | HTTP, Lambda ARN, API Gateway, OpenAPI, Smithy | HTTP `url` and `method` only |
| Additional handler types | `openapi`, `grpc`, `mcp`, `client` via its own `tools:` block | MCP via the gateway | — |
| Credential providers | Headers from env, static headers, redaction | `GATEWAY_IAM_ROLE`, `OAUTH`, `API_KEY` | **None** |
| `mock_result` / `mock_template` forwarded | No | No | **Yes** |

Two consequences worth knowing before you pick:

**Only Vertex runs mock tools.** Neither the Omnia nor the AgentCore adapter
carries `mock_result` or `mock_template` anywhere, and the AgentCore runtime has
no mock handling either — a `mock` tool on those targets has nothing behind it.
If your pack leans on mocked tools, Vertex is currently the only target that
reproduces `promptarena run` behaviour.

**Only Omnia and AgentCore authenticate live tools.** Vertex forwards URL and
method and nothing else — no headers, timeouts, redaction or request/response
mapping — so a live tool needing an auth header cannot work there yet.

On every target, if `tool_specs` never reaches the adapter, tools are advertised
to the model with nothing able to run them, and the model apologises instead of
answering.

## Invocation

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Unary request/response | Yes | `POST /invocations` | `POST /api/reasoning_engine` |
| Server-streamed output | Platform-dependent *(unverified)* | SSE on `/invocations` | ndjson on `/api/stream_reasoning_engine` |
| Persistent bidirectional socket | *(unverified)* | `GET /ws` upgrade, **text messages** | No |
| Server-side sessions | Via the platform | `X-Amzn-Bedrock-AgentCore-Runtime-Session-Id` header | Not used by the adapter |
| **Duplex voice** | **No** | **No** | **No** |

Two things are worth separating here, because the names invite confusion.

**A WebSocket is not duplex voice.** AgentCore's runtime does upgrade `/ws` and
hold the connection open, but `processWSMessage` unmarshals each frame as a
text request with `prompt`/`input` and writes one response back. It is
request/response over a persistent socket. Nothing in it carries audio frames,
and no adapter runtime calls PromptKit's `OpenDuplex()`.

**PromptKit itself can do duplex.** `sdk.OpenDuplex()` exists alongside
`Open()`, with `SendChunk`, `SendFrame`, `SendVideoChunk`, VAD and barge-in
handling. The capability is real; no deploy target exposes it. For Vertex this
is a hard block — the `reasoningEngines` REST surface has `query`, `streamQuery`
and `asyncQuery`, all request/response, and no bidirectional method exists
anywhere in the v1beta1 API.

Whether the AWS and Google control planes proxy a WebSocket upgrade through to
a custom container is *unverified* in both cases.

## Sessions and conversation state

Only AgentCore carries multi-turn context for you, via its session-id header.

The Vertex runtime opens a fresh PromptKit conversation per request and holds no
state between calls, so multi-turn context is the caller's responsibility. Agent
Runtime does expose a `sessions` API, but the adapter does not use it.

## Observability

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Config surface | Cluster-provided | `observability.tracing_enabled` | `observability.tracing_enabled` + `otlp_endpoint` |
| Traces | Cluster tooling | CloudWatch | Any OTLP collector |
| Metrics | Cluster tooling | CloudWatch metrics from evals | **None** |
| Eval scores in telemetry | *(unverified)* | Via `online_eval_config` | `gen_ai.evaluation.score` spans |
| Guardrail firings | *(unverified)* | *(unverified)* | **Logged only, never traced** |

The guardrail gap is a PromptKit-level issue, not a Vertex one: a `validators`
entry runs through the guardrail hook adapter, which emits no event, so a firing
guardrail rewrites the response and leaves no trace attribute. Declare an
`evals` section if you need scores in telemetry.

## Authentication

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| Adapter → control plane | `OMNIA_API_TOKEN` | AWS credentials | Application Default Credentials |
| Agent → model provider | Provider CRD reference | AWS IAM | ADC as the engine's service account |
| API keys in config | Avoidable, use the env var | No | No — none exist |

## Multi-agent

| | Omnia | AgentCore | Vertex |
|---|---|---|---|
| One deployment per agent | Yes | Yes | Yes |
| Routing between agents | Via the platform | `a2a_endpoint` wiring | **None** |

A multi-agent pack on Vertex becomes several independent engines that cannot
call each other. Agent cards are generated but cannot be attached:
`spec.agentCard` exists in the v1beta1 REST discovery document and is absent
from the published protobufs, so the Go client has no field to set.

## Choosing

- **Omnia** if you want the fullest feature set and are already on the Altaira
  platform. Richest tool handler support, full lifecycle.
- **AgentCore** if you are on AWS and need memory, Cedar policies, A2A wiring or
  session continuity as managed resources.
- **Vertex** if you want Gemini models with the least moving parts, or if your
  pack depends on mocked tools. Accept that teardown is manual, there is no A2A,
  and live tools cannot authenticate.

None of them do duplex voice today.
