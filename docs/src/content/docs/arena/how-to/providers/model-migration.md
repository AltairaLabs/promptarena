---
title: Gate Model Migrations
description: Run the same scenarios against the old and new model side by side. CI fails if the new model breaks an assertion the old one passed.
verified:
  commit: 95b220864e8df93d5a36b38ad288897bf70a4c5b
  sources:
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/tui/tui.go
    - arena/web/frontend/src/App.tsx
    - arena/web/frontend/src/components/arena/TrialMatrix.tsx
    - examples/model-migration/config.arena.yaml
    - examples/model-migration/mock-responses-v2.yaml
    - examples/model-migration/providers/model-v1.provider.yaml
    - examples/model-migration/providers/model-v2.provider.yaml
    - examples/model-migration/scenarios/billing-inquiry.scenario.yaml
    - examples/model-migration/scenarios/tech-inquiry.scenario.yaml
    - schemas/v1alpha1/common/assertions.json
---

Gate a model swap in CI with `examples/model-migration/`, a regression suite that runs the same scenarios against two mock providers simulating a model upgrade. The pattern works for any "did this swap break anything" question: GPT-4o to 4o-mini, Claude 3.5 to 4, OpenAI to Anthropic.

## What it proves

Most prompts still work after a model swap, so the change gets trusted. The few that broke produce subtly wrong outputs that pass visual review and surface in production.

The suite makes migration a gate:

- One scenario set, multiple providers registered, one report.
- Identical assertions per scenario; each must pass on every registered model.
- If any model fails an assertion the others passed, the report flags it and `run --ci` exits non-zero.
- Pin the suite in CI before the migration PR can merge.

## Run it

```bash
cd examples/model-migration
promptarena serve
```

The web UI lays the suite out as a scenario × provider matrix, one column per model. Click a cell to filter the run ledger below it to that scenario/model pair, then open a run to read the output and its assertion results. A wide field (a bake-off across a dozen or more models) scrolls sideways with the scenario column pinned, so every contender stays reachable.

Headless / CI:

```bash
promptarena run --ci --formats json,markdown
open out/results.md
```

Both providers are mock, so the example runs without API keys. The report shows side-by-side results.

## What a regression looks like

The default config has both mock providers passing all assertions. To see a regression surface, edit `mock-responses-v2.yaml` so the new model breaks the one-word format. Responses are keyed by the scenario's `metadata.name` under `scenarios:`, with turns keyed by turn number:

```yaml
scenarios:
  tech-inquiry:
    turns:
      1: "This sounds like a technical issue with the application."
```

Re-run. The `max_length` assertion fails on `model-v2` and passes on `model-v1`. The run summary lists the failing run as `scenario/provider/region`, and the command exits 1:

```text
Run Summary
===========

Total Runs:       4
Successful:       3 (75.0%)
Failed:           1 (25.0%)

Assertions:       8 total
Assertions Fail:  1
...
Providers:        model-v1 (2), model-v2 (2)
Scenarios:        2 scenarios
Regions:          default

Errors:
  • tech-inquiry/model-v2/default: pipeline execution failed: validation failed: [assertion 1 (max_length) failed: length 56, max 30]

Results saved to: out

Error: execution failed: 1 runs had errors, 1 assertions failed
```

The non-zero exit fails the CI step below, so the migration PR stays red until you rework the prompt or swap the new model out.

## Adding real models

The example ships only `providers/model-v1.provider.yaml` and `providers/model-v2.provider.yaml`. To test real models, add a provider file for each model, then replace the mock entries in `config.arena.yaml`. The file names below are examples you create:

```yaml
providers:
  - file: providers/openai-gpt4o.provider.yaml         # incumbent
  - file: providers/openai-gpt4o-mini.provider.yaml    # candidate
```

Same scenarios, same assertions. The report fans out across every provider, and CI gates on all of them passing.

## CI gate

```yaml
# .github/workflows/model-migration.yml
name: Model migration regression

on:
  pull_request:
    paths:
      - 'examples/model-migration/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run migration regression suite
        working-directory: examples/model-migration
        run: ../../bin/promptarena run --ci --formats json,markdown
      - name: Upload report
        if: always()
        uses: actions/upload-artifact@v7
        with:
          name: model-migration-report
          path: examples/model-migration/out/
```

GitHub Actions `run:` steps use `bash -e`, so the non-zero exit from `promptarena` fails the step. Uploading the report as an artifact lets reviewers read the per-model output on the PR.

## Extending it

- To add a third model, add another provider file, register it in `config.arena.yaml`, and run. Every scenario runs against every registered provider.
- `outcome_equivalent` checks a run's outcome against an expected value. Set `metric` to `tool_calls` (with `expected_tools`), `final_state` (with `expected_state`) or `content_hash` (with `expected_content`). It suits migrations where the prompt does not change.
- If a new model has known stricter or looser behaviour on certain assertions, use `when:` clauses to scope thresholds per provider (see [the bake-off how-to](/arena/how-to/voice/voice-bake-off/) for the pattern).
