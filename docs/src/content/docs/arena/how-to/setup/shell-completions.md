---
title: Configure Shell Completions
description: Enable tab completion for promptarena and packc in Bash, Zsh, Fish and PowerShell, including config-driven values for scenarios and providers.
verified:
  commit: 95b220864e8df93d5a36b38ad288897bf70a4c5b
  sources:
    - .goreleaser.yml
    - arena/cmd/promptarena/completion_interactive.go
    - arena/cmd/promptarena/config_inspect_interactive.go
    - arena/cmd/promptarena/run.go
    - arena/templates/loader.go
    - packc/main_interactive.go
---
Enable tab completion for `promptarena` and `packc` commands in your shell.

## What completes

Both binaries complete commands, subcommands and flag names. `promptarena` also completes these flag values:

| Flag | Values |
|---|---|
| `run --scenario` | scenarios in your `config.arena.yaml` |
| `run --provider` | provider IDs in your `config.arena.yaml` |
| `run --region` | `us`, `uk`, `au`, `eu`, `asia`, `latam` |
| `run --format`, `run --formats` | `json`, `junit`, `markdown`, `csv`, `objectives` |
| `config-inspect --section` | config sections |
| `init --provider` | `mock`, `openai`, `claude`, `gemini` |
| `init --template` | built-in templates, then templates from configured template repos |

`packc` completes from fixed lists only.

## Bash

### Install per user

```bash
# Create the completions directory if needed
mkdir -p ~/.local/share/bash-completion/completions

# Generate the completion scripts
promptarena completion bash > ~/.local/share/bash-completion/completions/promptarena
packc completion bash > ~/.local/share/bash-completion/completions/packc

# Reload your shell, or source the files
source ~/.local/share/bash-completion/completions/promptarena
source ~/.local/share/bash-completion/completions/packc
```

### Install system-wide

The shell opens a `>` redirect before `sudo` runs, so write the file through `sudo tee`:

```bash
promptarena completion bash | sudo tee /etc/bash_completion.d/promptarena > /dev/null
packc completion bash | sudo tee /etc/bash_completion.d/packc > /dev/null
```

### Load from .bashrc

Add to your `~/.bashrc`:

```bash
# PromptArena completions
if command -v promptarena &> /dev/null; then
    source <(promptarena completion bash)
fi
if command -v packc &> /dev/null; then
    source <(packc completion bash)
fi
```

## Zsh

### Generate and install

```bash
# Create the completions directory
mkdir -p ~/.zsh/completions

# Generate the completion scripts
promptarena completion zsh > ~/.zsh/completions/_promptarena
packc completion zsh > ~/.zsh/completions/_packc
```

### Add to .zshrc

Add to your `~/.zshrc`, before `compinit`:

```zsh
# Add the custom completions directory to fpath
fpath=(~/.zsh/completions $fpath)

# Initialize completions
autoload -Uz compinit && compinit
```

### Alternative: Oh My Zsh

Create a plugin directory for each binary and write the completion script into it:

```bash
mkdir -p ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/promptarena ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/packc
promptarena completion zsh > ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/promptarena/_promptarena
packc completion zsh > ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/packc/_packc
```

Then add both plugins to `plugins=(...)` in your `~/.zshrc`, for example `plugins=(git promptarena packc)`.

## Fish

```bash
# Generate the completion scripts
promptarena completion fish > ~/.config/fish/completions/promptarena.fish
packc completion fish > ~/.config/fish/completions/packc.fish
```

Fish loads completions from this directory automatically.

## PowerShell

### Generate and load

```powershell
# Generate the completion scripts
promptarena completion powershell > promptarena.ps1
packc completion powershell > packc.ps1

# Load completions in the current session
. ./promptarena.ps1
. ./packc.ps1
```

### Add to your profile

Add to your PowerShell profile (`$PROFILE`):

```powershell
# PromptArena completions
promptarena completion powershell | Out-String | Invoke-Expression
packc completion powershell | Out-String | Invoke-Expression
```

## Docker

For an Alpine-based image that already contains both binaries, add to your Dockerfile:

```dockerfile
# Install bash-completion
RUN apk add --no-cache bash-completion

# Generate and install completions
RUN mkdir -p /etc/bash_completion.d && \
    promptarena completion bash > /etc/bash_completion.d/promptarena && \
    packc completion bash > /etc/bash_completion.d/packc

# Source completions in bashrc
RUN echo 'source /etc/profile.d/bash_completion.sh' >> /root/.bashrc && \
    echo 'source /etc/bash_completion.d/promptarena' >> /root/.bashrc && \
    echo 'source /etc/bash_completion.d/packc' >> /root/.bashrc
```

## Verify

Press Tab to complete:

```bash
# Complete commands
promptarena ru<Tab>       # → promptarena run

# Complete flags
promptarena run --sc<Tab> # → promptarena run --scenario

# Complete scenario names from config.arena.yaml
promptarena run --scenario <Tab>
# → basic-chat  edge-cases  multi-turn  ...

# Complete provider IDs from config.arena.yaml
promptarena run --provider <Tab>
# → openai-gpt-4o-mini  claude-3-5-haiku  gemini-2-0-flash

promptarena config-inspect --section <Tab>
# → prompts  providers  scenarios  tools  selfplay  judges  defaults  validation

promptarena init --template <Tab>
# → code-assistant  content-generation  customer-support  mcp-integration  multimodal  quick-start  community/basic-chatbot ...
```

## Troubleshooting

### Completions not working

1. Check that the binary is on your `PATH`:
   ```bash
   which promptarena
   ```

2. Check that the completion script generates:
   ```bash
   promptarena completion bash | head -20
   ```

3. Install bash-completion:
   ```bash
   # macOS
   brew install bash-completion@2

   # Ubuntu/Debian
   apt install bash-completion

   # Alpine
   apk add bash-completion
   ```

4. Reload your shell:
   ```bash
   exec $SHELL
   ```

### No values for scenarios or providers

These completions read a valid `config.arena.yaml` in the current directory or a parent directory. With no config found, they return no values.

```bash
# Check that the config is found
cd /path/to/your/project
ls config.arena.yaml

# Test completion
promptarena __complete run --scenario ""
```

## Related pages

- [Install PromptArena](/arena/how-to/setup/installation/)
- [CLI commands](/arena/reference/cli-commands/)
