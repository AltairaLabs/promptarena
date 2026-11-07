# @altairalabs/packc

> PackC, the PromptPack compiler: compile a PromptArena config into a `.pack.json` PromptPack, and validate and inspect packs.

## Installation

### npx (no installation)

```bash
npx @altairalabs/packc compile -c config.arena.yaml
npx @altairalabs/packc validate my-pack.pack.json
```

### Global installation

```bash
npm install -g @altairalabs/packc

packc version
packc compile -c config.arena.yaml
```

### Project dev dependency

```bash
npm install --save-dev @altairalabs/packc
```

Then add scripts to `package.json`:

```json
{
  "scripts": {
    "build:prompts": "packc compile -c config.arena.yaml -o dist/prompts.pack.json",
    "validate:prompts": "packc validate dist/prompts.pack.json"
  }
}
```

## What is PackC?

PackC compiles the prompt configs, tools, evals, workflow, agents and skills that a PromptArena config references into a single [PromptPack](https://promptpack.org) JSON file, which the PromptKit runtime and SDK load. It also validates pack files against the PromptPack schema and prints a summary of a pack.

## Quick start

1. Point PackC at an Arena config that lists your prompts:

```yaml
# config.arena.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: my-app
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml
  providers: []
  defaults: {}
```

2. Compile it:

```bash
packc compile -c config.arena.yaml -o my-app.pack.json --pack-version 1.0.0
```

3. Validate and inspect the pack:

```bash
packc validate my-app.pack.json
packc inspect my-app.pack.json
```

## Commands

| Command | What it does |
|---------|--------------|
| `packc compile [-c <config>] [-o <file>] [--id <id>] [--pack-version <semver>]` | Compile every prompt in an Arena config into one pack. `-c` defaults to `config.arena.yaml`, `--id` to the current folder name, `-o` to `<id>.pack.json`, and `--pack-version` to `v1.0.0`. |
| `packc compile-prompt -p <prompt.yaml> -o <file> [--pack-version <semver>]` | Compile one prompt config into a one-prompt pack. |
| `packc validate <pack-file>...` | Validate one or more pack files. Exits 1 if any has an error. |
| `packc inspect <pack-file>` | Print a pack's prompts, tools, evals, skills, workflow, agents and metadata. |
| `packc version` | Print the version. |
| `packc completion <shell>` | Print a completion script for bash, zsh, fish or powershell. |

Full reference: [PackC docs](https://promptarena.altairalabs.ai/packc/).

## CI validation

```yaml
# .github/workflows/prompts.yml
- name: Build and validate prompts
  run: |
    npx @altairalabs/packc compile -c config.arena.yaml -o dist/prompts.pack.json
    npx @altairalabs/packc validate dist/prompts.pack.json
```

## How it works

The `packc` Go binary ships in a per-platform package (`@altairalabs/packc-linux-x64` and friends) that this package declares as an optional dependency. Your package manager installs the one that matches your OS and CPU, and a thin Node.js wrapper runs it. No install script runs and no Go toolchain is needed.

## Supported platforms

- macOS (Intel and Apple Silicon)
- Linux (x86_64 and arm64)
- Windows (x86_64 and arm64)

## Troubleshooting

### Binary not found

`packc` ships its Go binary in a per-platform package
(`@altairalabs/packc-linux-x64` and friends) that this package declares as an
**optionalDependency**. npm, pnpm, yarn and bun each select the one matching
your `os`/`cpu`. No install script runs, so nothing has to be allowlisted.

If you see `could not find its platform binary package`:

1. Make sure the install did not run with `--omit=optional` / `--no-optional`.
2. Delete `node_modules` and the lockfile and reinstall — a lockfile generated
   on a different platform will not carry your binary.
3. Confirm your platform is one of darwin/linux/win32 on x64 or arm64.

## Other installation methods

- **Homebrew**: `brew install altairalabs/tap/packc`
- **Go install**: `go install github.com/AltairaLabs/promptarena/v2/packc@latest`
- **Direct download**: [GitHub Releases](https://github.com/AltairaLabs/promptarena/releases)
- **Build from source**: clone the repo and run `go build -o bin/packc ./packc`

## Related tools

- **[@altairalabs/promptarena](https://www.npmjs.com/package/@altairalabs/promptarena)**: test and evaluate your prompts

## License

Apache-2.0. See [LICENSE](https://github.com/AltairaLabs/promptarena/blob/main/LICENSE).

## Support

- [GitHub Issues](https://github.com/AltairaLabs/promptarena/issues)
