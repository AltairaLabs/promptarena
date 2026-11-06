---
title: Test Agent Negotiation
description: Test a multi-turn negotiation with conversation-level assertions, scripted for CI or driven by a real LLM in self-play.
verified:
  commit: 631d3c353c2c9d71b362ae5cd20c137d42b19d5b
  sources:
    - .github/workflows/ci.yml
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/persona.go
    - arena/arenaconfig/types.go
    - arena/assertions/eval_adapter.go
    - arena/assertions/types.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/cmd/promptarena/serve_interactive.go
    - arena/engine/conversation_executor.go
    - arena/engine/eval_orchestrator.go
    - arena/engine/execution.go
    - arena/selfplay/registry.go
    - examples/adaptive-persona/config.arena.yaml
    - examples/assertions-test/providers/openai.provider.yaml
    - examples/text-negotiation/config.arena.yaml
    - examples/text-negotiation/mock-responses.yaml
    - examples/text-negotiation/prompts/landlord-agent.yaml
    - examples/text-negotiation/providers/mock-provider.yaml
    - examples/text-negotiation/scenarios/rental-negotiation.scenario.yaml
    - schemas/v1alpha1/provider.json
    - schemas/v1alpha1/scenario.json
    - scripts/test-examples.sh
---

Use `examples/text-negotiation/` to test a four-turn rental-price negotiation: the default config runs deterministically against a mock landlord, and the self-play steps swap in an LLM-driven tenant.

## Prerequisites

- The `promptarena` binary.
- The `examples/text-negotiation/` directory. The default config needs no API key.
- For self-play: an `OPENAI_API_KEY`.

## What it checks

A negotiation fails on its trajectory: the agent caves under pressure, drops the term commitment, or ends below its reservation price. The scenario scripts the tenant's turns, and `conversation_assertions` check the deal that lands. See [Scenario design](/arena/explanation/scenario-design/) and [Testing philosophy](/arena/explanation/testing-philosophy/) for why these checks sit at conversation level.

## Run it

1. Change into the example and start the web UI on port 8080 (add `--open` to launch the browser):

   ```bash
   cd examples/text-negotiation
   promptarena serve
   ```

2. Or run in the terminal. The TUI is the default and suits the dev loop:

   ```bash
   promptarena run
   ```

3. Or run headless, as CI does:

   ```bash
   promptarena run --ci --formats json,markdown
   open out/results.md
   ```

The landlord is a mock provider with scripted responses, so no key is needed.

## Add the assertions

The scenario's `conversation_assertions` check the end state:

```yaml
conversation_assertions:
  - type: content_includes
    params:
      patterns: ["twenty-four fifty"]
    message: "Final deal must land at the negotiated price"

  - type: content_includes
    params:
      patterns: ["twelve-month"]
    message: "Final deal must include the 12-month commitment"

  - type: content_excludes
    params:
      patterns: ["I accept twenty-three hundred", "twenty-three hundred works"]
    message: "Landlord must not capitulate on the below-reservation offer"
```

Per-turn checks tend to over-constrain a negotiation, so all three run once over the whole conversation.

For a stricter contract, add `llm_judge_session` with criteria over the full conversation:

```yaml
- type: assertion
  params:
    eval_type: llm_judge_session
    eval_params:
      criteria: |
        Did the landlord maintain a professional negotiation posture
        across all turns, avoiding capitulation while reaching an
        acceptable deal? Score 1.0 if yes.
    min_score: 0.8
```

This assertion needs a judge provider. Define one under `judges:` in `config.arena.yaml`; [Test RAG Agents](/arena/how-to/agents/rag-agent/) shows the setup. The [assertions reference](/arena/reference/assertions/) lists the assertion types, and the [Checks Reference](https://promptkit.altairalabs.ai/reference/checks/) lists the judge params.

## Switch to real self-play

The default scenario uses scripted user turns. These steps hand the tenant side to an LLM.

1. Create `personas/savvy-tenant.persona.yaml`:

   ```yaml
   apiVersion: promptkit.altairalabs.ai/v1alpha1
   kind: Persona
   metadata:
     name: savvy-tenant
   spec:
     id: savvy-tenant
     description: "Cost-conscious tenant negotiating rent."
     system_template: |
       You are a tenant negotiating monthly rent. Target $2300/month
       with a 12-month lease. Be polite but firm; don't accept above
       $2500. If the landlord won't budge below $2500, walk away.
   ```

2. Create `providers/openai-gpt4o-mini-text.provider.yaml`:

   ```yaml
   apiVersion: promptkit.altairalabs.ai/v1alpha1
   kind: Provider
   metadata:
     name: openai-gpt4o-mini-text
   spec:
     id: openai-gpt4o-mini-text
     type: openai
     model: gpt-4o-mini
   ```

3. In `config.arena.yaml`, list the provider in the `selfplay` group, then add a `self_play` block that loads the persona and declares the `selfplay-user` role:

   ```yaml
   spec:
     providers:
       - file: providers/mock-provider.yaml
       - file: providers/openai-gpt4o-mini-text.provider.yaml
         group: selfplay

     self_play:
       personas:
         - file: personas/savvy-tenant.persona.yaml
       roles:
         - id: selfplay-user
           provider: openai-gpt4o-mini-text
   ```

   Arena loads personas only from `self_play.personas`. It does not scan the `personas/` directory.

4. In the scenario, replace the scripted user turns with a `selfplay-user` turn:

   ```yaml
   turns:
     - role: selfplay-user
       persona: savvy-tenant
       turns: 4
   ```

5. Run with `OPENAI_API_KEY` set:

   ```bash
   promptarena run --ci --formats json,markdown
   ```

The assertions stay the same. The persona LLM generates each tenant turn and the landlord responds. The report shows every persona-driven turn, which helps when the persona behaves unexpectedly.

## Run it in CI

The repository runs this example in CI through `make test-examples`, which calls `scripts/test-examples.sh`. To gate your own repository, adapt this sample workflow. It builds `promptarena` from source; replace that step with however you install the binary.

```yaml
# .github/workflows/text-negotiation.yml (sample)
name: Text negotiation

on:
  pull_request:
    paths:
      - 'examples/text-negotiation/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go build -o bin/promptarena ./arena/cmd/promptarena
      - name: Run text-negotiation
        working-directory: examples/text-negotiation
        run: ../../bin/promptarena run --ci --formats json
```

With the default scripted config the job needs no secrets, so it runs on fork pull requests.

## Extend it

- Reservation-price assertions: parse the final agreed amount and assert numerically. `tool_calls_with_args` works well if the agent ends the negotiation with a `record_agreement(amount: ...)` tool call.
- Walk-away scenarios: script a tenant offer of $2000 and assert the landlord ends the conversation. Write mock responses with walk-away wording, then match it with `content_includes`.
- Adversarial tenants: add persona variants (aggressive, indecisive, deadline-pressured) to `self_play.personas` and point a `selfplay-user` turn at each. The landlord stays the same, so the outcome profiles differ by persona.

## Related pages

- [Test RAG Agents](/arena/how-to/agents/rag-agent/) sets up a judge provider.
- [Scenario design](/arena/explanation/scenario-design/) covers multi-turn scenario structure.
- [Assertions reference](/arena/reference/assertions/) lists every assertion type.
