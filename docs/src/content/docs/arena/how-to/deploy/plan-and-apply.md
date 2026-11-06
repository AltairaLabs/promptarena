---
title: 'Deploy: Plan and Apply'
description: Preview a deployment, apply it, check its status, refresh and import state, and tear it down.
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/cmd/promptarena/deploy_apply_interactive.go
    - arena/cmd/promptarena/deploy_destroy_interactive.go
    - arena/cmd/promptarena/deploy_import_interactive.go
    - arena/cmd/promptarena/deploy_interactive.go
    - arena/cmd/promptarena/deploy_plan_interactive.go
    - arena/cmd/promptarena/deploy_refresh_interactive.go
    - arena/cmd/promptarena/deploy_status_interactive.go
    - arena/deploy/flow/config.go
    - arena/deploy/flow/render.go
    - arena/deploy/flow/session.go
    - arena/tui/app/deploypage.go
    - deploy/client.go
    - deploy/provider.go
    - deploy/state.go
---

## Goal

Preview deployment changes, apply them, check status, and tear down resources.

## Prerequisites

- A deploy section in your arena config (see [Configure Deploy](/arena/how-to/deploy/configure/))
- The adapter for that provider installed (see [Install Adapters](/arena/how-to/deploy/install-adapters/))

`deploy`, `deploy plan` and `deploy apply` compile the pack from the arena config, so you do not need a separate compile step. Pass `--pack` to deploy a pre-compiled pack instead.

## Plan a Deployment

Preview the changes without modifying anything:

```bash
promptarena deploy plan
```

Target a specific environment:

```bash
promptarena deploy plan --env production
```

Without `--env`, the environment is `default`.

Example output (the resource names and the summary line come from the adapter):

```text
Deploy plan for environment: production (provider: omnia)
  Adapter:     /home/you/.promptarena/adapters/promptarena-deploy-omnia

Plan: 2 to create, 0 to update, 0 to delete

  + agent_runtime.greeting (Create agent runtime)
  + a2a_endpoint.greeting (Create A2A endpoint)

Plan saved. Run 'promptarena deploy apply' to execute.
```

`deploy plan` saves the plan to `.promptarena/deploy.plan` for a later `deploy apply`.

### Reading the Plan

| Symbol | Action | Description |
|--------|--------|-------------|
| `+` | CREATE | New resource will be created |
| `~` | UPDATE | Existing resource will be modified |
| `-` | DELETE | Resource will be removed |
| `!` | DRIFT | Resource has drifted from expected state |
| ` ` | NO_CHANGE | Resource exists and needs no changes |

## Apply a Deployment

Run plan and apply in one step:

```bash
promptarena deploy
```

Or target an environment:

```bash
promptarena deploy --env staging
```

What happens:

1. The CLI takes the deploy lock (`.promptarena/deploy.lock`).
2. It loads the arena config and merges the environment's config into the base config.
3. It compiles the pack from the arena config, or reads the file you passed with `--pack`.
4. If a prior state exists, it asks the adapter for current status and stores the refreshed adapter state.
5. It sends a plan request to the adapter and prints the plan.
6. It sends an apply request and prints each resource result the adapter returns.
7. It saves state to `.promptarena/deploy.state`.

To apply a plan you have already reviewed, run `deploy apply`. It reuses the saved plan when the pack checksum and the environment match the plan, and re-plans otherwise. A compiled pack records its compile time, so recompiling from the arena config changes the checksum. To apply exactly the plan you reviewed, pass the same pre-compiled pack to both commands:

```bash
packc compile --config arena.yaml --output app.pack.json --id my-app
promptarena deploy plan  --env production --pack app.pack.json
promptarena deploy apply --env production --pack app.pack.json
```

### Specifying a Pack File

```bash
promptarena deploy --pack dist/app.pack.json
```

### Using a Different Config File

The CLI reads `arena.yaml` by default. To point at a different file:

```bash
promptarena deploy --config staging.arena.yaml
```

## Check Deployment Status

Query the current state of deployed resources:

```bash
promptarena deploy status --env production
```

Example output:

```text
Deployment status for environment: production (provider: omnia)
  Adapter:     /home/you/.promptarena/adapters/promptarena-deploy-omnia

  Status: deployed

  agent_runtime.greeting: healthy
  a2a_endpoint.greeting: healthy

  Last deployed: 2026-02-16T10:30:00Z
  Pack checksum: sha256:abc123def456...
```

