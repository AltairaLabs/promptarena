---
title: Install Skills
description: Install, list and remove shared skills from Git repositories or local paths with promptarena skill, then reference them from a config.
verified:
  commit: 45cbd09a1d70928da45e071755b0c2172abfc8ca
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/skill_sources.go
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/skill.go
    - arena/cmd/promptarena/skill_interactive.go
    - packc/compiler/compiler.go
    - packc/compiler/helpers_integration.go
    - packc/compiler/skills_validator_integration.go
---

Install shared skills from a Git repository or a local directory, list them, and remove them.

## Prerequisites

- The `promptarena` binary.
- `git`, for installs from a repository.

In the commands below, `@acme/pdf-processing` and `@acme/pci-compliance` are placeholders. Replace them with a real `@org/name` reference.

## Install a skill

Install a skill from a Git repository:

```bash
promptarena skill install @acme/pdf-processing
```

Install a specific version (a Git tag or ref):

```bash
promptarena skill install @acme/pdf-processing@v1.0.0
```

Install from a local directory. The path must start with `./`, `../` or `/`:

```bash
promptarena skill install ./path/to/my-skill
```

For an `@org/name` reference, the CLI:

1. Clones `https://github.com/org/name` with `--depth 1`.
2. Fetches and checks out the version, if you gave one.
3. Checks that the directory contains a `SKILL.md` file.
4. Installs it to `~/.config/promptkit/skills/org/name/` (under `$XDG_CONFIG_HOME` when that is set).

A local directory installs to `~/.config/promptkit/skills/local/<dirname>/` and is listed as `@local/<dirname>`. Local installs skip the `SKILL.md` check.

## Install to the project

Add `--project` to install into the project directory instead of the user directory:

```bash
promptarena skill install @acme/pci-compliance --project
```

The skill installs to `.promptkit/skills/acme/pci-compliance/`, relative to the current directory. For an `@org/name` reference, a project install takes priority over a user install.

## Install into a specific directory

Add `--into` to install a skill into a directory of your choice, for example next to a workflow stage's configuration:

```bash
promptarena skill install @acme/pci-compliance --into ./skills/billing
```

The skill installs to `./skills/billing/pci-compliance/`. You cannot combine `--into` with `--project`.

## List installed skills

```bash
promptarena skill list
```

The output groups skills by location and shows each one's absolute path:

```text
Project:
  @acme/pci-compliance  (/work/proj/.promptkit/skills/acme/pci-compliance)

User:
  @acme/pdf-processing  (/home/you/.config/promptkit/skills/acme/pdf-processing)
```

With nothing installed, it prints `No skills installed.` and a hint.

## Remove a skill

```bash
promptarena skill remove @acme/pdf-processing
```

This deletes one installed copy per call: the project copy first, then the user copy. When a user copy remains, the command prints `Another copy of <org>/<name> is still installed at <path>; run the command again to remove it too.` Remove a local install with its `@local/<dirname>` name.

## Reference a skill from a config

In an Arena config (`spec.skills`) and in packs that `packc` compiles, a skill entry's `path` is either a directory relative to the config file or an `@org/name` reference to an installed skill:

```json
{
  "skills": [
    {"path": "@acme/pci-compliance"},
    {"path": "skills/billing/pci-compliance"},
    {"path": "skills/local-skill", "preload": true}
  ]
}
```

An `@org/name` reference resolves to the first installed copy in `.promptkit/skills/` beside the config file, then `.promptkit/skills/` in the current directory, then the user directory. A reference that is not installed fails with `skills: skill <org>/<name> is not installed (install it with: promptarena skill install @<org>/<name>)`. `packc` keeps the `@org/name` reference in the compiled pack, so the pack's consumer resolves it against its own installed skills; the PromptKit SDK's skills registry does that, see the [Skills Reference](https://promptkit.altairalabs.ai/reference/skills).

A directory path must stay inside the config directory. Install a skill there with `--into` to vendor it with the config.

## Skill discovery order

The runtime reads skill sources in the order the `skills` array lists them, whether each one is a directory or inline. The first source to register a skill name wins. A later skill with the same name is dropped with a warning. If that later entry sets `preload`, the registered skill becomes preloaded.

## Related pages

- [Skills Reference](https://promptkit.altairalabs.ai/reference/skills): the SKILL.md format, pack configuration and the SDK API.
- [CLI commands](/arena/reference/cli-commands/#promptarena-skill): the full `skill` command reference.
