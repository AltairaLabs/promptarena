---
title: Run in CI
description: Run Arena headless in CI/CD with the --ci flag, JSON, JUnit and Markdown reports, quality gates, and the GitHub Action.
sidebar:
  order: 30
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - .github/actions/promptarena-action/action.yml
    - .github/actions/promptarena-action/src/main.ts
    - .github/actions/promptarena-action/src/outputs.ts
    - .github/actions/promptarena-action/src/runner.ts
    - .github/workflows/release.yml
    - .goreleaser.yml
    - arena/arenaconfig/helpers.go
    - arena/arenaconfig/mock_config_check.go
    - arena/arenaconfig/types.go
    - arena/assertions/conversation.go
    - arena/assertions/types.go
    - arena/cmd/promptarena/main_integration.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/engine/engine.go
    - arena/engine/eval_orchestrator.go
    - arena/engine/trials.go
    - arena/results/json/json_repository.go
    - arena/stages/assertions.go
    - arena/tui/tui.go
    - examples/llm-judge/scenarios/helpfulness.yaml
    - schemas/v1alpha1/common/assertions.json
    - schemas/v1alpha1/scenario.json
---

Wire `promptarena run` into CI so the pipeline produces machine-readable reports
and fails on regressions. This page covers GitHub Actions (raw binary or the
packaged Action), GitLab CI and Jenkins, and deterministic zero-cost runs with
mock providers.

For the interactive alternatives, see [Use the TUI](/arena/how-to/interfaces/use-the-tui/)
and [Use the web UI](/arena/how-to/interfaces/use-the-web-ui/).

## Headless mode: `--ci`

`promptarena run --ci` disables the terminal UI and emits plain line-based logs
suitable for CI log capture.

```bash
promptarena run --ci
```

Use it whenever there is no interactive terminal: GitHub Actions runners, GitLab
executors, Jenkins agents, Docker builds, cron jobs. Without `--ci`,
`promptarena run` uses the TUI when it can detect a terminal and falls back to
simple output otherwise.

The gate itself is the **exit code**: with `--ci`, `promptarena run` exits `0`
when every scenario's assertions pass, and non-zero when any assertion fails or a
run errors. Without `--ci` it exits `0` whatever the results, so always pass
`--ci` in a pipeline. `--simple` also disables the TUI, but it prints a styled
summary and keeps the `0` exit status.

