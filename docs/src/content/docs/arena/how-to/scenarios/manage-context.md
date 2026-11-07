---
title: Manage Context
description: Set a token budget and a truncation strategy so long scenario conversations stay within the model's context window.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - arena/engine/conversation_executor.go
    - arena/engine/duplex_executor_assertions_integration.go
    - arena/turnexecutors/context_builder.go
    - arena/turnexecutors/pipeline_stages_integration.go
    - schemas/v1alpha1/scenario.json
---
Configure `context_policy` so a long conversation stays within a token budget.

## Prerequisites

- A scenario with enough turns to exceed the budget. See [Write Scenarios](/arena/how-to/scenarios/write-scenarios/).
- For `relevance` truncation, an API key for an embedding provider (see [Environment Variables](#environment-variables)).

## Basic Configuration

Add `context_policy` to your scenario:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: long-conversation-test

spec:
  task_type: support

  context_policy:
    token_budget: 8000
    strategy: oldest

  turns:
    - role: user
      content: "Start of conversation..."
    # ... many turns ...
```

## Truncation Strategies

`strategy` accepts `oldest`, `summarize`, `relevance` and `fail`. See [`context_policy`](/arena/reference/config-schema/#context_policy) for what each value does. The steps below cover `oldest` and `relevance`.

### Truncate Oldest (Default)

Use `oldest` to drop the oldest messages first when the conversation exceeds the token budget. It is the default:

```yaml
context_policy:
  token_budget: 8000
  strategy: oldest
```

### Relevance-Based Truncation

Use `relevance` if early messages matter to later turns and you accept the extra embedding calls. It keeps the messages most similar to the latest user message:

```yaml
context_policy:
  token_budget: 8000
  strategy: relevance
  relevance:
    provider: openai
    model: text-embedding-3-small
    min_recent_messages: 3
    similarity_threshold: 0.3
```

## Relevance Configuration

### Provider Options

Choose an embedding provider:

```yaml
# OpenAI (recommended for general use)
relevance:
  provider: openai
  model: text-embedding-3-small  # or text-embedding-3-large

# Gemini
relevance:
  provider: gemini
  model: gemini-embedding-001  # or gemini-embedding-2

# Voyage AI (recommended for retrieval tasks)
relevance:
  provider: voyageai
  model: voyage-3.5  # or voyage-code-3 for code
```

### Parameters

See [`context_policy`](/arena/reference/config-schema/#context_policy) in the config schema reference for every field, type and default.

### Example: Code Assistant

For code-related conversations, use a code-optimized model:

```yaml
context_policy:
  token_budget: 16000
  strategy: relevance
  relevance:
    provider: voyageai
    model: voyage-code-3
    min_recent_messages: 5
    similarity_threshold: 0.25
```

### Example: Customer Support

For support conversations, preserve context about the customer's issue:

```yaml
context_policy:
  token_budget: 8000
  strategy: relevance
  relevance:
    provider: openai
    model: text-embedding-3-small
    min_recent_messages: 3
    similarity_threshold: 0.35
    always_keep_system_role: true
```

## How Relevance Truncation Works

The runtime embeds the latest user message as the query. It scores each message by cosine similarity to the query. It keeps system messages when `always_keep_system_role` is `true`, and it keeps the last `min_recent_messages` messages. It drops every other message that scores below `similarity_threshold`, even when budget remains. The rest fill the budget, highest score first, while they fit. See [`context_policy`](/arena/reference/config-schema/#context_policy) for each field.

## Environment Variables

Set API keys for embedding providers:

```bash
# OpenAI
export OPENAI_API_KEY=sk-...

# Gemini
export GEMINI_API_KEY=...

# Voyage AI
export VOYAGE_API_KEY=...
```

## Complete Example

Run this scenario to verify the setup. The last turn asserts that the billing context survives truncation.

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: extended-support-conversation
  labels:
    category: support
    context: managed

spec:
  task_type: support
  description: "Tests context management in long conversations"

  context_policy:
    token_budget: 8000
    strategy: relevance
    relevance:
      provider: openai
      model: text-embedding-3-small
      min_recent_messages: 3
      similarity_threshold: 0.3

  turns:
    - role: user
      content: "I'm having trouble with my account billing"

    - role: user
      content: "The charge appeared on December 15th"

    - role: user
      content: "What's the weather like today?"
      # This unrelated message may be truncated

    - role: user
      content: "Back to my billing issue - can you help?"
      assertions:
        - type: content_includes
          params:
            patterns: ["billing", "charge", "December"]
          message: "Should remember billing context"
```

## Performance Considerations

- Set `cache_embeddings: true` to reuse embeddings within one conversation. See [`context_policy`](/arena/reference/config-schema/#context_policy).
- Use a smaller embedding model, such as `text-embedding-3-small`, for faster calls.
- Set `token_budget` to fit your LLM's context window.

## Troubleshooting

### Missing API Key

```text
OpenAI API key not found: set OPENAI_API_KEY environment variable
```

The log warning `failed to create embedding provider, relevance truncation will fall back to oldest` carries this message. The run then fails on the first over-budget turn with `strategy "relevance" requires RelevanceConfig with EmbeddingProvider; use "oldest" instead`. Set the required environment variable for your embedding provider.

### High Latency

If truncation is slow:
1. Enable embedding caching
2. Use a smaller embedding model
3. Increase `similarity_threshold` to keep fewer messages

### Context Lost

If important context is being truncated:
1. Lower `similarity_threshold`
2. Increase `min_recent_messages`
3. Ensure `always_keep_system_role: true`

## Related pages

- [Write Scenarios](/arena/how-to/scenarios/write-scenarios/)
- [Configure Providers](/arena/how-to/providers/configure-providers/)
- [Configuration Schema](/arena/reference/config-schema/#context_policy)
