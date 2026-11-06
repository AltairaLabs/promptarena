---
title: 'Deploy: Your First Deployment'
description: Install the AgentCore deploy adapter, add a deploy section to the Tutorial 1 project, preview the plan, deploy it to AWS, check it and tear it down.
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/deploy_adapter_interactive.go
    - arena/cmd/promptarena/deploy_apply_interactive.go
    - arena/cmd/promptarena/deploy_destroy_interactive.go
    - arena/cmd/promptarena/deploy_interactive.go
    - arena/cmd/promptarena/deploy_plan_interactive.go
    - arena/cmd/promptarena/deploy_status_interactive.go
    - arena/deploy/flow/config.go
    - arena/deploy/flow/options.go
    - arena/deploy/flow/render.go
    - arena/deploy/flow/session.go
    - deploy/state.go
    - packc/compiler/compiler.go
    - schemas/v1alpha1/arena.json
---

In this tutorial we deploy the greeting agent from [Tutorial 1](/arena/tutorials/01-first-test/) to AWS Bedrock AgentCore with `promptarena deploy`. We install an adapter, describe the target in the arena config, preview a plan, apply it, check the deployment and destroy it.

## What we will build

One AgentCore runtime running the `my-first-test` agent, plus a local record of the deployment in `.promptarena/deploy.state`.

## Prerequisites

- The `my-first-test` project from [Tutorial 1](/arena/tutorials/01-first-test/).
- Go 1.26 or later and `git`, to build the runtime binary the adapter uploads.
- An AWS account with Bedrock AgentCore enabled, and AWS credentials in your shell (`aws configure`, or the `AWS_*` environment variables).
- An IAM role that AgentCore runtimes can assume. [AgentCore: first deployment](/arena/tutorials/deploy/agentcore/first-deployment/) shows how to create one. Note its ARN.

Steps 1 to 4 run without AWS credentials. Steps 5 to 7 create, inspect and delete real AWS resources.

## Step 1: Install the adapter

An adapter is a separate binary that `promptarena deploy` launches to talk to one cloud. Install the AgentCore adapter:

```bash
promptarena deploy adapter install agentcore
```

The CLI downloads the release binary for your OS and architecture:

```text
Installing adapter "agentcore" v0.3.2 (darwin/arm64)...
  Downloading from: https://github.com/AltairaLabs/promptarena-deploy-agentcore/releases/download/v0.3.2/promptarena-deploy-agentcore_darwin_arm64
  Installed to: /Users/you/.promptarena/adapters/promptarena-deploy-agentcore
```

List the installed adapters:

```bash
promptarena deploy adapter list
```

```text
user-level (/Users/you/.promptarena/adapters):
  promptarena-deploy-agentcore
```

## Step 2: Build the runtime binary

AgentCore runs your agent with a PromptKit runtime binary built for Linux ARM64. Build it from the adapter repository, next to your project:

```bash
cd ..
git clone https://github.com/AltairaLabs/promptarena-deploy-agentcore.git
make -C promptarena-deploy-agentcore build-runtime-arm64
cd my-first-test
```

This writes `../promptarena-deploy-agentcore/promptkit-runtime`.

## Step 3: Add the deploy section

Add a `deploy` block under `spec:` in `config.arena.yaml`, after `defaults`. Replace the role ARN with yours:

```yaml
  deploy:
    provider: agentcore
    config:
      region: us-west-2
      runtime_role_arn: arn:aws:iam::123456789012:role/AgentCoreRuntime
      runtime_binary_path: ../promptarena-deploy-agentcore/promptkit-runtime
```

`provider` names the adapter. Everything under `config` goes to the adapter unchanged; [AgentCore configuration](/arena/reference/deploy/agentcore/configuration/) lists its fields.

## Step 4: Preview the plan

`promptarena deploy` reads `arena.yaml` unless you pass `--config`, so we name our file:

```bash
promptarena deploy plan --config config.arena.yaml
```

The CLI compiles a pack from the config, sends it to the adapter, and prints what the adapter would create:

```text
Deploy plan for environment: default (provider: agentcore)
  Adapter:     /Users/you/.promptarena/adapters/promptarena-deploy-agentcore

Plan: 1 to create, 0 to update, 0 to delete

  + agent_runtime.my_first_test (Create AgentCore runtime for my_first_test)

Plan saved. Run 'promptarena deploy apply' to execute.
```

`+` marks a resource to create. Without `--env`, the environment is `default`. The plan is saved to `.promptarena/deploy.plan`.

## Step 5: Apply the plan

```bash
promptarena deploy apply --config config.arena.yaml
```

Apply reuses the saved plan while the pack and environment are unchanged:

```text
Applying deployment to environment: default (provider: agentcore)
  Using saved plan.
  Adapter:     /Users/you/.promptarena/adapters/promptarena-deploy-agentcore
```

The adapter then prints one line per resource as it creates it, and the command ends with `Apply complete.`. The CLI records the deployment in `.promptarena/deploy.state`.

## Step 6: Check the deployment

```bash
promptarena deploy status --config config.arena.yaml
```

The output starts with `Deployment status for environment: default (provider: agentcore)`, then the adapter's overall status, one `type.name: status` line per resource, and the `Last deployed` time and `Pack checksum` from the state file.

## Step 7: Tear it down

```bash
promptarena deploy destroy --config config.arena.yaml
```

The adapter deletes each resource, the CLI removes `.promptarena/deploy.state`, and the command ends with `Deployment destroyed.`.

## Common issues

### `deploy config "arena.yaml" not found`

You ran a `deploy` command without `--config`. Pass `--config config.arena.yaml`.

### `failed to create AWS client`

`deploy apply` printed `apply failed: adapter error -32603: agentcore: failed to create AWS client` because the shell has no AWS credentials. Run `aws configure`, or export `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and `AWS_REGION`, and apply again.

### `runtime_role_arn is required` or `runtime_binary_path is required`

The adapter validates its `config` before planning. Add the missing field from Step 3.

## Next steps

- [Tutorial: Multi-Environment Deployments](/arena/tutorials/deploy/multi-environment/): give dev, staging and production their own config.
- [Plan and apply](/arena/how-to/deploy/plan-and-apply/): the deploy workflow in day-to-day use.
- [Deploy CLI commands](/arena/reference/deploy/cli-commands/): every deploy subcommand and flag.
