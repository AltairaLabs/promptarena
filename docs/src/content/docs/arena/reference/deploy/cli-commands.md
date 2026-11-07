---
title: 'Deploy: CLI Commands'
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - .github/workflows/sync-adapter-registry.yml
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/adapter_registry.json
    - arena/cmd/promptarena/deploy_adapter_interactive.go
    - arena/cmd/promptarena/deploy_apply_interactive.go
    - arena/cmd/promptarena/deploy_config_interactive.go
    - arena/cmd/promptarena/deploy_destroy_interactive.go
    - arena/cmd/promptarena/deploy_import_interactive.go
    - arena/cmd/promptarena/deploy_interactive.go
    - arena/cmd/promptarena/deploy_login_interactive.go
    - arena/cmd/promptarena/deploy_plan_interactive.go
    - arena/cmd/promptarena/deploy_refresh_interactive.go
    - arena/cmd/promptarena/deploy_status_interactive.go
    - arena/deploy/flow/config.go
    - arena/deploy/flow/connect.go
    - arena/deploy/flow/credentials.go
    - arena/deploy/flow/login.go
    - arena/deploy/flow/options.go
    - arena/deploy/flow/render.go
    - arena/deploy/flow/session.go
    - deploy/client.go
    - deploy/lock.go
    - deploy/manager.go
    - deploy/provider.go
    - deploy/state.go
---

`promptarena deploy` manages prompt pack deployments to cloud providers through adapter plugins. It plans, applies, monitors and destroys deployments across environments.

```bash
promptarena deploy [subcommand] [flags]
```

## Global flags

These flags apply to `deploy` and every subcommand. `--pack` is accepted by the `deploy` subcommands that run in a project (`plan`, `apply`, `status`, `destroy`, `refresh`, `import`, `login`).

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--env` | `-e` | string | `""` (resolves to `default`) | Target environment name: letters, digits, `.`, `_`, `-`, starting with a letter or digit |
| `--config` | | string | `config.arena.yaml`, or `arena.yaml` when only that exists | Path to config file |
| `--pack` | | string | compiled from `--config` | Pre-compiled `.pack.json` to deploy |

Without `--pack`, `plan`, `apply` and bare `deploy` compile the pack from `--config`. `status`, `destroy`, `refresh` and `import` do not read or compile the pack.

State, saved plans and the deploy lock are per environment, under `.promptarena/envs/<env>/`. A command reads and writes only the state of the environment it targets.

An adapter's stderr is passed through to the terminal. When an adapter exits mid-command, the error includes the last lines it wrote to stderr.

## Commands

### deploy

Runs plan and apply in sequence. It always plans afresh and ignores any saved plan; it does not invoke `deploy apply`.

```bash
promptarena deploy [flags]
```

Takes the global flags only.

**Behaviour:**

1. Acquire the environment's deploy lock.
2. Load the `deploy` section of the config.
3. Resolve the pack: compile from `--config`, or read `--pack`.
4. Merge the base config with the environment overrides.
5. Call adapter `ValidateConfig`; an invalid config stops here. An adapter without `validate_config` is skipped.
6. Load the environment's prior state, if any.
7. Refresh state from the live environment.
8. Call adapter `Plan`.
9. Call adapter `Apply`.
10. Save state with the adapter state, pack version, pack checksum and timestamps.

**Examples:**

```bash
promptarena deploy
promptarena deploy --env production
promptarena deploy --pack dist/app.pack.json
promptarena deploy --config staging.arena.yaml --env staging
```

---

### deploy plan

Previews deployment changes without modifying any cloud resource. Saves the plan to `.promptarena/envs/<env>/deploy.plan` for `deploy apply`. It takes the environment's deploy lock, validates the config with the adapter (as `deploy` does), and may refresh and rewrite local state when prior state exists.

```bash
promptarena deploy plan [flags]
```

Takes the global flags only.

**Output:**

The first line is `Deploy plan for environment: <env> (provider: <provider>)`, followed by an `  Adapter:     <path>` line when the adapter binary is installed. Then a `Plan:` line with the adapter's summary, and one row per change: `  <symbol> <type>.<name> (<detail>)`. With no changes it prints `  No changes required.`

```
Deploy plan for environment: production (provider: agentcore)
  Adapter:     /home/you/.promptarena/adapters/promptarena-deploy-agentcore

Plan: 1 to create, 1 to update, 1 to delete

  + agent_runtime.greeting (Create agent runtime)
  ~ a2a_endpoint.greeting (Update A2A endpoint)
  - old_resource.legacy (Delete legacy resource)

