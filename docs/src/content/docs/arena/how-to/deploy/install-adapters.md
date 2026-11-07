---
title: 'Deploy: Install Adapters'
description: Install, list and remove deploy adapter binaries, and control which binary a project uses.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - .github/workflows/sync-adapter-registry.yml
    - arena/cmd/promptarena/adapter_registry.json
    - arena/cmd/promptarena/deploy_adapter_interactive.go
    - arena/deploy/flow/preflight.go
    - deploy/manager.go
---

## Goal

Install, list, and remove deploy adapter binaries.

## Prerequisites

- The `promptarena` CLI installed

## Install an Adapter

Install the Omnia adapter from the built-in registry:

```bash
promptarena deploy adapter install omnia
```

Install a specific version:

```bash
promptarena deploy adapter install omnia@1.4.0
```

The other adapters use the same command:

```bash
promptarena deploy adapter install agentcore
promptarena deploy adapter install vertex
promptarena deploy adapter install foundry
```

What happens:

1. The CLI looks up the adapter in its built-in registry. An unknown name fails with `unknown adapter "<name>"; available: ...`.
2. It resolves the version: the one you pinned with `@version`, otherwise the newest GitHub release. If the GitHub Releases API is unreachable (offline or rate-limited), it prints a warning and falls back to the version in the built-in registry. A daily workflow keeps that fallback version current.
3. It downloads the binary for your OS and architecture from the adapter's GitHub Releases.
4. It writes the binary to `~/.promptarena/adapters/promptarena-deploy-{provider}` (for example `promptarena-deploy-omnia`) with mode `0755`.

### Download URL Format

Adapter binaries are fetched from GitHub Releases:

```text
https://github.com/{repo}/releases/download/v{version}/promptarena-deploy-{provider}_{os}_{arch}
```

For example, on macOS ARM64:

```text
https://github.com/AltairaLabs/PromptArena-deploy-omnia/releases/download/v1.4.0/promptarena-deploy-omnia_darwin_arm64
```

A 404 means the release does not exist or has no binary for your platform. The error says which, and where the version came from.

## List Installed Adapters

See all installed adapters:

```bash
promptarena deploy adapter list
```

Expected output, one block per directory that holds an adapter:

```text
project-local (/path/to/project/.promptarena/adapters):
  promptarena-deploy-omnia
user-level (/home/you/.promptarena/adapters):
  promptarena-deploy-agentcore
```

The list command searches two locations:

1. Project-local: `.promptarena/adapters/` in the current directory
2. User-level: `~/.promptarena/adapters/` in your home directory

It does not list adapters found on `$PATH`.

## Remove an Adapter

Remove an installed adapter:

```bash
promptarena deploy adapter remove omnia
```

This removes the binary from `~/.promptarena/adapters/`. It does not touch project-local copies.

## Adapter Discovery

When you run a deploy command, the CLI uses the first executable it finds:

1. Project-local: `.promptarena/adapters/promptarena-deploy-{provider}`
2. User-level: `~/.promptarena/adapters/promptarena-deploy-{provider}`
3. System `$PATH`: `promptarena-deploy-{provider}`

A project-local binary overrides the user-level one. Commands print the path they use on an `Adapter:` line.

## Manual Installation

To use an adapter you built yourself, place the binary in one of the discovery locations:

```bash
# Build from source
cd promptarena-deploy-myprovider
go build -o promptarena-deploy-myprovider .

# Install to the user-level directory
mkdir -p ~/.promptarena/adapters
cp promptarena-deploy-myprovider ~/.promptarena/adapters/
chmod 755 ~/.promptarena/adapters/promptarena-deploy-myprovider
```

Or install it into the project, to pin a version for everyone working on it:

```bash
mkdir -p .promptarena/adapters
cp promptarena-deploy-myprovider .promptarena/adapters/
chmod 755 .promptarena/adapters/promptarena-deploy-myprovider
```

## Available Adapters

| Adapter | Provider | Description |
|---------|----------|-------------|
| `omnia` | Omnia Kubernetes platform | Deploy to an Omnia workspace (PromptPack, AgentRuntime, ToolRegistry CRDs) |
| `agentcore` | AWS Bedrock AgentCore | Deploy to AWS Bedrock AgentCore |
| `vertex` | Google Agent Runtime | Deploy to Agent Runtime, formerly Vertex AI Agent Engine (one engine per agent) |
| `foundry` | Azure AI Foundry | Deploy Azure AI Foundry hosted agents |

Their lifecycle support, tool execution, A2A and observability differ. See [Target Capabilities](/arena/reference/deploy/target-capabilities/) for a side-by-side comparison.

## Verification

Confirm an adapter is installed and starts:

```bash
# The adapter appears in the list
promptarena deploy adapter list

# The plan prints the adapter path and connects to it
promptarena deploy plan
```

## Troubleshooting

### Adapter not found

`adapter not found for provider "<p>"` means no executable `promptarena-deploy-<p>` exists in any discovery location. Check that the binary exists and is executable:

```bash
ls -la ~/.promptarena/adapters/
```

### Permission denied

Discovery skips a file without the executable bit. Fix its permissions:

```bash
chmod 755 ~/.promptarena/adapters/promptarena-deploy-omnia
```

### Wrong architecture

If you see exec format errors, the binary was built for a different OS or architecture. Reinstall:

```bash
promptarena deploy adapter remove omnia
promptarena deploy adapter install omnia
```

## See Also

- [Configure Deploy](/arena/how-to/deploy/configure/): set up the deploy section after installing an adapter
- [Use the TUI: Deploy from the TUI](/arena/how-to/interfaces/use-the-tui/#deploy-from-the-tui): the Deploy wizard's preflight step detects a missing adapter and prints this install command; adapter management is CLI-only
- [CLI Commands](/arena/reference/deploy/cli-commands/): complete adapter command reference
- [Adapter SDK](/arena/reference/deploy/adapter-sdk/): build your own adapter
