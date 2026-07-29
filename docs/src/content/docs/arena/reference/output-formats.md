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

> **There is no HTML report.** Interactive review is served by the web UI — run `promptarena serve` and you get the same content the old `report.html` carried, plus live runs. See [Interactive Web UI](#interactive-web-ui) below. It needs no format flag — it reads the per-run JSON files from your output directory.
>
> `html` is still accepted as a format for backwards compatibility: passing `--format html`, or setting `html` in `defaults.output.formats`, silently produces a Markdown report instead. Existing configs keep working — use `markdown` in new ones.

## Output Directory Structure

After running tests, Arena creates the following structure:

```text
out/
  results.json           # JSON results
  results.md             # Markdown report
  junit.xml              # JUnit XML
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

JSON output currently has no format-specific options. It is enabled by adding
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