Pair `--ci` with mock providers for fast, deterministic, no-API-cost runs (see
[Zero-cost runs](#zero-cost-runs-with-mock-providers) below):

```bash
promptarena run --ci --mock-provider --mock-config mock-responses.yaml
```

## Report formats

Pass `--format` (alias `--formats`) a comma-separated list. When omitted, Arena
uses `defaults.output.formats` from your config, falling back to `json`. The
formats for CI are below; `csv` and `objectives` are also available (see the
[output formats reference](/arena/reference/output-formats/)).

| Format | Output | Path flag (default) |
|---|---|---|
| `json` | Per-run JSON files plus an `index.json` summary in the out dir | `--out` dir (default `defaults.output.dir`, else `out/`) |
| `junit` | JUnit XML, read by most CI test reporters | `--junit-file` (default `defaults.output.junit.file`, else `<out>/junit.xml`) |
| `markdown` | Markdown report | `--markdown-file` (default `defaults.output.markdown.file`, else `<out>/results.md`) |

```bash
# JUnit for the CI test reporter, JSON for archival, Markdown for humans
promptarena run --ci --format junit,json,markdown
```

The output directory is `--out`, else `defaults.output.dir`, else `out/`. The
`index.json` summary in it carries `total_runs`, `successful` and `errors`, which
a gating step can read. See the [output formats reference](/arena/reference/output-formats/) for the
full schema of each report.

:::note[There is no HTML report]
Arena does not emit an HTML report; the markdown report covers it. The `html`
format name, the `--html` flag and the `html_report` config key produce
**markdown**.
:::

## Quality gates: fail the build on regressions

A quality gate is a CI check that exits non-zero on the regressions you want to
catch before merge: behavioural drift, tool-call regressions, safety guardrails
that stopped firing, or a latency budget breach. Because `promptarena run --ci`
already exits non-zero on any assertion failure, wiring the gate is usually
"make this step a required check."

For noisier real-provider runs, tune the assertions themselves rather than the CI
step. Set `trials` on the scenario to run it several times, and
`pass_threshold` on an assertion to say what share of those trials it must pass:

```yaml
spec:
  trials: 5                 # run the scenario 5 times
  turns:
    - role: user
      content: "Where is order 1234?"
      assertions:
        - type: assertion
          params:
            eval_type: llm_judge
            eval_params:
              criteria: "Agent stayed professional under pressure"
            min_score: 0.7
          pass_threshold: 0.8   # 4 of 5 trials must pass
  conversation_assertions:
    - type: tools_called
      params:
        tool_names: [lookup_order]
      pass_threshold: 0.8       # 4 of 5 trials must pass
```

An assertion without `pass_threshold` must pass in every trial. The run
reported for the scenario carries the verdict across trials, so the exit code,
JUnit and CSV fail when any assertion falls below its threshold.

| Gate type | Assertion shape | Why |
|---|---|---|
| Tool-call regression | `tools_called` / `tool_calls_with_args` / `tool_call_sequence` | Deterministic, binary, cheap |
| Safety guardrail | `guardrail_triggered` | Reads recorded validations; no LLM cost |
| Model migration | `content_includes` / `outcome_equivalent` / `max_length` | Compare per-model outputs; fail if any cell regresses |
| Latency budget | `latency_budget` | Reads `LatencyMs` from the assistant message |
| LLM-judged quality | `type: assertion` wrapping `llm_judge`, `min_score` + `pass_threshold` | Tolerate noise; pair with `trials` |

If you need to key a downstream step on the numbers rather than the exit code,
read `index.json` from the out directory (`total_runs`, `successful`, `errors`),
or use the GitHub Action's outputs (`passed`, `failed`, `errors`, `total`,
`success`). `failed` and `errors` both count every run that did not pass (the
`errors` field of `index.json`); skipped runs are in `total` but in neither
`passed` nor `failed`.

## The GitHub Action

Arena ships a packaged GitHub Action at `.github/actions/promptarena-action/`. It
installs the requested Arena version, runs your config headless, surfaces
pass/fail counts as step outputs, and fails the job on test failures by default.
Reference it as `AltairaLabs/promptarena/.github/actions/promptarena-action@v2`:
`v2` is a floating tag each release moves to the newest `v2.x.y`, and a release
tag such as `@v2.4.0` pins it.

```yaml
name: Arena quality gate

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

jobs:
  arena:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run Arena tests
        id: arena
        uses: AltairaLabs/promptarena/.github/actions/promptarena-action@v2
        with:
          config-file: config.arena.yaml
          version: latest
          formats: json,junit,markdown
        env:
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}

      - name: Publish test results
        uses: dorny/test-reporter@v2
        if: always()
        with:
          name: Arena Tests
          path: out/junit.xml
          reporter: java-junit

      - name: Upload reports
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: arena-reports
          path: out/
```

Useful inputs: `config-file` (required), `version`, `scenarios`, `providers`,
`regions`, `formats` (default `json,junit`), `output-dir`, `junit-output`,
`override-providers`, `fail-on-error` (default `true`), and `working-directory`.
Set `fail-on-error: 'false'` to inspect `steps.arena.outputs.success` yourself
instead of failing the job automatically:

```yaml
      - name: Run Arena tests (advisory)
        id: arena
        uses: AltairaLabs/promptarena/.github/actions/promptarena-action@v2
        with:
          config-file: config.arena.yaml
          fail-on-error: 'false'

      - name: Gate
        if: steps.arena.outputs.success == 'false'
        run: |
          echo "::error::Arena gate failed: ${{ steps.arena.outputs.total }} runs, success is false"
          exit 1
```

### Fork-safe split (raw binary)

Real-provider runs need API keys, which fork PRs can't see. The standard pattern
splits into a keyless job that runs on every PR (validation + mocks) and a
secret-gated job that skips for forks:

```yaml
jobs:
  # Runs on every PR including forks. Cheap, deterministic, no secrets.
  validate-and-mock:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run mock-provider scenarios
        run: ./bin/promptarena run --ci --mock-provider --format json,junit

  # Skips for forks: the head repo check fails when secrets aren't available.
  real-providers:
    runs-on: ubuntu-latest
    if: github.event.pull_request.head.repo.full_name == github.repository
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run against real providers
        env:
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: ./bin/promptarena run --ci --format markdown,json
      - name: Upload report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: arena-report
          path: out/
```

Add both jobs to your branch protection rule's required status checks. Keep the
keyless job strictly required; make the real-provider job required or advisory
depending on cost tolerance.

## GitLab CI

```yaml
# .gitlab-ci.yml
stages:
  - test

arena-tests:
  stage: test
  image: golang:1.26
  variables:
    OPENAI_API_KEY: $OPENAI_API_KEY
    ANTHROPIC_API_KEY: $ANTHROPIC_API_KEY
  before_script:
    - go install github.com/AltairaLabs/promptarena/v2/arena/cmd/promptarena@latest
  script:
    - promptarena run --ci --format junit,json
  artifacts:
    when: always
    reports:
      junit: out/junit.xml
    paths:
      - out/
```

GitLab surfaces the JUnit report in the merge request widget automatically via the
`reports.junit` key. Store API keys as masked/protected CI variables.

## Jenkins

```groovy
// Jenkinsfile
pipeline {
    agent any
    environment {
        OPENAI_API_KEY    = credentials('openai-api-key')
        ANTHROPIC_API_KEY = credentials('anthropic-api-key')
    }
    stages {
        stage('Install') {
            steps {
                sh 'go install github.com/AltairaLabs/promptarena/v2/arena/cmd/promptarena@latest'
            }
        }
        stage('Mock validation') {
            steps {
                sh 'promptarena run --ci --mock-provider --format junit'
            }
        }
        stage('Provider tests') {
            steps {
                sh 'promptarena run --ci --format junit,markdown'
            }
        }
    }
    post {
        always {
            junit 'out/**/junit.xml'
            archiveArtifacts artifacts: 'out/**/*', allowEmptyArchive: true
        }
    }
}
```

The `junit` post step turns Arena's JUnit XML into Jenkins test-trend graphs; the
non-zero exit from `promptarena run` marks the build failed.

## Zero-cost runs with mock providers

Mock providers make CI runs deterministic and free: no provider API keys, no
network, no per-run cost. `--mock-provider` replaces every configured provider
with a canned-response mock; `--mock-config` points at a YAML file of scripted
responses. Arena rejects unknown keys in that file, so a typo fails the run
instead of silently falling back to the default response. Tools still execute for real, so tool-call and workflow assertions
remain meaningful.

```bash
# Deterministic gate, no API keys required
promptarena run --ci --mock-provider --mock-config mock-responses.yaml
```

This is ideal for the per-PR gate (including fork PRs) and for validating config
structure before spending money on real-provider runs. To keep CI runs cheap and
stable when you do hit real providers, drop concurrency to respect rate limits:

```bash
promptarena run --ci --concurrency 2 --format junit,json
```

See [Use mock providers](/arena/how-to/providers/use-mock-providers/) for authoring mock
response files, and [Generate mock responses](/arena/how-to/scenarios/generate-mock-responses-from-arena/)
to capture them from a live run.

## Related

- [Output formats reference](/arena/reference/output-formats/): the schema of each report
- [CLI commands reference](/arena/reference/cli-commands/): every `run` flag
- [Use the TUI](/arena/how-to/interfaces/use-the-tui/): the interactive terminal UI
- [Use the web UI](/arena/how-to/interfaces/use-the-web-ui/): the browser-based interface
