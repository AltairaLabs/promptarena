---
title: 'Deploy: Multi-Environment Deployments'
description: Give dev and production their own deploy config with environments overrides, preview each one, and deploy both from one working directory.
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/deploy_destroy_interactive.go
    - arena/cmd/promptarena/deploy_interactive.go
    - arena/cmd/promptarena/deploy_plan_interactive.go
    - arena/cmd/promptarena/deploy_status_interactive.go
    - arena/deploy/flow/config.go
    - deploy/state.go
    - packc/compiler/compiler.go
---

In this tutorial we extend the deployment from [Your First Deployment](/arena/tutorials/deploy/first-deployment/) with a `dev` and a `production` environment. Each environment overrides part of the base adapter config, and we deploy both from the same working directory.

## What we will build

Two AgentCore deployments of the `my-first-test` agent: `dev` in `us-west-2` and `production` in `us-east-1`, each tagged with its environment name.

## Prerequisites

- Everything from [Your First Deployment](/arena/tutorials/deploy/first-deployment/), with its resources destroyed (Step 7 there).
- `jq`, to read the saved plan.

## Step 1: Add the environments

Replace the `deploy` block in `config.arena.yaml` with this one:

```yaml
  deploy:
    provider: agentcore
    config:
      region: us-west-2
      runtime_role_arn: arn:aws:iam::123456789012:role/AgentCoreRuntime
      runtime_binary_path: ../promptarena-deploy-agentcore/promptkit-runtime
      tags:
        environment: default
    environments:
      dev:
        config:
          tags:
            environment: dev
      production:
        config:
          region: us-east-1
          tags:
            environment: production
```

For each environment, the CLI starts from the base `config` and replaces each top-level key the environment sets. A key the environment sets replaces the whole base value, so `tags` is replaced, not merged.

## Step 2: Preview each environment

`--env` picks the environment. The deploy commands read `config.arena.yaml` by default, so no `--config` is needed:

```bash
promptarena deploy plan --env dev
```

```text
Deploy plan for environment: dev (provider: agentcore)
  Adapter:     /Users/you/.promptarena/adapters/promptarena-deploy-agentcore

Plan: 1 to create, 0 to update, 0 to delete

  + agent_runtime.my_first_test (Create AgentCore runtime for my_first_test)

Plan saved. Run 'promptarena deploy apply --env dev' to execute.
```

Each environment keeps its own saved plan, state and lock under `.promptarena/envs/<env>/`. The saved plan holds the merged config the adapter received. Print it:

```bash
jq -r .request.deploy_config .promptarena/envs/dev/deploy.plan
```

```json
{"region":"us-west-2","runtime_binary_path":"../promptarena-deploy-agentcore/promptkit-runtime","runtime_role_arn":"arn:aws:iam::123456789012:role/AgentCoreRuntime","tags":{"environment":"dev"}}
```

Now preview production and print its config:

```bash
promptarena deploy plan --env production
jq -r .request.deploy_config .promptarena/envs/production/deploy.plan
```

```json
{"region":"us-east-1","runtime_binary_path":"../promptarena-deploy-agentcore/promptkit-runtime","runtime_role_arn":"arn:aws:iam::123456789012:role/AgentCoreRuntime","tags":{"environment":"production"}}
```

Production has its own region and tag, and keeps the base role and binary. Planning production did not touch dev's saved plan.

## Step 3: Deploy dev

Apply the plan we reviewed:

```bash
promptarena deploy apply --env dev
```

The first line after the header is `  Using saved plan.`: the pack has not changed since Step 2, so apply runs exactly the plan we read. The CLI records the deployment in `.promptarena/envs/dev/deploy.state`.

## Step 4: Deploy production

From the same directory:

```bash
promptarena deploy apply --env production
```

Production's deployment is recorded in `.promptarena/envs/production/deploy.state`, next to dev's and separate from it. Both runtimes are named `my_first_test`, from the directory name; they live in different regions.

## Step 5: Check both environments

```bash
promptarena deploy status --env dev
promptarena deploy status --env production
```

Each starts with `Deployment status for environment: <env> (provider: agentcore)`, followed by that environment's resources only.

## Step 6: Clean up

```bash
promptarena deploy destroy --env dev
promptarena deploy destroy --env production
```

Each ends with `Deployment destroyed.`. Destroying dev sends only dev's state to the adapter, so production keeps running until its own destroy.

## Next steps

- [Configure deploy](/arena/how-to/deploy/configure/): the `deploy` block and environment overrides in day-to-day use.
- [Deploy from CI/CD](/arena/how-to/deploy/ci-cd/): deploy each environment from its own CI job.
- [State management](/arena/explanation/deploy/state-management/): what the state file holds and how the CLI uses it.
