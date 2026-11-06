---
title: 'Tutorial 3: Multi-Turn Conversations'
description: Test that an assistant keeps context across a conversation, walk it through a multi-step flow, and let a persona play the customer.
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/run.go
    - arena/engine/conversation_executor.go
    - arena/engine/execution.go
    - arena/stages/scenario_context_extraction.go
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/scenario.json
---
In this tutorial we test conversations rather than single questions. We write a support assistant, check that it remembers a name across five turns, check that it walks a customer through a booking, and finally let an AI persona play the customer.

## What we will build

A project with one prompt, one provider and three scenarios, all passing against OpenAI's `gpt-4o-mini`:

```text
my-arena-tests/
├── config.arena.yaml
├── prompts/support.yaml
├── providers/openai.yaml
├── personas/frustrated-customer.persona.yaml
└── scenarios/
    ├── memory-test.yaml
    ├── flight-booking.yaml
    └── self-play.yaml
```

## Prerequisites

- PromptArena installed (see [Tutorial 1](/arena/tutorials/01-first-test/))
- An OpenAI API key

## Step 1: Create the project

```bash
mkdir -p my-arena-tests/prompts my-arena-tests/providers my-arena-tests/personas my-arena-tests/scenarios
cd my-arena-tests
```

## Step 2: Create a prompt and a provider

Create `prompts/support.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: support

spec:
  task_type: support
  version: v1.0.0
  description: Support assistant

  system_template: |
    You are a customer support assistant. Be concise and helpful.
```

Create `providers/openai.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai-gpt4o-mini

spec:
  id: openai-gpt4o-mini
  type: openai
  model: gpt-4o-mini
```

`promptarena` does not read `.env` files, so export your key in the shell you run it from:

```bash
export OPENAI_API_KEY="sk-your-api-key-here"
```

## Step 3: Test that the assistant remembers

Every `role: user` turn in a scenario is sent with the whole conversation so far, so a later turn can depend on an earlier one. We test that by telling the assistant our name in turn 1, changing the subject three times, and asking for the name back in turn 5.

Create `scenarios/memory-test.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: long-term-memory-test

spec:
  id: long-term-memory-test
  description: Remembers the customer's name after several unrelated questions
  task_type: support

  turns:
    - role: user
      content: "Hi, my name is Alice and I'm calling about my account"
      assertions:
        - type: content_includes
          params:
            patterns: ["Alice"]
          message: "Should acknowledge the name"

    - role: user
      content: "What are your business hours?"
    - role: user
      content: "Do you offer international shipping?"
    - role: user
      content: "What's your return policy?"

    - role: user
      content: "What was my name again?"
      assertions:
        - type: content_includes
          params:
            patterns: ["Alice"]
          message: "Should remember the name from turn 1"
```

Turns without `assertions` still run, so they pad the conversation. Only the first and last turns are checked here.

## Step 4: Create the arena configuration

Create `config.arena.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: my-arena-tests

spec:
  prompt_configs:
    - id: support
      file: prompts/support.yaml

  providers:
    - file: providers/openai.yaml

  scenarios:
    - file: scenarios/memory-test.yaml

  defaults:
    temperature: 0.7
    max_tokens: 500
    concurrency: 1
    output:
      dir: out
      formats: [json, markdown]
```

Arena only runs scenarios listed under `spec.scenarios`; it does not scan the `scenarios/` directory.

## Step 5: Run it

`--scenario` takes the scenario `id`, not the file name:

```bash
promptarena run --simple --scenario long-term-memory-test
```

`--simple` prints plain output instead of the interactive terminal UI. After the log lines you see the summary:

```text
Run Summary
────────────────────────────────────────────────────

Total runs      1
Passed          1 (100.0%)
Failed          0 (0.0%)
```

Cost, tokens and duration follow and vary from run to run.

## Step 6: Read the conversation

Each run writes a JSON file to `out/`, named after the scenario. Its `Messages` array holds every user and assistant message, and each assistant message carries its assertion results in `meta.assertions.results`:

```bash
jq -c '.Messages[] | select(.role == "assistant") | {content, results: [.meta.assertions.results[]? | {type, passed, message}]}' out/*_long-term-memory-test_*.json
```

The last line is the answer to "What was my name again?", with its passing check:

```json
{"content":"Your name is Alice. How can I assist you further?","results":[{"type":"content_includes","passed":true,"message":"Should remember the name from turn 1"}]}
```