Plan saved. Run 'promptarena deploy apply --env production' to execute.
```

**Change symbols:**

| Symbol | Action |
|--------|--------|
| `+` | CREATE |
| `~` | UPDATE |
| `-` | DELETE |
| `!` | DRIFT |
| ` ` | NO_CHANGE |

**Examples:**

```bash
promptarena deploy plan
promptarena deploy plan --env production
```

---

### deploy apply

Applies deployment changes under the environment's deploy lock. If the environment's saved plan from `deploy plan` matches the current pack checksum, `apply` uses it (`Using saved plan.`). Otherwise it validates the config and plans first. The checksum ignores the pack's compile time, so a plan stays fresh across recompiles of an unchanged config.

```bash
promptarena deploy apply [flags]
```

Takes the global flags only.

**Examples:**

```bash
promptarena deploy apply --env staging
```

---

### deploy status

Shows the current deployment status and resource health.

```bash
promptarena deploy status [flags]
```

Takes the global flags only.

**Output:**

```
Deployment status for environment: production (provider: agentcore)
  Adapter:     /home/you/.promptarena/adapters/promptarena-deploy-agentcore

  Status: deployed

  agent_runtime.greeting: healthy
  a2a_endpoint.greeting: healthy — serving at https://example.com/a2a

  Last deployed: 2026-02-16T10:30:00Z
  Pack version:  v1.0.0
  Pack checksum: sha256:abc123def456...