With no state file, `status` prints `No deployment state found. Run 'promptarena deploy' first.` and exits 0.

### Status Values

| Status | Meaning |
|--------|---------|
| `deployed` | All resources are running |
| `not_deployed` | No resources found |
| `degraded` | Some resources are unhealthy |
| `unknown` | Unable to determine status |

### Resource Health

| Health | Meaning |
|--------|---------|
| `healthy` | Resource is running normally |
| `unhealthy` | Resource exists but has issues |
| `missing` | Resource was expected but not found |

## Destroy a Deployment

Remove all managed resources:

```bash
promptarena deploy destroy --env staging
```

What happens:

1. The CLI takes the deploy lock and loads the prior state. With no state, it prints `No deployment state found. Nothing to destroy.` and exits 0.
2. It sends a destroy request with the prior state to the adapter.
3. The adapter removes the resources it manages.
4. The CLI deletes the local state file.

The state file holds one deployment, whichever environment wrote it last. `deploy destroy --env staging` sends that state to the adapter even if production wrote it. Check the `environment` field in `.promptarena/deploy.state` first to see which deployment the state describes.

## Redeployment Workflow

When you update a prompt and redeploy, the adapter receives the prior state and determines what changed:

```bash
promptarena deploy plan --env production
promptarena deploy --env production
```

The plan shows `~` (UPDATE) for resources that need modification and a blank symbol for resources with no changes.

## Refresh State

Refresh local state to match the live environment. `deploy refresh` asks the adapter for current status, stores the adapter state it returns, and records `last_refreshed`:

```bash
promptarena deploy refresh --env production
```

Example output:

```text
Refreshing state for environment: production (provider: omnia)
  Adapter:     /home/you/.promptarena/adapters/promptarena-deploy-omnia

  Status: degraded

  agent_runtime.greeting: healthy
  a2a_endpoint.greeting: unhealthy — endpoint configuration changed

State refreshed successfully.
```

`deploy` and `deploy plan` refresh state the same way before planning, so a manual refresh is for checking health without planning changes.

## Import Resources

Import a resource that already exists in the target environment into deployment state, so the adapter manages it from then on:

```bash
promptarena deploy import <type> <name> <id>
```

Examples:

```bash
# Import an agent runtime by its container ID
promptarena deploy import agent_runtime my-agent container-abc123

# Import an A2A endpoint
promptarena deploy import a2a_endpoint my-ep endpoint-xyz789

# Import into a specific environment
promptarena deploy import agent_runtime my-agent container-abc123 --env production
```

Example output:

```text
Importing agent_runtime.my-agent (container-abc123) into environment: production
  Adapter:     /home/you/.promptarena/adapters/promptarena-deploy-omnia

  Imported: agent_runtime.my-agent — healthy
```

The resource types and identifiers an import accepts depend on the adapter. After importing, the resource appears in `deploy plan` and `deploy status` output, and will be managed by subsequent `deploy` and `deploy destroy` operations.

## Troubleshooting

### No deployment state found

`status`, `refresh` and `destroy` need a prior deployment in this project directory. Deploy first:

```bash
promptarena deploy --env production
```

### Error: failed to compile pack

`failed to compile pack from arena.yaml: ...` means the arena config does not compile. Fix the reported error, or deploy a pre-compiled pack:

```bash
promptarena deploy --pack path/to/app.pack.json
```

### Error: deploy lock is held by another process

Another `deploy`, `apply`, `destroy`, `refresh` or `import` is running in this project directory. Wait for it to finish, then run your command again. The operating system releases the lock when that process exits.

### Deployment shows no changes

If every resource shows NO_CHANGE, the adapter found nothing to update. Change your prompts or config and plan again.

## See Also

- [Configure Deploy](/arena/how-to/deploy/configure/): set up the deploy section
- [CI/CD Integration](/arena/how-to/deploy/ci-cd/): automate plan and apply
- [Use the TUI: Deploy from the TUI](/arena/how-to/interfaces/use-the-tui/#deploy-from-the-tui): run plan, confirm, apply, and status as a guided wizard (destroy, import, and refresh remain CLI-only)
- [CLI Commands](/arena/reference/deploy/cli-commands/): complete flag reference
- [State Management](/arena/explanation/deploy/state-management/): how state tracking works