The reply wording varies from run to run. Replies to turns without assertions show `"results":[]`. When an assertion fails, Arena stops that scenario and skips its remaining turns.

## Step 7: Walk a multi-step flow

Here the customer supplies information one piece at a time, and we check that the assistant asks for what is missing. Real model replies are worded differently each run, so each assertion accepts several phrasings with a regular expression.

Create `scenarios/flight-booking.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: flight-booking

spec:
  id: flight-booking
  description: Collects flight details step by step
  task_type: support

  turns:
    - role: user
      content: "I need to book a flight"
      assertions:
        - type: content_matches
          params:
            pattern: "(?i)(destination|where|from|depart)"
          message: "Should ask where the trip is"

    - role: user
      content: "To New York"
      assertions:
        - type: content_matches
          params:
            pattern: "(?i)(date|when|depart|travel)"
          message: "Should ask about timing"

    - role: user
      content: "Next Friday"
      assertions:
        - type: content_matches
          params:
            pattern: "(?i)(class|economy|return|round|passenger|how many)"
          message: "Should ask for more details"
```

Add it to `config.arena.yaml`:

```yaml
  scenarios:
    - file: scenarios/memory-test.yaml
    - file: scenarios/flight-booking.yaml
```

Run both scenarios, naming them by id:

```bash
promptarena run --simple --scenario long-term-memory-test,flight-booking
```

The summary now reads `Total runs      2` and `Passed          2 (100.0%)`.

## Step 8: Let a persona play the customer

Scripted turns test the paths you thought of. A self-play turn hands the user's side to a second model that follows a persona, so the conversation goes where the persona takes it.

Create `personas/frustrated-customer.persona.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Persona
metadata:
  name: frustrated-customer

spec:
  id: frustrated-customer
  description: A customer whose order has not arrived
  system_template: |
    You are a frustrated customer whose order has not arrived. Write one short message at a time.
  goals: [Get the order located]
  constraints: [Stay in character]
  style:
    verbosity: short
    challenge_level: medium
    friction_tags: [impatient]
  defaults:
    temperature: 0.7
```

Create `scenarios/self-play.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: self-play-customer-interaction

spec:
  id: self-play-customer-interaction
  description: A persona keeps a late-order conversation going
  task_type: support

  turns:
    # A scripted opening
    - role: user
      content: "My order hasn't arrived and it's been a week."

    # The persona writes the next user messages
    - role: customer
      persona: frustrated-customer
      turns: 3
```

`role: customer` names a self-play role, which we define in `config.arena.yaml`. Its `provider` is the id of the model that plays the persona, and it must be one of the providers listed under `spec.providers`. Create `config.arena.yaml` again, now with the self-play block and all three scenarios:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: my-arena-tests

spec:
  prompt_configs:
    - id: support
      file: prompts/support.yaml

  providers:
    - file: providers/openai.yaml

  scenarios:
    - file: scenarios/memory-test.yaml
    - file: scenarios/flight-booking.yaml
    - file: scenarios/self-play.yaml

  self_play:
    personas:
      - file: personas/frustrated-customer.persona.yaml
    roles:
      - id: customer
        provider: openai-gpt4o-mini

  defaults:
    temperature: 0.7
    max_tokens: 500
    concurrency: 1
    output:
      dir: out
      formats: [json, markdown]
```

Run the whole suite:

```bash
promptarena run --simple
```

All three scenarios pass. Each self-play run writes different messages, so read the one it produced:

```bash
jq -c '.Messages[] | {role, content}' out/*_self-play-customer-interaction_*.json
```

After the system prompt and the scripted opening, the persona's messages appear as `user` entries, for example `My order number is 123456. Can you tell me where it is?`.

## Common issues

### `scenario <name> not found`

`--scenario` takes the scenario `id` (or `metadata.name` when there is no `id`), and the scenario's file must be listed under `spec.scenarios`.

### `spec: description is required`

Every scenario needs a `spec.description`.

### An assertion fails and later turns never run

Real replies vary. Match on a short word or a regular expression with alternatives, not a full sentence, as in Step 7.

## Next steps

You can now test context retention, guided flows and persona-driven conversations.

- [Tutorial 4: MCP Tools](/arena/tutorials/04-mcp-tools/): test tool calling inside conversations.
- [Tutorial 5: CI Integration](/arena/tutorials/05-ci-integration/): run these tests automatically.
- [Write Scenarios](/arena/how-to/scenarios/write-scenarios/): more scenario patterns.
- [Scenario format](/arena/reference/scenario-format/): every scenario field.
