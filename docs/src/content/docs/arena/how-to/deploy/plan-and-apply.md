---
title: 'Deploy: Plan and Apply'
description: Preview a deployment, apply it, check its status, refresh and import state, and tear it down.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - arena/cmd/promptarena/deploy_apply_interactive.go
    - arena/cmd/promptarena/deploy_destroy_interactive.go
    - arena/cmd/promptarena/deploy_import_interactive.go
    - arena/cmd/promptarena/deploy_interactive.go
    - arena/cmd/promptarena/deploy_plan_interactive.go
    - arena/cmd/promptarena/deploy_refresh_interactive.go
    - arena/cmd/promptarena/deploy_status_interactive.go
    - arena/deploy/flow/config.go
    - arena/deploy/flow/connect.go
    - arena/deploy/flow/options.go
    - arena/deploy/flow/render.go
    - arena/deploy/flow/session.go
    - arena/tui/app/deploypage.go
    - deploy/client.go
    - deploy/lock.go
    - deploy/provider.go
    - deploy/state.go
---

## Goal

Preview deployment changes, apply them, check status, and tear down resources.

## Prerequisites

- A deploy section in your arena config (see [Configure Deploy](/arena/how-to/deploy/configure/))
- The adapter for that provider installed (see [Install Adapters](/arena/how-to/deploy/install-adapters/))

`deploy`, `deploy plan` and `deploy apply` compile the pack from the arena config, so you do not need a separate compile step. Pass `--pack` to deploy a pre-compiled pack instead. `deploy status` does not compile the pack.

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

Plan saved. Run 'promptarena deploy apply --env production' to execute.
```

`deploy plan` takes the environment's deploy lock, and saves the plan to `.promptarena/envs/<env>/deploy.plan` for a later `deploy apply` to the same environment.

Before planning, the CLI sends the merged deploy config to the adapter's `validate_config`. If the adapter reports it invalid, the command stops with `deploy config is invalid for "<provider>": <reasons>` and nothing is planned. Validation warnings are printed with the plan's own warnings. An adapter that does not implement `validate_config` is planned without it.

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

1. The CLI takes the environment's deploy lock (`.promptarena/envs/<env>/deploy.lock`).
2. It loads the arena config and merges the environment's config into the base config.
3. It compiles the pack from the arena config, or reads the file you passed with `--pack`.
4. It asks the adapter to validate the merged config (`validate_config`).
5. If a prior state exists for this environment, it asks the adapter for current status and stores the refreshed adapter state.
6. It sends a plan request to the adapter and prints the plan.
7. It sends an apply request and prints each resource result the adapter returns.
8. It saves state to `.promptarena/envs/<env>/deploy.state`.

To apply a plan you have already reviewed, run `deploy apply` with the same `--env`. It reuses the environment's saved plan when the pack checksum matches the plan, and otherwise prints `Saved plan is stale (the pack changed since it was planned), re-planning...`. The checksum ignores the pack's compile time, so recompiling an unchanged arena config keeps the saved plan:

```bash
promptarena deploy plan  --env production
promptarena deploy apply --env production   # Using saved plan.
```

### Specifying a Pack File

```bash
promptarena deploy --pack dist/app.pack.json
```

### Using a Different Config File

The CLI reads `config.arena.yaml` by default, the same file `promptarena run` reads and `promptarena init` writes. When there is no `config.arena.yaml` but there is an `arena.yaml`, it reads `arena.yaml`. To point at a different file:

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
  Pack version:  v1.0.0
  Pack checksum: sha256:abc123def456...
```

With no state for the environment, `status` prints `No deployment state found for environment "production". Run 'promptarena deploy --env production' first.` and exits 0. `status` never compiles the pack, so it works even when the arena config does not compile.

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

1. The CLI takes the environment's deploy lock and loads that environment's state. With none, it prints `No deployment state found for environment "staging". Nothing to destroy.` and exits 0.
2. It sends a destroy request with the prior state to the adapter.
3. The adapter removes the resources it manages.
4. The CLI deletes the environment's state file and saved plan.

State is kept per environment, so `deploy destroy --env staging` only ever sends staging's state. Production's deployment is untouched.

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

`status`, `refresh` and `destroy` need a prior deployment to that environment in this project directory. Deploy first:

```bash
promptarena deploy --env production
```

### Error: failed to compile pack

`failed to compile pack from config.arena.yaml: ...` means the arena config does not compile. Fix the reported error, or deploy a pre-compiled pack:

```bash
promptarena deploy --pack path/to/app.pack.json
```

### Error: deploy lock is held by another process

Another `deploy`, `plan`, `apply`, `destroy`, `refresh` or `import` for the same environment is running in this project directory; the message names its pid. Wait for it to finish, then run your command again. The operating system releases the lock when that process exits, so there is no lock file to delete.

### Error: legacy deploy state ... belongs to environment

Earlier releases kept one `.promptarena/deploy.state` for every environment. The CLI uses that file only for the environment it records. Run `promptarena deploy status --env <environment named in the error>` once; it moves the file to `.promptarena/envs/<env>/`, and commands for other environments then work.

### Error: deploy config is invalid

The adapter's `validate_config` rejected the merged deploy config. Fix the reasons it lists in the `deploy:` section (or the environment's overrides) and plan again.

### Adapter errors and logs

The adapter's stderr is shown in the terminal, so its log lines appear alongside the CLI's output. When an adapter exits mid-command, the error ends with the last lines it wrote to stderr (`adapter closed connection unexpectedly; adapter stderr: ...`), which is usually the panic or fatal error.

### Deployment shows no changes

If every resource shows NO_CHANGE, the adapter found nothing to update. Change your prompts or config and plan again.

## See Also

- [Configure Deploy](/arena/how-to/deploy/configure/): set up the deploy section
- [CI/CD Integration](/arena/how-to/deploy/ci-cd/): automate plan and apply
- [Use the TUI: Deploy from the TUI](/arena/how-to/interfaces/use-the-tui/#deploy-from-the-tui): run plan, confirm, apply, and status as a guided wizard (destroy, import, and refresh remain CLI-only)
- [CLI Commands](/arena/reference/deploy/cli-commands/): complete flag reference
- [State Management](/arena/explanation/deploy/state-management/): how state tracking works
