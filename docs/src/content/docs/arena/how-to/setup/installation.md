---
title: Install PromptArena
description: Install the promptarena CLI with npm, Homebrew, go install or a release archive, then create a first project with promptarena init.
verified:
  commit: 02ec7c860b978fcce24022d9ccb868d94208c751
  sources:
    - .github/workflows/release.yml
    - .goreleaser.yml
    - arena/arenaconfig/types.go
    - arena/audio/otobackend/otobackend_interactive.go
    - arena/cmd/promptarena/agent_brief.go
    - arena/cmd/promptarena/audio_backend.go
    - arena/cmd/promptarena/chat_interactive.go
    - arena/cmd/promptarena/completion_interactive.go
    - arena/cmd/promptarena/init.go
    - arena/cmd/promptarena/run.go
    - arena/templates/builtin/quick-start/template.yaml
    - arena/templates/generator.go
    - arena/voice/session_audioio_interactive.go
    - npm/promptarena/lib/resolve.js
    - npm/promptarena/package.json
---
Install the `promptarena` binary and create a first project.

## Install

### Option 1: npm or npx

The npm package ships prebuilt binaries for each platform, so it needs no Go toolchain. It works with npm, yarn and pnpm, and selects the binary for your OS and CPU at install time.

```bash
# Run without installing
npx @altairalabs/promptarena run -c config.arena.yaml

# Install globally
npm install -g @altairalabs/promptarena

# Or add to your project as a dev dependency
npm install --save-dev @altairalabs/promptarena
```

To run it from `package.json`:

```json
{
  "scripts": {
    "test:prompts": "promptarena run -c ./tests/config.arena.yaml"
  },
  "devDependencies": {
    "@altairalabs/promptarena": "^2.4.0"
  }
}
```

### Option 2: Homebrew (macOS and Linux)

The AltairaLabs tap publishes `promptarena` and `packc` as separate casks:

```bash
# Install promptarena
brew install altairalabs/tap/promptarena

# Install packc, if you need it
brew install altairalabs/tap/packc

# Verify the installation
promptarena --version
```

Or tap first and install by name:

```bash
brew tap altairalabs/tap
brew install --cask promptarena
```

### Option 3: go install

This compiles `promptarena` from source with your Go toolchain. It needs no C compiler or audio headers: speaker playback (oto) and the voice console (PortAudio) load their system audio libraries at run time, so a `go install` build, like the release binaries, plays audio. The `promptarena` command links the speaker backend itself; a Go program that imports `arena/engine` without it does not pull in oto.

```bash
# Install with Go (note the arena/cmd/promptarena package path)
go install github.com/AltairaLabs/promptarena/v2/arena/cmd/promptarena@latest

# The binary is in $GOBIN, or $GOPATH/bin when GOBIN is unset
promptarena --version
```

### Option 4: Download a release archive

Download the archive for your platform from the [PromptArena releases](https://github.com/AltairaLabs/promptarena/releases) page. Archives are named `promptarena_<version>_<Os>_<arch>.tar.gz`, where `<Os>` is `Darwin`, `Linux` or `Windows` and `<arch>` is `x86_64` or `arm64`. Each release also attaches `checksums.txt`.

```bash
# Example for Apple silicon (adjust the version and platform)
curl -LO https://github.com/AltairaLabs/promptarena/releases/download/v2.4.0/promptarena_2.4.0_Darwin_arm64.tar.gz
tar -xzf promptarena_2.4.0_Darwin_arm64.tar.gz promptarena
chmod +x promptarena
sudo mv promptarena /usr/local/bin/promptarena
```

### Build from a checkout

```bash
# Clone the repository
git clone https://github.com/AltairaLabs/promptarena.git
cd promptarena

# Build and install to $GOBIN (or $GOPATH/bin)
go install ./arena/cmd/promptarena
```

## Verify the installation

```bash
promptarena --help
```

The output lists the usage and the available commands.

## Enable shell completions (optional)

```bash
# Bash
promptarena completion bash > ~/.local/share/bash-completion/completions/promptarena

# Zsh
promptarena completion zsh > ~/.zsh/completions/_promptarena

# Fish
promptarena completion fish > ~/.config/fish/completions/promptarena.fish
```

See [Configure Shell Completions](/arena/how-to/setup/shell-completions/) for each shell's setup.

## Create your first project

```bash
# Create a project with guided setup
promptarena init my-llm-tests

# Or use quick mode with defaults
promptarena init my-llm-tests --quick --provider mock

# Go to the project
cd my-llm-tests

# Run the test
promptarena run
```

`init` creates:

- the Arena configuration (`config.arena.yaml`)
- a provider (`providers/`)
- a sample scenario (`scenarios/`)
- a prompt configuration (`prompts/`)
- a `.env.example` file listing the API keys to export (skip it with `--no-env`)
- a README with next steps
- the AI coding-agent brief: `AGENTS.md` and the `.claude/skills/promptarena-authoring/` skill, which tell an agent such as Claude Code or Codex how to write PromptArena configs (skip it with `--no-agent`)

`init` keeps files that already exist in the project directory. Pass `--force` to overwrite them.

### Write the config by hand

To create the config yourself, list each prompt, provider and scenario file under `spec`. A `defaults` block is required:

```bash
mkdir my-llm-tests
cd my-llm-tests

cat > config.arena.yaml << 'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: my-llm-tests

spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.prompt.yaml

  providers:
    - file: providers/mock.provider.yaml

  scenarios:
    - file: scenarios/basic-test.scenario.yaml

  defaults:
    temperature: 0.7
    max_tokens: 1500
    output:
      dir: out
      formats: ["json", "markdown"]
EOF
```

## Troubleshooting

### Command not found after go install

```bash
# Add the Go bin directory to your PATH
export PATH=$PATH:$(go env GOPATH)/bin

# Persist it in your shell profile (~/.zshrc or ~/.bashrc)
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.zshrc
source ~/.zshrc
```

### Permission denied after downloading

```bash
# Make the extracted binary executable
chmod +x promptarena
```

### Homebrew not found

Install Homebrew first:

```bash
# macOS/Linux
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

## Related pages

- [Write scenarios](/arena/how-to/scenarios/write-scenarios/)
- [Configure providers](/arena/how-to/providers/configure-providers/)
- [Tutorial: first test](/arena/tutorials/01-first-test/)
