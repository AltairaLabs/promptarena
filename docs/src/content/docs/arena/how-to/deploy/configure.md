---
title: 'Deploy: Configure'
description: Add a deploy section to your arena config, bind it to an adapter, and override it per environment.
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/deploy_config_interactive.go
    - arena/cmd/promptarena/deploy_interactive.go
    - arena/deploy/flow/config.go
    - arena/deploy/flow/connect.go
    - arena/deploy/flow/options.go
    - arena/deploy/flow/session.go
    - deploy/manager.go
    - deploy/provider.go
---

## Goal

Add a `deploy` section to your arena config so `promptarena deploy` can target a provider.

## Prerequisites

- An adapter installed (see [Install Adapters](/arena/how-to/deploy/install-adapters/))
- An arena config file (`arena.yaml` by default)

## Basic Configuration

The arena config is a manifest, so `deploy` goes under `spec`, beside `prompt_configs` and `providers`. A top-level `deploy:` key fails schema validation with `Additional property deploy is not allowed`.

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: my-agent
spec:
  # prompt_configs, providers, defaults, ...
  deploy:
    provider: omnia
    config:
      api_endpoint: https://omnia.example.com
      workspace: my-workspace
      providers:
        - { name: default, ref: claude-prod, role: llm }
```

| Field | Required | Description |
|-------|----------|-------------|
| `provider` | Yes | Adapter name. The CLI runs the binary `promptarena-deploy-{provider}`. |
| `config` | No | Provider-specific configuration, sent to the adapter as JSON |
| `environments` | No | Per-environment `config` overrides, keyed by environment name |

The remaining examples on this page show only the `deploy` block, which sits under `spec`.

## Provider Config

The CLI does not interpret `config`; each adapter defines its own fields. The tables below list the main fields for two adapters. Each adapter's own documentation lists every field.

### Omnia

The Omnia adapter targets an Omnia workspace. It needs the workspace API endpoint, the workspace name, a token, and the provider bindings for the deployed `AgentRuntime`:

```yaml
deploy:
  provider: omnia
  config:
    api_endpoint: https://omnia.example.com
    workspace: my-workspace
    # token: export OMNIA_API_TOKEN, or run `promptarena deploy login`
    providers:
      - { name: default,    ref: claude-prod,  role: llm }
      - { name: embeddings, ref: text-embed-3, role: embedding }
    runtime:
      replicas: 2
      autoscaling:
        enabled: true
        type: hpa                        # hpa | keda (keda enables scale-to-zero)
        min_replicas: 2
        max_replicas: 10
    labels:
      team: platform
```

| Field | Required | Description |
|-------|----------|-------------|
| `api_endpoint` | Yes | Omnia workspace API base URL |
| `workspace` | Yes | Workspace name, as its lowercase slug |
| `api_token` | Yes, unless supplied another way | `omnia_sk_` bearer token. Leave it out of the file and set `OMNIA_API_TOKEN` or run [`deploy login`](/arena/how-to/deploy/login/). |
| `providers` | Yes | Role-aware bindings `[{name, ref, role}]`. Roles: `llm`, `embedding`, `tts`, `stt`, `image`, `inference`. |
| `runtime` | No | Replica count, CPU and memory, and HPA or KEDA autoscaling |
| `labels` | No | Extra labels merged onto every created resource |

The CLI does not expand environment variables in the config file. `api_token: ${OMNIA_API_TOKEN}` sends that literal string as the token.

### AgentCore

The AWS Bedrock AgentCore adapter requires a region, the IAM role the runtime assumes, and the path to the pre-compiled runtime binary:

```yaml
deploy:
  provider: agentcore
  config:
    region: us-west-2
    runtime_role_arn: arn:aws:iam::123456789012:role/agentcore-runtime
    runtime_binary_path: ./dist/agentcore-runtime
```

| Field | Required | Description |
|-------|----------|-------------|
| `region` | Yes | AWS region, for example `us-west-2` |
| `runtime_role_arn` | Yes | IAM role ARN for the AgentCore runtime |
| `runtime_binary_path` | Yes | Path to the pre-compiled Go runtime binary |

## Environment Overrides

Add an `environments` entry for each environment whose config differs from the base:

```yaml
deploy:
  provider: agentcore
  config:
    region: us-west-2
    runtime_role_arn: arn:aws:iam::123456789012:role/agentcore-runtime
    runtime_binary_path: ./dist/agentcore-runtime
    tags:
      team: platform
  environments:
    production:
      config:
        region: us-east-1
        runtime_role_arn: arn:aws:iam::123456789012:role/agentcore-runtime-prod
```

### How Merging Works

When you run a command with `--env production`, the CLI builds the config it sends to the adapter:

1. It copies the base `config`.
2. It copies each top-level key of `environments.production.config` over it. A key in the environment replaces the base key, and a new key is added.
3. If the result has no `api_token` and `promptarena deploy login` stored a token for this provider and config file, it adds that token as `api_token`.

The merge is one level deep. A nested map in the environment, such as `tags`, replaces the base map entirely instead of merging into it.

Effective config for `--env production`:

```json
{
  "region": "us-east-1",
  "runtime_role_arn": "arn:aws:iam::123456789012:role/agentcore-runtime-prod",
  "runtime_binary_path": "./dist/agentcore-runtime",
  "tags": { "team": "platform" }
}
```

Without `--env`, the environment is `default`. With no `environments.default` entry, the adapter receives the base `config` unchanged.

## Validate Configuration

Run a plan. The Omnia and AgentCore adapters validate the config at the start of `plan` and fail with a list of the problems:

```bash
promptarena deploy plan --env production
```

## Specifying the Pack File and Config File

See [Plan and Apply](/arena/how-to/deploy/plan-and-apply/#specifying-a-pack-file) for `--pack` and `--config`. By default the CLI reads `arena.yaml` and compiles the pack from it.

## Troubleshooting

### Error: no deploy: section

`no deploy: section in "arena.yaml"` means the config has no `spec.deploy`. Add the section under `spec`, as in [Basic Configuration](#basic-configuration).

### Error: provider is required

Schema validation reports `spec.deploy: provider is required`. Add `provider` to the deploy section.

### Error: adapter not found for provider

The CLI can't find a binary named `promptarena-deploy-{provider}`. Install the adapter:

```bash
promptarena deploy adapter install omnia
```

## See Also

- [Install Adapters](/arena/how-to/deploy/install-adapters/): install the adapter before configuring
- [Log In](/arena/how-to/deploy/login/): let the adapter write the Omnia config for you
- [Plan and Apply](/arena/how-to/deploy/plan-and-apply/): use your configuration to deploy
- [Multi-Environment Tutorial](/arena/tutorials/deploy/multi-environment/): step-by-step environment setup