```

The `Pack version:` line appears when the state records one. Resource rows are `  <type>.<name>: <status>[ — <detail>]`. The `Adapter:` line appears when the adapter binary is installed.

**Status values:**

| Status | Meaning |
|--------|---------|
| `deployed` | All resources running |
| `not_deployed` | No resources found |
| `degraded` | Some resources unhealthy |
| `unknown` | Unable to determine |

**Resource health:**

| Health | Meaning |
|--------|---------|
| `healthy` | Running normally |
| `unhealthy` | Exists but has issues |
| `missing` | Expected but not found |

With no state for the environment, `status` prints `  No deployment state found for environment "<env>". Run 'promptarena deploy --env <env>' first.` and exits 0. It does not need the adapter installed in that case. `status` does not take the lock and never compiles the pack, so it works when the arena config does not compile.

**Examples:**

```bash
promptarena deploy status
promptarena deploy status --env production
```

---

### deploy destroy

Tears down all managed resources.

```bash
promptarena deploy destroy [flags]
```

Takes the global flags only.

**Behaviour:**

1. Acquire the environment's deploy lock.
2. Load the environment's state. With none it prints `  No deployment state found for environment "<env>". Nothing to destroy.` and exits 0.
3. Call adapter `Destroy` with that state.
4. Delete the environment's state file and saved plan.
5. Print `Deployment destroyed.`

**Examples:**

```bash
promptarena deploy destroy --env staging
```

---

### deploy refresh

Refreshes local state from the live environment. The adapter reports the current state of all resources and local state is updated to match. Use it to detect drift between local state and cloud resources.

```bash
promptarena deploy refresh [flags]
```

Takes the global flags only.

**Behaviour:**

1. Acquire the environment's deploy lock.
2. Load the environment's state. With none it prints `  No deployment state found for environment "<env>". Run 'promptarena deploy --env <env>' first.` and exits 0.
3. Call adapter `Status` with the prior state.
4. Update the adapter state and the `last_refreshed` timestamp.
5. Save the updated state.
6. Print each resource as `<type>.<name>: <status>`, then `State refreshed successfully.`

**Examples:**

```bash
promptarena deploy refresh
promptarena deploy refresh --env production
```

---

### deploy import

Imports a pre-existing resource into deployment state, so PromptKit manages resources created outside the deploy workflow.

```bash
promptarena deploy import <type> <name> <id> [flags]
```

Takes the global flags.

**Arguments:**

| Argument | Description |
|----------|-------------|
| `type` | Resource type (e.g., `agent_runtime`, `a2a_endpoint`) |
| `name` | Resource name to assign in local state |
| `id` | Provider-specific resource identifier |

**Behaviour:**

1. Acquire the environment's deploy lock.
2. Load the environment's state, or create new state if none exists.
3. Call adapter `Import` with the resource type, name and identifier.
4. Update or create local state with the adapter response.
5. Save the state with a `last_refreshed` timestamp.
6. Print the imported resource.

**Examples:**

```bash
promptarena deploy import agent_runtime my-agent container-abc123
promptarena deploy import a2a_endpoint my-ep endpoint-xyz789
promptarena deploy import agent_runtime my-agent container-abc123 --env production
```

---

### deploy login

Authenticates in the browser and writes the deploy config. Available only for adapters that advertise the `login` capability.

```bash
promptarena deploy login [--provider <name>]
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--provider` | string | `deploy.provider` from the config | Adapter to log in with |

**Behaviour:**

1. Resolve the provider (`--provider` or the config's `deploy.provider`).
2. Load the config file. It must exist (`config file not found: <path>` otherwise); a `deploy:` section is optional, and login creates one with `provider` set when it is missing.
3. Connect the adapter and verify it advertises the `login` capability.
4. Start a loopback callback server on `127.0.0.1` and open the browser to the adapter's authorize URL (with a CSRF `state`).
5. The provider authenticates the user (it brokers OIDC; the CLI never sees the identity provider) and redirects back to the loopback with a one-time code.
6. Exchange the code via the adapter for a deploy profile and a scoped token.
7. Merge the profile into the arena config and store the token in `~/.promptarena/credentials`, never in the config file.

At deploy time the stored token is loaded when the config carries no `api_token`. Precedence: explicit `api_token`, provider env var, credentials store.

The command needs a local browser. In CI, supply the token through the provider's environment variable.

**Examples:**

```bash
promptarena deploy login
promptarena deploy login --provider omnia
```

---

### deploy config import

Merges an exported deploy profile into the deploy config and validates it. The profile is a JSON or YAML mapping, typically exported by your deploy provider (for example Omnia), containing connection details, a scoped token, and discovered providers and skills. Only `deploy.config` is touched; comments, key order and other sections of the config file are preserved.

```bash
promptarena deploy config import <profile-file|-> [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--provider` | string | `omnia` | Provider to set when the deploy section is created |
| `--skip-validate` | bool | `false` | Skip validating the merged config against the adapter |

**Behaviour:**

1. Read the profile from the file, or from stdin when the argument is `-`.
2. Deep-merge it under `spec.deploy.config`, creating the deploy section (with `provider`) if absent. Existing keys are overridden and nested mappings merged.
3. Write the updated config back. The config file must already exist, and its permission bits are preserved.
4. Unless `--skip-validate` is set, validate the merged config through the adapter's `validate_config` RPC.

**Examples:**

```bash
promptarena deploy config import omnia-profile.json
promptarena deploy config import - < omnia-profile.yaml
promptarena deploy config import profile.json --provider omnia
promptarena deploy config import profile.json --skip-validate
```

---

### deploy adapter install

Installs an adapter binary from the registry.

```bash
promptarena deploy adapter install <provider>[@<version>]
```

**Behaviour:**

1. Look up the provider in the built-in adapter registry, which supplies its GitHub repo.
2. Determine the version: the one you pinned, or the newest release from the GitHub Releases API.
3. Download the binary from GitHub Releases for the current OS and architecture.
4. Install to `~/.promptarena/adapters/promptarena-deploy-{provider}`.
5. Set executable permissions (0755).

The version is resolved live, so a newly released adapter is installable immediately.

If the GitHub API cannot be reached, the install falls back to the version recorded in the built-in registry and prints a warning to stderr:

```
warning: could not resolve latest agentcore from GitHub (...);
falling back to registry default v0.3.1
```

The recorded version is refreshed daily and affects only this fallback.

Download URL format:

```
https://github.com/{repo}/releases/download/v{version}/promptarena-deploy-{provider}_{os}_{arch}
```

**Examples:**

```bash
promptarena deploy adapter install agentcore
promptarena deploy adapter install agentcore@0.2.0
```

---

### deploy adapter list

Lists installed adapters.

```bash
promptarena deploy adapter list
```

**Output:**

Each search directory that contains adapters is printed as a `label (path):` header followed by the binary filenames found in it:

```
project-local (/path/to/project/.promptarena/adapters):
  promptarena-deploy-custom
user-level (/home/you/.promptarena/adapters):
  promptarena-deploy-agentcore
```

With no adapters installed it prints `No adapters installed.` and a hint to run `promptarena deploy adapter install <provider>`.

Search locations, in order:

1. Project-local: `.promptarena/adapters/`
2. User-level: `~/.promptarena/adapters/`

---

### deploy adapter remove

Removes an installed adapter binary from `~/.promptarena/adapters/`.

```bash
promptarena deploy adapter remove <provider>
```

**Examples:**

```bash
promptarena deploy adapter remove agentcore
```

## Configuration

The deploy section of the arena config:

```yaml
deploy:
  provider: agentcore          # Required: adapter name
  config:                       # Optional: base provider config
    region: us-west-2
  environments:                 # Optional: per-environment overrides
    production:
      config:
        region: us-east-1
```

## File paths

| Path | Description |
|------|-------------|
| `config.arena.yaml` | Default config file (`arena.yaml` when only that exists) |
| `*.pack.json` | Optional pre-compiled pack file (passed via `--pack`) |
| `.promptarena/envs/<env>/deploy.state` | The environment's deployment state (JSON) |
| `.promptarena/envs/<env>/deploy.plan` | The environment's saved plan |
| `.promptarena/envs/<env>/deploy.lock` | The environment's lock file (prevents concurrent access; kept after use) |
| `.promptarena/adapters/` | Project-local adapters |
| `~/.promptarena/adapters/` | User-level adapters |

## See Also

- [Configure Deploy](/arena/how-to/deploy/configure/): configuration guide
- [Plan and Apply](/arena/how-to/deploy/plan-and-apply/): deployment workflows
- [Protocol](/arena/reference/deploy/protocol/): JSON-RPC method details
