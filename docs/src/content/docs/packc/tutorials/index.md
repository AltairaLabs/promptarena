---
title: Tutorials
description: Five packc tutorials, from compiling a first pack to releasing packs from GitHub Actions.
sidebar:
  order: 0
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - packc/main_interactive.go
---
These tutorials teach `packc` by building packs, from one prompt to a release pipeline. Each builds on the one before, so start with the first if you are new to `packc`.

1. [Your First Pack](/packc/tutorials/01-first-pack/): install `packc`, then compile, inspect and validate a one-prompt pack.
2. [Multi-Prompt Packs](/packc/tutorials/02-multi-prompt/): compile several prompts and a tool into one pack.
3. [Validation Workflows](/packc/tutorials/03-validation-workflow/): see what compile and validate catch, and block broken packs with a pre-commit hook.
4. [Pack Management](/packc/tutorials/04-pack-management/): build a versioned pack per environment and compare releases.
5. [CI/CD Pipeline](/packc/tutorials/05-ci-cd-pipeline/): compile and validate in GitHub Actions, and release packs on a tag.

## Prerequisites

- Go 1.26 or later, to install `packc` with `go install`
- A terminal and a text editor
- Git, for tutorials 3 and 5
- A GitHub account, for tutorial 5

## Getting help

- [How-to guides](/packc/how-to/) for specific tasks
- [Reference](/packc/reference/) for every command and flag
- [GitHub issues](https://github.com/AltairaLabs/promptarena/issues) to report a problem
