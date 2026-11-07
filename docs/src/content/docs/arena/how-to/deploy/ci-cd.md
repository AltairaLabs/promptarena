---
title: 'Deploy: CI/CD Integration'
description: Run deploy plans on pull requests and deployments on merge with GitHub Actions, and keep deploy state between runs.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - Makefile
    - arena/cmd/promptarena/adapter_registry.json
    - arena/cmd/promptarena/deploy_destroy_interactive.go
    - arena/cmd/promptarena/deploy_interactive.go
    - arena/cmd/promptarena/deploy_plan_interactive.go
    - arena/cmd/promptarena/deploy_refresh_interactive.go
    - arena/cmd/promptarena/deploy_status_interactive.go
    - arena/deploy/flow/config.go
    - arena/deploy/flow/options.go
    - arena/deploy/flow/session.go
    - deploy/state.go
    - packc/main_interactive.go
---

## Goal

Automate deployments with GitHub Actions.

## Prerequisites

- A deploy section in your arena config (see [Configure Deploy](/arena/how-to/deploy/configure/))
- Cloud provider credentials stored as GitHub secrets

`promptarena deploy` compiles the pack from the arena config (`config.arena.yaml` by default), so the workflows need no separate compile step. The [Multi-Environment Pipeline](#multi-environment-pipeline) below pre-compiles with `packc` to deploy the same pack to every stage.

## GitHub Actions Workflow

### Basic Deploy on Push

Deploy to production when code is pushed to the main branch:

```yaml
# .github/workflows/deploy.yml
name: Deploy
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Install PromptArena
        run: go install github.com/AltairaLabs/promptarena/v2/arena/cmd/promptarena@latest

      - name: Install adapter
        run: promptarena deploy adapter install omnia

      - name: Deploy
        env:
          OMNIA_API_TOKEN: ${{ secrets.OMNIA_API_TOKEN }}
        run: promptarena deploy --env production
```

To target AWS Bedrock AgentCore, install the `agentcore` adapter and supply AWS credentials instead:

```yaml
      - name: Install adapter
        run: promptarena deploy adapter install agentcore

      - name: Deploy
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          AWS_REGION: us-west-2
        run: promptarena deploy --env production
```

### Plan on Pull Request

Preview changes on pull requests without applying them:

```yaml
# .github/workflows/deploy-plan.yml
name: Deploy Plan
on:
  pull_request:
    branches: [main]

jobs:
  plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Install PromptArena
        run: go install github.com/AltairaLabs/promptarena/v2/arena/cmd/promptarena@latest

      - name: Install adapter
        run: promptarena deploy adapter install agentcore

      - name: Plan deployment
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          AWS_REGION: us-west-2
        run: promptarena deploy plan --env production
```

### Multi-Environment Pipeline

Deploy to staging first, then to production after approval. This pipeline compiles the pack once with `packc` and deploys the same `app.pack.json` to every environment with `--pack`, so both stages receive identical bytes. Without `--pack`, each deploy recompiles the pack from the config, which gives the same pack content as long as the config is unchanged.

```yaml
# .github/workflows/deploy-pipeline.yml
name: Deploy Pipeline
on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Install packc
        run: go install github.com/AltairaLabs/promptarena/v2/packc@latest

      - name: Compile pack
        run: packc compile --config config.arena.yaml --output app.pack.json --id my-app

      - name: Upload pack
        uses: actions/upload-artifact@v4
        with:
          name: pack
          path: app.pack.json

  deploy-staging:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Install PromptArena
        run: go install github.com/AltairaLabs/promptarena/v2/arena/cmd/promptarena@latest

      - name: Install adapter
        run: promptarena deploy adapter install agentcore

      - name: Download pack
        uses: actions/download-artifact@v4
        with:
          name: pack

      - name: Deploy to staging
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
        run: promptarena deploy --env staging --pack app.pack.json

  deploy-production:
    needs: deploy-staging
    runs-on: ubuntu-latest
    environment: production
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Install PromptArena
        run: go install github.com/AltairaLabs/promptarena/v2/arena/cmd/promptarena@latest

      - name: Install adapter
        run: promptarena deploy adapter install agentcore

      - name: Download pack
        uses: actions/download-artifact@v4
        with:
          name: pack

      - name: Deploy to production
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
        run: promptarena deploy --env production --pack app.pack.json
```

The `environment: production` setting applies GitHub's environment protection rules, so a required reviewer must approve before the production job runs.

## Wrapping the deploy commands

The deploy subcommands take `--env` directly, so you can call them from a shell script or from your project's task runner:

```bash
promptarena deploy plan    --env staging   # preview changes
promptarena deploy         --env staging   # plan + apply
promptarena deploy status  --env staging   # current status
promptarena deploy destroy --env staging   # tear down
```

## State Management in CI

The CLI keeps each environment's deploy state in `.promptarena/envs/<env>/deploy.state` in the working directory. A CI runner starts clean, so persist the file between runs. See [State Management](/arena/explanation/deploy/state-management/) for what the file holds.

### Options for CI State

Option 1: commit state to the repository.

```yaml
- name: Deploy
  run: promptarena deploy --env production

- name: Commit state
  run: |
    git add .promptarena/envs/production/deploy.state
    git commit -m "Update deploy state" || true
    git push
```

Option 2: keep state in artifact storage.

```yaml
- name: Download prior state
  uses: actions/download-artifact@v4
  with:
    name: deploy-state-production
    path: .promptarena/envs/production
  continue-on-error: true

- name: Deploy
  run: promptarena deploy --env production

- name: Upload state
  uses: actions/upload-artifact@v4
  with:
    name: deploy-state-production
    path: .promptarena/envs/production/deploy.state
```

Each environment's state lives in its own directory, so a pipeline that deploys several environments persists each one separately (for example, one artifact per environment, `deploy-state-staging` and `deploy-state-production`).

## Troubleshooting

### Adapter not available in CI

Install the adapter in your CI workflow before deploying:

```yaml
- name: Install adapter
  run: promptarena deploy adapter install omnia   # or: agentcore
```

### Credentials not available

Check that the secrets exist in your repository settings and that the workflow passes them to the deploy step. For Omnia:

```yaml
env:
  OMNIA_API_TOKEN: ${{ secrets.OMNIA_API_TOKEN }}
```

For AWS Bedrock AgentCore:

```yaml
env:
  AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
  AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
```

### State file missing between runs

Without a state file, `deploy` sends no prior state to the adapter, and `status`, `refresh` and `destroy` print `No deployment state found`. Persist the state file with one of the options above.

## See Also

- [Plan and Apply](/arena/how-to/deploy/plan-and-apply/): manual deployment workflows
- [Configure Deploy](/arena/how-to/deploy/configure/): deploy section setup
- [State Management](/arena/explanation/deploy/state-management/): what deploy state holds
