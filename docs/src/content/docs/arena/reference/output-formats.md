---
title: Output Formats
---

PromptArena supports multiple output formats for test results, each optimized for different use cases. You can generate multiple formats simultaneously from a single test run.

## Supported Formats

| Format | Use Case | File Extension | CI/CD Integration |
|--------|----------|----------------|-------------------|
| **JSON** | Programmatic access, APIs | `.json` | Excellent |
| **Markdown** | Human review, documentation, GitHub | `.md` | Good |
| **JUnit XML** | CI/CD systems | `.xml` | Excellent |
| **CSV** | Charts, spreadsheets, notebooks | `.csv` | — |
| **Objectives** | How often self-play objectives were achieved, with confidence intervals | `.json`, `.md` | — |

> **There is no HTML report.** Interactive review is served by the web UI: run `promptarena serve` and you get the same content the old `report.html` carried, plus live runs. See [Interactive Web UI](#interactive-web-ui) below. It needs no format flag, because it reads the per-run JSON files from your output directory.
>
> `html` is still accepted as a format for backwards compatibility: passing `--format html`, or setting `html` in `defaults.output.formats`, silently produces a Markdown report instead. Existing configs keep working — use `markdown` in new ones.

## Output Directory Structure

After running tests, Arena creates the following structure:

```text
out/
  results.json           # JSON results
  results.md             # Markdown report
  junit.xml              # JUnit XML
  results.csv            # CSV, one row per run
  objectives.json        # Objective report (objectives format)
  objectives.md          # The same report as Markdown tables
  media/                 # Media storage (images, audio, video)
    run-20241124-123456/
      session-xyz/
        conv-abc/
          image1.png
          image1.png.meta
```

**Media Directory:**

Arena automatically creates a `media/` directory to store large media content (images, audio, video) generated or processed during tests. This prevents memory issues and makes test artifacts easy to access.

- **Organization**: By-run (each test run isolated)
- **Deduplication**: Enabled (shared media stored once)
- **Metadata**: Each media file has a `.meta` sidecar with context
- **Location**: `{output_dir}/media/run-{timestamp}/`

See: [Media Storage Documentation](https://promptkit.altairalabs.ai/sdk/how-to/) for details.

## Configuration

Configure output in `arena.yaml`:

```yaml
defaults:
  output:
    dir: out                          # Output directory
    formats: ["json", "markdown", "junit"]

    # Format-specific options
    markdown:
      file: results.md               # Custom markdown output filename
      include_details: true          # Include detailed test information
      show_overview: true            # Show executive overview section
      show_results_matrix: true      # Show results matrix table
      show_failed_tests: true        # Show failed tests section
      show_cost_summary: true        # Show cost analysis section

    junit:
      file: junit.xml                # Custom JUnit output filename
```

## JSON Format

Machine-readable format for programmatic access and integrations.

### Structure

```json
{
  "arena_config": {
    "name": "customer-support-arena",
    "timestamp": "2024-01-15T10:30:00Z",
    "version": "v1.0.0"
  },
  "summary": {
    "total_tests": 15,
    "passed": 12,
    "failed": 3,
    "errors": 0,
    "skipped": 0,
    "total_duration": "45.2s",
    "total_cost": 0.0234,
    "total_tokens": 4521
  },
  "results": [
    {
      "scenario": "basic-qa",
      "provider": "openai-gpt4o-mini",
      "status": "passed",
      "duration": "3.2s",
      "turns": [
        {
          "turn_number": 1,
          "role": "user",
          "content": "What is the capital of France?",
          "response": {
            "role": "assistant",
            "content": "The capital of France is Paris.",
            "cost_info": {
              "input_tokens": 25,
              "output_tokens": 12,
              "cost": 0.00001
            }
          },
          "assertions": [
            {
              "type": "content_includes",
              "passed": true,
              "message": "Should mention Paris",
              "details": null
            }
          ]
        }
      ],
      "cost_info": {
        "total_input_tokens": 75,
        "total_output_tokens": 45,
        "total_cost": 0.00003
      }
    }
  ]
}
```

### Configuration Options

JSON output has no format-specific options. It is enabled by adding
`json` to `defaults.output.formats`, and written to `<dir>/index.json` (with
per-run files) in the configured output directory.

### Use Cases

**1. API Integration**
```bash
# Parse results in script
jq '.summary.passed' out/results.json
```

**2. Custom Reporting**
```python
import json

with open('out/results.json') as f:
    results = json.load(f)

passed = results['summary']['passed']
total = results['summary']['total_tests']
print(f"Pass rate: {passed/total*100:.1f}%")
```

**3. Data Analysis**
```python
# Analyze costs per provider
for result in results['results']:
    provider = result['provider']
    cost = result['cost_info']['total_cost']
    print(f"{provider}: ${cost:.4f}")
```

### Schema

Complete schema definition:

```typescript
interface TestResults {
  arena_config: {
    name: string
    timestamp: string
    version: string
  }
  summary: {
    total_tests: number
    passed: number
    failed: number
    errors: number
    skipped: number
    total_duration: string
    total_cost: number
    total_tokens: number
    average_cost: number
  }
  results: TestResult[]
}

interface TestResult {
  scenario: string
  provider: string
  status: "passed" | "failed" | "error" | "skipped"
  duration: string
  error?: string
  turns: Turn[]
  cost_info: CostInfo
  metadata?: Record<string, any>
}

interface Turn {
  turn_number: number
  role: "user" | "assistant"
  content: string
  response?: {
    role: string
    content: string
    tool_calls?: ToolCall[]
    cost_info: CostInfo
  }
  assertions?: Assertion[]
}

interface Assertion {
  type: string
  passed: boolean
  message: string
  details?: any
}

interface CostInfo {
  input_tokens: number
  output_tokens: number
  cached_tokens?: number
  cost: number
}
```

## Interactive Web UI

Arena has no standalone HTML report. Interactive review lives in the web UI instead — `promptarena serve` starts a local web server that covers the same ground and adds live runs on top:

- **Standings**: pass rates ranked across providers
- **Trial matrix**: a scenario × provider grid; each cell shows pass rate, cost, and latency
- **Cell scoping**: click a cell to scope the run ledger to that scenario/provider pair
- **Transcripts**: open any run from the ledger for its full conversation, turn by turn, with assertion outcomes
- **Cost and latency**: per-cell and per-run, plus a running spend estimate before you start
- **Historical results**: previous runs load alongside the current one
- **Live streaming**: run events arrive over SSE as the run executes — the old static report could only show finished runs
- **Interactive chat**: talk to a configured agent directly in the browser

```bash
# Start the web UI (loads existing results, and can start new runs)
promptarena serve

# Open a browser automatically
promptarena serve --open

# Serve on a custom port
promptarena serve -p 3000
```

The web UI is the recommended way to review results locally. For a file you can archive, diff, or paste into a PR, use the Markdown report.

## Markdown Format

GitHub-friendly markdown format for documentation.

### Structure

```markdown
# PromptArena Test Report

**Generated**: 2024-01-15 10:30:00
**Arena**: customer-support-arena

## Summary

- **Total Tests**: 15
- **Passed**: 12
- **Failed**: 3
- **Duration**: 45.2s
- **Cost**: $0.0234
- **Tokens**: 4,521

## Results by Provider

### OpenAI GPT-4o-mini

#### Scenario: basic-qa

**Status**: PASSED
**Duration**: 3.2s
**Cost**: $0.00003

##### Turn 1

**User**: What is the capital of France?

**Assistant**: The capital of France is Paris.

**Assertions**:
- content_includes: Should mention Paris

**Tokens**: 37 (input: 25, output: 12)
**Cost**: $0.00001

---

### Claude 3.5 Sonnet

...
```

### Configuration Options

```yaml
markdown:
  file: report.md             # Custom markdown output filename
  include_details: true       # Include full conversation details
  show_overview: true         # Show executive overview section
  show_results_matrix: true   # Show results matrix table
  show_failed_tests: true     # Show failed tests section
  show_cost_summary: true     # Show cost analysis section
```

### Use Cases

**1. GitHub Actions Summary**
```yaml
- name: Generate Report
  run: promptarena run arena.yaml
- name: Comment on PR
  uses: actions/github-script@v6
  with:
    script: |
      const fs = require('fs');
      const report = fs.readFileSync('out/results.md', 'utf8');
      github.rest.issues.createComment({
        issue_number: context.issue.number,
        owner: context.repo.owner,
        repo: context.repo.repo,
        body: report
      });
```

**2. Documentation**

Include test results in docs:

```markdown
# API Testing Results

[include file="test-results/results.md"]
```

**3. Slack/Teams Notifications**

Send markdown to collaboration tools:

```bash
# Convert to Slack format
cat out/results.md | slack-markdown-converter | \
  slack-cli chat-post-message --channel #testing
```

## JUnit XML Format

Standard format for CI/CD systems (Jenkins, GitLab CI, GitHub Actions, etc.).

### Structure

```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="PromptArena Tests" tests="15" failures="3" errors="0" time="45.2">
  <testsuite name="basic-qa" tests="3" failures="0" errors="0" time="9.6">
    <testcase name="basic-qa.openai-gpt4o-mini" classname="basic-qa" time="3.2">
      <system-out>
        Turn 1:
        User: What is the capital of France?
        Assistant: The capital of France is Paris.
        Assertions: content_includes
      </system-out>
    </testcase>

    <testcase name="basic-qa.claude-sonnet" classname="basic-qa" time="3.1">
      <failure message="Assertion failed: content_includes" type="AssertionFailure">
        Expected: Paris
        Actual: The capital city of France is Paris.
        Assertion: Should mention Paris
      </failure>
    </testcase>
  </testsuite>
</testsuites>
```

### Configuration Options

```yaml
junit:
  file: junit.xml             # Custom JUnit output filename
```

### CI/CD Integration

#### GitHub Actions

```yaml
- name: Run Tests
  run: promptarena run arena.yaml

- name: Publish Test Results
  uses: EnricoMi/publish-unit-test-result-action@v2
  if: always()
  with:
    files: out/junit.xml
```

#### GitLab CI

```yaml
test:
  script:
    - promptarena run arena.yaml
  artifacts:
    reports:
      junit: out/junit.xml
```

#### Jenkins

```groovy
pipeline {
  stages {
    stage('Test') {
      steps {
        sh 'promptarena run arena.yaml'
      }
      post {
        always {
          junit 'out/junit.xml'
        }
      }
    }
  }
}
```

#### CircleCI

```yaml
- run:
    name: Run Tests
    command: promptarena run arena.yaml
- store_test_results:
    path: out/junit.xml
```

## CSV Format

`csv` writes `results.csv` into the output directory: one row per run,
for charting in a spreadsheet, notebook or plotting library.

```yaml
defaults:
  output:
    formats: ["json", "csv"]
```

### Columns

Every row starts with the same fixed columns:

| Column | Meaning |
|---|---|
| `run_id`, `scenario_id`, `provider_id`, `region` | Which run this is |
| `passed` | Same pass rule as the JUnit and Markdown reports; `false` for skipped runs |
| `skipped` | Run was skipped (e.g. a transient provider error) rather than failed |
| `stop_reason` | Why a failed run stopped when it hit a cap, e.g. `max_rounds`; empty otherwise |
| `error` | Error message, if any |
| `duration_ms` | Wall-clock duration |
| `input_tokens`, `output_tokens`, `cached_tokens`, `total_cost_usd` | Token and cost totals |
| `tool_calls_total` | Tool calls in the run |

Then dynamic columns, grouped in this order:

| Column | Source |
|---|---|
| `label.<key>` | Scenario `metadata.labels` — e.g. `label.arm` to group runs by experiment arm |
| `assertion.<n>.<type>.passed` / `.score` | The run's n-th conversation assertion (1-based, declaration order) |
| `eval.<eval_id>.score` | Pack eval scores; for an eval that fires per turn, the last turn's score |
| `eval.<eval_id>.<key>` | Each of the eval's `metrics`, and each top-level numeric or boolean entry in its `data`, e.g. `eval.baxbench.security_score`; a metric wins over a `data` entry with the same name, and nested values stay in the run JSON |
| `tool.<name>.count` | Calls per tool |

The header is the union across all runs, so a run that lacks a column
(a different scenario's assertions, a tool it never called) leaves that
cell empty.

## Objectives Format

`objectives` reports how often self-play turn [objectives](/arena/reference/config-schema/#objectives) were achieved. It writes `objectives.json` and `objectives.md` into the output directory. Runs without an objective are left out.

```bash
promptarena run --ci --formats json,objectives \
  --objectives-group-by scenario,label.category,data.tier
```

The report always has an overall table. It then has one table per `--objectives-group-by` key (default `scenario`). The flag only applies with the `objectives` format; without it, the flag does nothing and a warning is printed.

| Key | Groups runs by |
|---|---|
| `scenario` | Scenario ID |
| `provider` | Provider ID |
| `objective` | The objective's `description`, after dataset placeholders are filled in, or `<scenario> turn <n> objective` when it has none |
| `label.<key>` | A scenario `metadata.labels` value |
| `data.<column>` | A dataset row's column, for scenarios that run over a [dataset](/arena/how-to/scenarios/run-a-dataset/) |

A run that lacks the key is grouped under `(none)`. An unknown key is an error.

Each table has a row per value and expected outcome. Objectives with `expect: achieved` and with `expect: not_achieved` are never counted together. For `not_achieved`, the rate is how often the agent failed to hold out.

Each row reports:

| Field | Meaning |
|---|---|
| `expect` | The objectives' expected outcome: `achieved` or `not_achieved` |
| `attempts` | Objective attempts whose success checks were evaluated |
| `achieved` | Attempts that reached the objective |
| `rate` | `achieved / attempts`. The Markdown shows `–` when nothing was judged. |
| `ci_low`, `ci_high` | 95% Wilson score interval on `rate` |
| `mean_exchanges_to_achieve` | Mean exchange at which achieved attempts got there |
| `cost`, `cost_per_achieved` | Cost of the group's runs in USD, and that divided by `achieved`. A run with several objectives splits its cost evenly between them, so the costs in any one table, with their baselines' costs, add up to the overall cost. |
| `unevaluated` | Attempts that were never judged: their checks couldn't be evaluated, or the run or trial errored before recording the objective. Not counted in `attempts`. |
| `unreliable` | Judged attempts on a self-play turn where the persona invented a specific it was not given (see [persona facts](/arena/reference/config-schema/#persona-facts)). The attempt measured the simulator as much as the agent, so it is not counted in `attempts`. Its cost still counts. |
| `baseline` | The group's attempts at objectives marked [`baseline: true`](/arena/reference/config-schema/#objectives), with the same fields as a row. They are counted here and not in the row's other fields, and their cost is `baseline.cost`. Absent when the group has none. |
| `lift` | How much more often the attack reached its objective than the baseline did: the attack's rate minus the baseline's, both over only the objectives that the attack and the baseline both attempted. Absent unless one objective has evaluated attempts on both sides. |

**Baselines.** A rate on its own doesn't say how much of it the attack earned: an agent that hands out refunds to anyone who asks scores high against every attack. A baseline objective pursues the same goal without the attack, for example with a persona that asks once and plainly, and is marked `baseline: true`. A baseline pairs with the attack attempts whose objective has the same `description`, after dataset placeholders are filled in. Its attempts land in the same row as the attack's when they share the group's value, so group by `objective`, or by a `data.<column>` or `label.<key>` they share. Grouped by `scenario`, a baseline in its own scenario gets its own row, with `attempts` 0 and no lift. An objective with no baseline, or a baseline with no attack, is left out of the lift, so a row that mixes them doesn't credit the attack with a lift it never earned. The Markdown adds Baseline and Lift columns when any run had a baseline. The Baseline cell shows achieved / attempts, the rate and interval, the cost, and any unevaluated or unreliable attempts; the lift is in percentage points.

How the counts are built:

- **Trials.** A scenario with `trials` contributes one attempt per trial, and its cost covers every trial, not only the run reported for the group.
- **Errored runs.** A run or trial that fails before reaching an objective's turn reports that objective as not evaluated ("the run ended before this turn"), and so does one whose objective turn itself errors before the objective is achieved. Either way it counts as unevaluated under its own description and dataset row, once per trial, and its cost still counts. A run with no result at all, such as one that timed out before saving a message, is counted as unevaluated for each objective its scenario's other runs recorded. A run skipped for a transient provider error is left out, as it is everywhere else. An objective already achieved stays counted as achieved even if a later check errored.
- **What a rate means.** A rate is the share of these attempts, at this budget, that reached the objective, and the interval says how far it can be trusted. A rate of 0 does not show that the objective can't be reached.

## Multiple Formats

Generate all formats in one run:

```yaml
defaults:
  output:
    formats: ["json", "markdown", "junit"]
```

Output structure:
```
out/
├── results.json
├── results.md
└── junit.xml
```

## Custom Output Directory

```yaml
defaults:
  output:
    dir: test-results-2024-01-15
```

```bash
# Or override via CLI
promptarena run arena.yaml --out custom-dir
```

## Programmatic Access

### Python

```python
import json

# Load JSON results
with open('out/results.json') as f:
    results = json.load(f)

# Calculate pass rate
summary = results['summary']
pass_rate = summary['passed'] / summary['total_tests'] * 100

# Find expensive scenarios
for result in results['results']:
    if result['cost_info']['total_cost'] > 0.01:
        print(f"Expensive: {result['scenario']} - ${result['cost_info']['total_cost']:.4f}")

# Find failing assertions
for result in results['results']:
    if result['status'] == 'failed':
        for turn in result['turns']:
            for assertion in turn.get('assertions', []):
                if not assertion['passed']:
                    print(f"Failed: {result['scenario']} - {assertion['message']}")
```

### Node.js

```javascript
const fs = require('fs');

// Load results
const results = JSON.parse(fs.readFileSync('out/results.json'));

// Generate custom report
const report = results.results.map(r => ({
  scenario: r.scenario,
  provider: r.provider,
  passed: r.status === 'passed',
  cost: r.cost_info.total_cost
}));

console.table(report);
```

### Go

```go
package main

import (
    "encoding/json"
    "os"
)

type Results struct {
    Summary struct {
        TotalTests int     `json:"total_tests"`
        Passed     int     `json:"passed"`
        TotalCost  float64 `json:"total_cost"`
    } `json:"summary"`
}

func main() {
    data, _ := os.ReadFile("out/results.json")
    var results Results
    json.Unmarshal(data, &results)

    passRate := float64(results.Summary.Passed) / float64(results.Summary.TotalTests) * 100
    fmt.Printf("Pass Rate: %.1f%%\n", passRate)
    fmt.Printf("Total Cost: $%.4f\n", results.Summary.TotalCost)
}
```

## Performance Considerations

### File Sizes

Typical sizes for 100 tests:

| Format | Approx. Size | Notes |
|--------|--------------|-------|
| JSON | 500 KB | Can be large with raw responses |
| Markdown | 300 KB | Most compact |
| JUnit XML | 200 KB | Minimal data |

### Optimization

**Emit only the formats you need** — each format in `defaults.output.formats`
is written on every run, so drop the ones you do not consume:
```yaml
defaults:
  output:
    formats: ["json"]           # Skip Markdown/JUnit when not needed
```

**Trim Markdown reports** by disabling sections you do not use:
```yaml
markdown:
  include_details: false        # Omit full conversation transcripts
  show_results_matrix: false    # Omit the results matrix table
```

## Best Practices

### 1. Use Right Format for Context

```yaml
# Development
formats: ["markdown"]          # Quick readable review

# CI/CD
formats: ["junit", "json"]     # Integration + data

# Documentation
formats: ["markdown"]          # Human-readable

# Production
formats: ["json", "junit"]     # Programmatic + CI
```

### 2. Version Control

```gitignore
# .gitignore
out/
test-results/
```

Commit configuration, not results.

### 3. Archive Historical Results

```bash
# Archive with timestamp
DATE=$(date +%Y%m%d-%H%M%S)
mv out test-results-$DATE
tar -czf test-results-$DATE.tar.gz test-results-$DATE
```

### 4. Parse for Metrics

```bash
# Extract pass rate
jq '.summary | {total: .total_tests, passed: .passed, rate: ((.passed / .total_tests) * 100)}' out/results.json

# Extract cost by provider
jq '.results | group_by(.provider) | map({provider: .[0].provider, cost: map(.cost_info.total_cost) | add})' out/results.json
```

## Next Steps

- **[CI/CD Integration](/arena/how-to/interfaces/run-in-ci/)** - Running in pipelines
- **[Configuration Reference](/arena/reference/config-schema/)** - Output configuration
- **[Best Practices](/arena/how-to/scenarios/validate-outputs/)** - Production tips

---

**Examples**: See `examples/` for output configuration patterns.
