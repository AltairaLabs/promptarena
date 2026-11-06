---
title: Use Project Templates
description: Scaffold a PromptArena test project with promptarena init, using the built-in quick-start template or a community template.
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - arena/cmd/promptarena/init.go
    - arena/cmd/promptarena/init_interactive.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/templates.go
    - arena/templates/builtin/code-assistant/template.yaml
    - arena/templates/builtin/content-generation/template.yaml
    - arena/templates/builtin/customer-support/template.yaml
    - arena/templates/builtin/mcp-integration/template.yaml
    - arena/templates/builtin/multimodal/template.yaml
    - arena/templates/builtin/quick-start/template.yaml
    - arena/templates/generator.go
    - arena/templates/loader.go
    - arena/templates/registry.go
    - arena/templates/repos.go
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/promptconfig.json
    - schemas/v1alpha1/provider.json
    - schemas/v1alpha1/scenario.json
---

Scaffold a ready-to-run PromptArena test project with `promptarena init`.

## Prerequisites

- The `promptarena` binary. See [Installation](/arena/how-to/setup/installation/).
- An API key for the provider you choose, unless you use `mock`.

## Create a project in quick mode

Quick mode takes each template variable's default and asks no questions:

```bash
# Mock provider: no API calls, no key
promptarena init my-test --quick --provider mock

# Real provider: openai, claude or gemini
promptarena init my-test --quick --provider openai
```

The built-in templates are `quick-start` (the default for `--template`), `code-assistant`, `content-generation`, `customer-support`, `mcp-integration` and `multimodal`. See the [CLI reference](/arena/reference/cli-commands/) for every `init` flag.

Quick mode with `--provider openai` writes:

```text
my-test/
├── .claude/skills/promptarena-authoring/   (SKILL.md and 4 reference files)
├── .env
├── .gitignore
├── AGENTS.md
├── README.md
├── config.arena.yaml
├── prompts/assistant.prompt.yaml
├── providers/openai.provider.yaml
└── scenarios/basic-test.scenario.yaml
```

`--provider mock` also writes `mock-responses.yaml` at the project root, and `providers/mock.provider.yaml` replaces the OpenAI provider file. `AGENTS.md` is skipped with `--no-agent`.

## Create a project interactively

Run `init` without `--quick`:

```bash
promptarena init my-project
```

The command prompts for:

- `Project name`, only when you give no name on the command line (default `my-arena-tests`).
- The provider.
- `include_env`: whether to create a `.env` file.

## Create a project from a community template

Community templates come from an index, which defaults to the `promptkit-templates` repository. `templates list` prints the index entries only; it does not list the built-in templates.

```bash
# List community templates
promptarena templates list

# Fetch a template into the cache
promptarena templates fetch --template basic-chatbot --version 1.0.0

# Render a template into ./out
promptarena templates render --template basic-chatbot --version 1.0.0 --values values.yaml --out ./out

# Update all cached templates
promptarena templates update

# Create a project from a community template
promptarena init my-project --template basic-chatbot --template-index https://raw.githubusercontent.com/AltairaLabs/promptkit-templates/main/index.yaml
```

The `templates` commands take `--index` and `--cache-dir`; `init` takes `--template-index` and `--template-cache` for the same settings. `render` takes variables from `--values` and `--set`. See the [CLI reference](/arena/reference/cli-commands/) for the full lists.

## Run the generated project

1. Change into the project:

   ```bash
   cd my-test
   ```

2. Export the API key in your shell. `promptarena` does not load `.env`, so the placeholder file has no effect until you export the variable:

   ```bash
   export OPENAI_API_KEY=sk-...
   ```

   Claude reads `ANTHROPIC_API_KEY` or `CLAUDE_API_KEY`. Gemini reads `GEMINI_API_KEY` or `GOOGLE_API_KEY`. The mock provider needs no key.

3. Run the tests:

   ```bash
   promptarena run
   ```

4. Read the results. The default output directory is `out/`, holding `index.json`, one `<run>.json` per run and `results.md`:

   ```bash
   cat out/results.md
   ```

## What the generator writes

`config.arena.yaml` lists the prompt, provider and scenario files and sets the run defaults:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: my-test

spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.prompt.yaml

  providers:
    - file: providers/openai.provider.yaml

  scenarios:
    - file: scenarios/basic-test.scenario.yaml

  defaults:
    temperature: 0.7
    max_tokens: 1500
    seed: 42
    concurrency: 1
    output:
      dir: out
      formats: ["json", "markdown"]
```

`prompts/assistant.prompt.yaml` holds the system prompt in `spec.system_template`, with `task_type: general`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: assistant
spec:
  task_type: general
  system_template: |
    You are a helpful AI assistant. Be concise and friendly.
```

`providers/openai.provider.yaml` names the provider, its type and its model:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai
spec:
  id: openai-gpt4o-mini
  type: openai
  model: gpt-4o-mini
```

`scenarios/basic-test.scenario.yaml` holds two turns. Each assertion carries its `message` beside `params`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: basic-test
spec:
  id: greeting-test
  task_type: general
  turns:
    - role: user
      content: "What's the capital of France?"
      assertions:
        - type: contains_any
          params:
            patterns: ["Paris", "paris"]
          message: "Response should mention Paris"
```

## Customize the project

Edit the generated files, or add scenarios and providers and list them in `config.arena.yaml`:

```bash
cp scenarios/basic-test.scenario.yaml scenarios/advanced-test.scenario.yaml
```

Add `- file: scenarios/advanced-test.scenario.yaml` under `spec.scenarios`. To move from the mock provider to a real one, add a provider file under `spec.providers` and remove the `additional_config` block from any copy of the mock file.

## Put the project under version control

The generated `.gitignore` lists `.env`, `out/` and `*.log`:

```bash
git init
git add .
git commit -m "Initial project setup"
```

## Troubleshooting

### Unauthorized or missing bearer error

The key is empty because `promptarena` does not read `.env`. Export the key in the shell, as in step 2 above.

### Provider is not offered

`--provider "anthropic" is not offered by template quick-start (choose one of: mock, openai, claude, gemini)`. Pass one of the listed values:

```bash
promptarena init my-test --quick --provider claude
```

### Existing files overwritten

`init` does not check whether the target directory exists, and it overwrites files that share a generated name. Choose a new project name to keep an existing project.

## Related pages

- [Write test scenarios](/arena/how-to/scenarios/write-scenarios/)
- [Configure providers](/arena/how-to/providers/configure-providers/)
- [Develop templates](/arena/how-to/setup/develop-templates/)
- [Tutorial: first test](/arena/tutorials/01-first-test/)
- [CLI reference](/arena/reference/cli-commands/)
- [Configuration schema](/arena/reference/config-schema/)
