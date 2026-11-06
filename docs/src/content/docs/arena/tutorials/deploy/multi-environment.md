---
title: 'Deploy: Multi-Environment Deployments'
description: Give dev and production their own deploy config with environments overrides, preview each one, and deploy each from its own working directory.
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

In this tutorial we extend the deployment from [Your First Deployment](/arena/tutorials/deploy/first-deployment/) with a `dev` and a `production` environment. Each environment overrides part of the base adapter config, and we deploy each one from its own working directory.

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

`--env` picks the environment:

```bash
promptarena deploy plan --config config.arena.yaml --env dev
```

```text
Deploy plan for environment: dev (provider: agentcore)
  Adapter:     /Users/you/.promptarena/adapters/promptarena-deploy-agentcore

Plan: 1 to create, 0 to update, 0 to delete

  + agent_runtime.my_first_test (Create AgentCore runtime for my_first_test)

Plan saved. Run 'promptarena deploy apply' to execute.
```

The saved plan holds the merged config the adapter received. Print it:

```bash
jq -r .request.deploy_config .promptarena/deploy.plan
```

```json
{"region":"us-west-2","runtime_binary_path":"../promptarena-deploy-agentcore/promptkit-runtime","runtime_role_arn":"arn:aws:iam::123456789012:role/AgentCoreRuntime","tags":{"environment":"dev"}}
```

Now preview production and print its config:

```bash
promptarena deploy plan --config config.arena.yaml --env production
jq -r .request.deploy_config .promptarena/deploy.plan
```

```json
{"region":"us-east-1","runtime_binary_path":"../promptarena-deploy-agentcore/promptkit-runtime","runtime_role_arn":"arn:aws:iam::123456789012:role/AgentCoreRuntime","tags":{"environment":"production"}}
```

Production has its own region and tag, and keeps the base role and binary.

## Step 3: Deploy dev

```bash
promptarena deploy --config config.arena.yaml --env dev
```

`deploy` with no subcommand plans and then applies. The CLI records the deployment in `.promptarena/deploy.state`.

:::caution
A working directory holds one deployment record, `.promptarena/deploy.state`, whatever `--env` you pass. Deploying production from this directory would replace dev's record, and a later `status` or `destroy` would act on production's resources. Deploy each environment from its own working directory.
:::

## Step 4: Deploy production from its own directory

Make a copy of the project for production, without its deploy state:

```bash
cd ..
cp -R my-first-test my-first-test-production
rm -rf my-first-test-production/.promptarena
cd my-first-test-production
promptarena deploy plan --config config.arena.yaml --env production
```

```text
Deploy plan for environment: production (provider: agentcore)
  Adapter:     /Users/you/.promptarena/adapters/promptarena-deploy-agentcore

Plan: 1 to create, 0 to update, 0 to delete

  + agent_runtime.my_first_test_production (Create AgentCore runtime for my_first_test_production)

Plan saved. Run 'promptarena deploy apply' to execute.
```

The runtime name comes from the directory name, so the production runtime is `my_first_test_production`. Apply it:

```bash
promptarena deploy apply --config config.arena.yaml --env production
```

## Step 5: Check both environments

Each directory reports its own deployment:

```bash
promptarena deploy status --config config.arena.yaml --env production
cd ../my-first-test
promptarena deploy status --config config.arena.yaml --env dev
```

Each starts with `Deployment status for environment: <env> (provider: agentcore)`, followed by that deployment's resources.

## Step 6: Clean up

Destroy each environment from its own directory:

```bash
promptarena deploy destroy --config config.arena.yaml --env dev
cd ../my-first-test-production
promptarena deploy destroy --config config.arena.yaml --env production
```

Each ends with `Deployment destroyed.`.

## Next steps

- [Configure deploy](/arena/how-to/deploy/configure/): the `deploy` block and environment overrides in day-to-day use.
- [Deploy from CI/CD](/arena/how-to/deploy/ci-cd/): deploy each environment from its own CI job.
- [State management](/arena/explanation/deploy/state-management/): what the state file holds and how the CLI uses it.
