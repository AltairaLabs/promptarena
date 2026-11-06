---
title: Test RAG Agents
description: Gate a RAG agent with faithfulness, answer_relevancy, contextual_precision, contextual_recall, contextual_relevancy and hallucination assertions. Run keyless with a mock judge, or swap in a real LLM judge.
verified:
  commit: 3df344ffbc8c604e1a8935929c28499d778d19f0
  sources:
    - .github/workflows/ci.yml
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - arena/assertions/types.go
    - arena/cmd/promptarena/completion_interactive.go
    - arena/cmd/promptarena/run.go
    - arena/engine/builder_integration.go
    - arena/engine/conversation_executor.go
    - arena/engine/eval_orchestrator.go
    - arena/redteam/generate.go
    - arena/results/markdown/markdown_repository.go
    - examples/rag-agent/config.arena.yaml
    - examples/rag-agent/mock-responses-judge.yaml
    - examples/rag-agent/providers/mock-judge.yaml
    - examples/rag-agent/scenarios/paris-capital.scenario.yaml
---

Use `examples/rag-agent/` to score a RAG agent's answer against its retrieved context with the six named RAG checks: `faithfulness`, `answer_relevancy`, `contextual_precision`, `contextual_recall`, `contextual_relevancy` and `hallucination`.

## Prerequisites

- The `promptarena` binary.
- The `examples/rag-agent/` directory. The default config needs no API key.

## What it proves

The RAG checks are thin wrappers over `llm_judge` with default prompts adapted from public DeepEval and Ragas references (Apache 2.0). Each check is an eval handler that emits the judge's score. The demo runs all six against one question, one answer and its retrieved context, graded by a mock LLM judge.

Eval handlers emit scores only. Wrap each one in `type: assertion` with a `min_score` to make it gate the scenario. See the [eval framework](/arena/explanation/eval-framework/) for how evals, assertions and guardrails relate.

## The assertion shape

Add one `type: assertion` entry per check under a turn's `assertions`:

```yaml
turns:
  - role: user
    content: "What is the capital of France?"
    assertions:
      - type: assertion
        params:
          eval_type: faithfulness
          eval_params:
            contexts:
              - "Paris is the capital and most populous city of France."
              - "Located on the Seine River in north-central France."
          min_score: 0.8

      - type: assertion
        params:
          eval_type: answer_relevancy
          min_score: 0.8

      - type: assertion
        params:
          eval_type: contextual_precision
          eval_params:
            contexts: [...]
          min_score: 0.5

      # ... contextual_recall, contextual_relevancy, hallucination
```

All six assertions share this shape. The judge is the one configured under `judges:` in `config.arena.yaml`.

Pass the retrieved context inline as `contexts: [...]` (a list of chunks) or as `context: "..."` (a single chunk). `answer_relevancy` takes no context: it scores the answer against the question, which is the `question` param or the last user turn. See the [Checks Reference](https://promptkit.altairalabs.ai/reference/checks/) for every parameter of each check.

## Run it

1. Change into the example and start the web UI:

   ```bash
   cd examples/rag-agent
   promptarena serve
   ```

2. Or run headless, as CI does:

   ```bash
   promptarena run --ci --formats json,markdown
   open out/results.md
   ```

The RAG assistant and the LLM judge are both mock providers. The mock judge returns `{"passed": true, "score": 0.92, "reasoning": "..."}` for every call, so all six assertions pass under the scenario's `min_score` thresholds: `0.8` for most checks and `0.5` for `contextual_precision` and `contextual_relevancy`.

## Swapping in a real judge

1. Create a provider file for the judge:

   ```yaml
   # providers/openai-judge.yaml
   apiVersion: promptkit.altairalabs.ai/v1alpha1
   kind: Provider
   metadata:
     name: openai-judge
   spec:
     id: openai-judge
     type: openai
     model: gpt-4o-mini
   ```

2. In `config.arena.yaml`, replace the mock judge entry under `providers:` with the new file in the `judge` group:

   ```yaml
   providers:
     - file: providers/mock-assistant.yaml
       group: default
     - file: providers/openai-judge.yaml
       group: judge
   ```

3. Point the judge at the new provider:

   ```yaml
   judges:
     - name: rag-judge
       provider: openai-judge
   ```

4. Run with `OPENAI_API_KEY` set. The mock assistant can stay or be swapped too: the assertions grade the answer whichever provider produced it.

The config defines one judge, and Arena uses it for every RAG assertion.

## CI gate

Add a workflow like this to your own repository. The example builds `promptarena` from source; replace that step with however you install the binary.

```yaml
# .github/workflows/rag-agent.yml
name: RAG agent

on:
  pull_request:
    paths:
      - 'examples/rag-agent/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run RAG scenarios
        working-directory: examples/rag-agent
        run: ../../bin/promptarena run --ci --formats json
```

`--ci` exits non-zero when an assertion fails, which is what fails the job. The default config is keyless. Swap the mock judge for a real one to grade real outputs.

## Naming and credit

The handler default prompts are adapted from public DeepEval and Ragas reference implementations (Apache 2.0). Attribution lives in each handler's docstring.

## Related pages

- The [Checks Reference](https://promptkit.altairalabs.ai/reference/checks/) has the full parameter list for each RAG check.
- [Validate Outputs](/arena/how-to/scenarios/validate-outputs/) covers the broader assertion mechanism.
- [Eval framework](/arena/explanation/eval-framework/) explains how evals, assertions and guardrails relate.
