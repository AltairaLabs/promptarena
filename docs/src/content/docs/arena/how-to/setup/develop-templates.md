---
title: Develop Templates
description: Build a template in a local clone of the community template repo, render it with the promptarena CLI, and submit it.
verified:
  commit: 95b220864e8df93d5a36b38ad288897bf70a4c5b
  sources:
    - arena/cmd/promptarena/init.go
    - arena/cmd/promptarena/templates.go
    - arena/cmd/promptarena/templates_cmd_test.go
    - arena/templates/builtin/quick-start/template.yaml
    - arena/templates/loader.go
    - arena/templates/registry.go
    - arena/templates/repos.go
    - arena/templates/repos_test.go
    - arena/templates/resolver.go
    - arena/templates/resolver_test.go
    - arena/templates/templates_test.go
    - arena/templates/types.go
---

Add a template to the community repo and render it locally before you open a pull request.

## Prerequisites

- The `promptarena` binary, or the Go toolchain to build it and run the template tests.
- A clone of [`promptkit-templates`](https://github.com/AltairaLabs/promptkit-templates), for example at `../promptkit-templates`.

## Steps

1. Create the template package.
   - Add a directory to the template repo named after the template.
   - Add a `template.yaml` with `apiVersion: promptkit.altairalabs.ai/v1alpha1`, `kind: Template`, `spec.files` and any `variables`.
   - To keep large content in separate files, give a `spec.files` entry a `source` instead of inline content. A `source` path resolves relative to the directory of the `template.yaml` being rendered.

2. Add the template to the index.
   - In `index.yaml`, add an entry under `spec.entries` with `name`, `version`, `description` and `source: <template-name>/template.yaml`.
   - Use semver-style versions, and increment the version when you change a template.

3. Register your clone as a template repo.

   ```bash
   promptarena templates repo add --name local --url ../promptkit-templates/index.yaml
   promptarena templates list --index local
   ```

   Every `templates` command also accepts the index file path directly: `--index ../promptkit-templates/index.yaml`. Pass the same `--index` to `fetch` and `render`; a path index is cached under a key derived from its absolute path, so the two find the same copy.

4. Fetch and render the template.

   ```bash
   promptarena templates fetch --template local/your-template --version 1.0.0
   promptarena templates render --template local/your-template --version 1.0.0 \
     --values values.example.yaml --out ./out
   ```

   `render --template` reads the cached copy and needs `--version`. `fetch` caches `template.yaml` together with every file a relative `source` names, so a template that uses `source` renders from the cache. A `source` that climbs out of the template directory (`../`) is rejected at fetch. To render while you edit, skip the cache and render straight from your clone:

   ```bash
   promptarena templates render --file ../promptkit-templates/your-template/template.yaml \
     --values values.example.yaml --out ./out
   ```

5. Run the template tests (optional). From a PromptArena checkout:

   ```bash
   GOCACHE=.cache/go-build go test ./arena/templates ./arena/cmd/promptarena
   ```

   These cover template loading, repo resolution and the `templates` commands.

6. Document the template. Add a `README.md` to the template directory that describes the generated files and how to run them, and a `values.example.yaml` that shows the expected variables.

## Submission checklist

- [ ] `templates render` succeeds locally.
- [ ] `index.yaml` has the correct version and source path.
- [ ] Provider variables use the template provider options: `openai`, `claude`, `gemini`, `mock`.
- [ ] The template directory has a README and example values.
- [ ] The template tests pass (optional).

Open a pull request against `promptkit-templates` with the new template and the index update.

## Related pages

- [Use project templates](/arena/how-to/setup/use-project-templates/)
- [CLI commands](/arena/reference/cli-commands/)
