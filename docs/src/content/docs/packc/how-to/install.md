---
title: Install PackC
description: Install the packc compiler with npm, Homebrew, go install or a release archive, and enable shell completions.
sidebar:
  order: 1
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - .goreleaser.yml
    - Makefile
    - npm/packc/lib/resolve.js
    - npm/packc/package.json
    - packc/main.go
    - packc/main_interactive.go
---
Install the `packc` binary and check that it runs.

## Prerequisites

- For `go install` or a source build: Go 1.26 or later.
- For npm: Node.js and npm, yarn or pnpm.

## Install

Pick one method.

### npm

The npm package ships prebuilt binaries for Linux, macOS and Windows on amd64 and arm64, so it needs no Go toolchain.

```bash
# Install globally
npm install -g @altairalabs/packc

# Or run without installing
npx @altairalabs/packc version
```

### Homebrew (macOS and Linux)

```bash
brew install altairalabs/tap/packc
```

### go install

```bash
go install github.com/AltairaLabs/promptarena/v2/packc@latest
```

The binary goes to `$GOBIN`, or `$GOPATH/bin` when `GOBIN` is unset. To pin a release, replace `latest` with its tag, for example `@v2.4.0`.

### Release archive

Download `packc_<version>_<Os>_<arch>.tar.gz` from the [PromptArena releases](https://github.com/AltairaLabs/promptarena/releases) page. `<Os>` is `Darwin`, `Linux` or `Windows`, and `<arch>` is `x86_64` or `arm64`. Each release also attaches `checksums.txt`.

```bash
curl -LO https://github.com/AltairaLabs/promptarena/releases/download/v2.4.0/packc_2.4.0_Darwin_arm64.tar.gz
tar -xzf packc_2.4.0_Darwin_arm64.tar.gz packc
sudo mv packc /usr/local/bin/packc
```

### Build from a checkout

```bash
git clone https://github.com/AltairaLabs/promptarena.git
cd promptarena
go build -o bin/packc ./packc
```

`make build-go` builds both `bin/promptarena` and `bin/packc`.

## Verify

```bash
packc version
```

A release build (npm, Homebrew or an archive) prints its version, for example `packc 2.4.0`. A `go install` or source build prints `packc dev`, because only the release build sets the version string.

`packc help` prints the command list.

## Enable shell completions

`packc completion <shell>` prints a completion script for `bash`, `zsh`, `fish` or `powershell`:

```bash
# Bash
packc completion bash > ~/.local/share/bash-completion/completions/packc

# Zsh (the directory must be on your fpath)
packc completion zsh > ~/.zsh/completions/_packc

# Fish
packc completion fish > ~/.config/fish/completions/packc.fish
```

Start a new shell to load it. [Configure shell completions](/arena/how-to/setup/shell-completions/) covers fpath and bash-completion setup in detail.

## Troubleshooting

### packc: command not found after go install

Go's bin directory is not on your `PATH`. Add it to `~/.zshrc` or `~/.bashrc`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

On Windows, add `%USERPROFILE%\go\bin` to the `Path` environment variable and restart the terminal.

### go install fails with a Go version error

The module needs Go 1.26 or later. Check with `go version`, and upgrade from [go.dev/dl](https://go.dev/dl/) or with your package manager. Alternatively, install with npm or Homebrew, which need no Go toolchain.

## Uninstall

```bash
npm uninstall -g @altairalabs/packc     # npm
brew uninstall altairalabs/tap/packc    # Homebrew
rm "$(go env GOPATH)/bin/packc"         # go install
```

## Related

- [Compile packs](/packc/how-to/compile-packs/)
- [First pack tutorial](/packc/tutorials/01-first-pack/)
- [version reference](/packc/reference/version/)
- [Install PromptArena](/arena/how-to/setup/installation/)
