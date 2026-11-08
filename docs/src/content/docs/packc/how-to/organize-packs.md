---
title: Organize Pack Files
description: Lay out arena configs and compiled packs by environment, feature or version, name pack IDs that packc accepts, and decide which packs to commit.
sidebar:
  order: 4
verified:
  commit: 66ec692303a313b1f7ece7ce55dd5f73859efe29
  sources:
    - packc/compiler/compiler.go
    - packc/main.go
    - packc/main_interactive.go
---
Set up a directory layout for your configs and compiled packs that a build script can follow.

## Prerequisites

- `packc` installed ([Install PackC](/packc/how-to/install/)).
- More than one pack to build: several environments, features or released versions.

## Steps

### 1. Choose a layout

`packc compile` writes one pack per arena config, so the layout of `packs/` follows the layout of your configs. `packc` creates any missing output directory. Pick the axis your deployments vary along.

#### By environment

Use this when dev, staging and prod use different configs:

```text
config/
├── arena.dev.yaml
├── arena.staging.yaml
└── arena.prod.yaml
packs/
├── dev/app.pack.json
├── staging/app.pack.json
└── prod/app.pack.json
```

```bash
for env in dev staging prod; do
  packc compile --config "config/arena.$env.yaml" \
    --output "packs/$env/app.pack.json" --id "app-$env"
done
```

#### By feature

Use this when each feature ships its own pack:

```text
config/
├── customer-support/config.arena.yaml
└── sales/config.arena.yaml
packs/
├── customer-support.pack.json
└── sales.pack.json
```

```bash
for feature in customer-support sales; do
  packc compile --config "config/$feature/config.arena.yaml" \
    --output "packs/$feature.pack.json" --id "$feature"
done
```

#### By version

Use this when you keep released packs side by side:

```text
packs/
├── v1.1.0/app.pack.json
├── v1.2.0/app.pack.json
└── latest -> v1.2.0
```

```bash
VERSION=1.2.0
packc compile --config config.arena.yaml \
  --output "packs/v$VERSION/app.pack.json" --id app
ln -sfn "v$VERSION" packs/latest
```

Combine axes when you need to, for example `packs/prod/customer-support.pack.json`.

### 2. Name pack IDs

A pack ID must start with a lowercase letter and contain only lowercase letters, digits and hyphens. `packc compile` rejects any other `--id` before compiling.

```bash
--id customer-support        # valid
--id customer-support-prod   # valid
--id customer-support-v1     # valid
--id customer-support-1.2.0  # rejected: dots are not allowed
--id 2024-demo               # rejected: must start with a letter
```

Keep semantic versions out of the ID. Put the release version in the pack's `version` field with `--pack-version 1.2.0` (without it, `packc compile` writes `v1.0.0`), and in the output path, as in the version layout above.

For file names, use the ID with a `.pack.json` suffix. `--output` defaults to `<id>.pack.json`, and the default keeps names and IDs in step.

### 3. Script the build

Put the loop in one script or Makefile target, so developers and CI build the same way:

```bash
#!/bin/bash
# scripts/build-packs.sh
set -e

for env in dev staging prod; do
  packc compile --config "config/arena.$env.yaml" \
    --output "packs/$env/app.pack.json" --id "app-$env"
  packc validate "packs/$env/app.pack.json"
done
```

Validate inside the loop so a failure stops the build at the pack that caused it. `packc validate` also takes several files, as in [Verify](#verify).

### 4. Decide what to commit

Compiled packs are build output. Rebuild them in CI and keep them out of version control:

```gitignore
packs/
```

To commit only the production packs, ignore the rest:

```gitignore
packs/dev/
packs/staging/
```

If you commit packs, rebuild them in CI and fail when the result differs from the committed file. Two builds of the same inputs with the same `packc` version differ only in `compilation.created_at`, so compare without it:

```bash
packc compile --config config/arena.prod.yaml --output /tmp/app.pack.json --id app-prod
diff <(jq 'del(.compilation.created_at)' /tmp/app.pack.json) \
     <(jq 'del(.compilation.created_at)' packs/prod/app.pack.json)
```

### 5. Remove old versions

With the version layout, delete all but the newest few directories. This keeps the three most recent:

```bash
cd packs
ls -d v*/ | sort -V | head -n -3 | xargs rm -rf
```

`head -n -3` needs GNU coreutils. On macOS, install `coreutils` and use `ghead`.

## Verify

```bash
find packs -name '*.pack.json' -print0 | xargs -0 packc validate
```

`packc validate` checks every file it is given. Each valid pack prints `✓ Pack structure is valid`, a final line counts the valid packs, and the command exits 1 if any pack fails.

## Related

- [Compile packs](/packc/how-to/compile-packs/)
- [Validate packs](/packc/how-to/validate-packs/)
- [CI/CD integration](/packc/how-to/ci-cd-integration/)
- [Pack management tutorial](/packc/tutorials/04-pack-management/)
- [Pack format](/packc/explanation/pack-format/)
