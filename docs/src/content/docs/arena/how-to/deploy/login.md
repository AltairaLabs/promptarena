---
title: 'Deploy: Log In'
description: Authenticate in the browser and let the adapter write the deploy config and store a scoped token.
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/cmd/promptarena/deploy_login_interactive.go
    - arena/deploy/flow/config.go
    - arena/deploy/flow/credentials.go
    - arena/deploy/flow/login.go
    - arena/tui/app/deploypage.go
    - deploy/provider.go
---

## Goal

Authenticate in your browser and let the adapter write the deploy config for you,
instead of exporting a profile and running `deploy config import`.

## Prerequisites

- An adapter installed whose provider supports login (see
  [Install Adapters](/arena/how-to/deploy/install-adapters/)). An adapter that
  supports login advertises the `login` capability.
- A local browser. This flow does not work headless; see [CI](#ci--headless).
- An arena config file (for example from `promptarena init`). Login merges into
  it and does not create the file. If it has no `spec.deploy` section, login
  creates one with `provider` set from `--provider`.
- Any non-secret coordinates the provider needs to start a login, already set
  in the deploy section. For Omnia, that is `api_endpoint`; login fills in the
  rest. A provider that needs nothing up front can log in from a config with
  no deploy section at all.

## Log in

```bash
promptarena deploy login --provider omnia
```

Omit `--provider` if your config already has a `deploy.provider`:

```bash
promptarena deploy login
```

What happens:

1. The CLI starts a callback listener on `127.0.0.1` and opens your browser to
   the provider's authorize page. It also prints the URL.
2. You authenticate. The provider runs its own login (OIDC or otherwise), and the
   CLI never talks to your identity provider.
3. You select the target workspace in the provider's UI.
4. The browser redirects back to the CLI, which writes the config and stores the
   token.

## What gets written

- The arena config (`config.arena.yaml` by default, or the file given with `--config`): the deploy
  profile is merged under `spec.deploy.config` (endpoint, workspace, providers and
  skills). Login keeps the rest of the file, including comments and key order.
  The token is not written here.
- `~/.promptarena/credentials` (mode `0600`): the scoped token, keyed to the
  provider and the absolute path of this config file.

## Then deploy

```bash
promptarena deploy plan
promptarena deploy
```

Deploy commands load the stored token automatically. Token precedence is: an
explicit `api_token` in the config, then the stored credential, then the
provider's environment variable (for Omnia, `OMNIA_API_TOKEN`).

## CI / headless

`deploy login` needs a local browser, so don't run it in CI. Commit the
non-secret deploy config and supply the token through the provider's environment
variable (for Omnia, `OMNIA_API_TOKEN`). See [CI/CD Integration](/arena/how-to/deploy/ci-cd/).

## Troubleshooting

| Symptom | Cause / fix |
|---------|-------------|
| `adapter "<p>" does not support login` | The adapter has no `login` capability. Configure manually (see [Configure Deploy](/arena/how-to/deploy/configure/)) or install a newer adapter. |
| `no provider specified; pass --provider` | The config has no `deploy.provider`. Pass `--provider`. |
| `config file not found: <path>` | Login merges into an existing arena config and never creates one. Create it first (for example with `promptarena init`), then log in again. A config without a `deploy:` section is fine: login adds one, with `provider` set from `--provider`. |
| The browser didn't open | Open the URL the CLI printed. |
| `login timed out waiting for the browser callback` | The CLI waits 5 minutes for the callback. Run login again and finish in the browser. |

## See Also

- [Configure Deploy](/arena/how-to/deploy/configure/): set the deploy section up manually
- [Plan and Apply](/arena/how-to/deploy/plan-and-apply/): deploy once configured
- [Use the TUI: Deploy from the TUI](/arena/how-to/interfaces/use-the-tui/#deploy-from-the-tui): the Deploy wizard's preflight step offers this same login flow with `[l]`
- [CLI Commands](/arena/reference/deploy/cli-commands/): full `deploy login` reference
