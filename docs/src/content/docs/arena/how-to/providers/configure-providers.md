---
title: Configure LLM Providers
description: Add OpenAI, Anthropic, Gemini, Azure, Ollama, vLLM and gateway providers to an arena, set credentials, and select providers per run.
verified:
  commit: 02ec7c860b978fcce24022d9ccb868d94208c751
  sources:
    - arena/arenaconfig/ignored_fields.go
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - arena/arenaconfig/yamldocs.go
    - arena/assertions/types.go
    - arena/cmd/promptarena/config_inspect_interactive.go
    - arena/cmd/promptarena/init.go
    - arena/cmd/promptarena/run.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/engine/builder_integration.go
    - arena/engine/classify_registry.go
    - arena/engine/eval_orchestrator.go
    - arena/engine/execution.go
    - arena/inspect/render_providers.go
    - arena/templates/builtin/quick-start/template.yaml
    - examples/customer-support/providers/openai-gpt4o-mini.provider.yaml
    - examples/mcp-chatbot/providers/openai-gpt4o-mini.provider.yaml
    - examples/ollama-local/docker-compose.yaml
    - examples/ollama-local/providers/ollama-llama.provider.yaml
    - schemas/v1alpha1/arena.json
    - schemas/v1alpha1/provider.json
---
Add LLM providers to an arena and give each one credentials.

## Prerequisites

- A PromptArena project with a `config.arena.yaml` (`promptarena init` creates one).
- An API key for each hosted provider you add, or a running Ollama or vLLM server for a local one.

## Create a provider with the generator

```bash
promptarena init my-test --quick --provider openai
```

`--provider` accepts `mock`, `openai`, `claude` and `gemini`. Without it, `init` prompts for the provider. The generator writes `providers/<name>.provider.yaml` with `id`, `type` and `model`, and a `.env.example` file that lists the API key variables to export (skip it with `--no-env`).

## Write a provider file

Create one file per provider under `providers/`. Only `spec.type` is required; add `model` in practice.

```yaml
# providers/openai.yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai-gpt54-mini
spec:
  type: openai
  model: gpt-5.4-mini
  capabilities: [text, streaming, vision, tools, json]
  defaults:
    temperature: 0.6
    max_tokens: 2000
```

A provider file holds one YAML document: a file with a second document after a `---` separator fails to load. Put each provider in its own file. The provider id is `spec.id`, or `metadata.name` when `spec.id` is absent.

`capabilities` is a list of free strings. A scenario that sets `required_capabilities` runs only against providers that list all of them, and is skipped when none does. The filter applies to the default and `provider_group` selection, not to a scenario that lists `spec.providers`. See [capabilities](/arena/reference/config-schema/#capabilities) for the values and [Write Scenarios](/arena/how-to/scenarios/write-scenarios/) for the scenario side.

Declare fewer capabilities for limited models:

```yaml
spec:
  type: ollama
  model: llama3.2:3b
  capabilities: [text, streaming, tools, json]  # no vision below 11B
```

## Configure a provider type

### OpenAI

```yaml
spec:
  type: openai
  model: gpt-5.4
  defaults:
    temperature: 0.7
    max_tokens: 4000
```

Credentials come from `OPENAI_API_KEY`. Models include `gpt-6.1-terra`, `gpt-5.5`, `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.4-nano` and the `o3` reasoning model.

### Anthropic Claude

```yaml
spec:
  type: claude
  model: claude-sonnet-5-5
  defaults:
    temperature: 0.6
    max_tokens: 4000
```

Credentials come from `ANTHROPIC_API_KEY`. Models include `claude-opus-5-5`, `claude-sonnet-5-5`, `claude-haiku-5-5` and `claude-fable-5-2`.

### Google Gemini

```yaml
spec:
  type: gemini
  model: gemini-3.8-flash
  defaults:
    temperature: 0.7
    max_tokens: 2000
```

Credentials come from `GEMINI_API_KEY`, or `GOOGLE_API_KEY` when that is unset. Models include `gemini-4-argon`, `gemini-3.8-flash`, `gemini-3.5-flash-lite` and `gemini-3.1-pro-preview`.

### Azure OpenAI

Use `type: openai` with an Azure platform. The provider derives the deployment URL from `platform.endpoint` and `model`:

```yaml
spec:
  type: openai
  model: gpt-5.4
  platform:
    type: azure
    endpoint: https://your-resource.openai.azure.com
    additional_config:
      api_version: "2024-12-01-preview"   # optional, this is the default
```

With `platform.type: azure`, authentication uses the Azure Default Credential chain: Managed Identity, Azure CLI, or `AZURE_TENANT_ID`, `AZURE_CLIENT_ID` and `AZURE_CLIENT_SECRET`. The `credential` fields are ignored. Azure uses the Chat Completions API.

### Ollama (local)

```yaml
spec:
  type: ollama
  model: llama3.2:1b
  base_url: http://localhost:11434
  additional_config:
    keep_alive: "5m"   # keep the model loaded
```

Ollama needs no API key. Start the server and pull the model:

```bash
ollama serve
ollama pull llama3.2:1b
```

Models: any model from `ollama list`.

### vLLM

```yaml
spec:
  type: vllm
  model: meta-llama/Llama-3.2-3B-Instruct
  base_url: http://localhost:8000
  additional_config:
    use_beam_search: false
    best_of: 1
```

vLLM needs no API key. Start the server:

```bash
docker run --rm --gpus all -p 8000:8000 vllm/vllm-openai:latest \
  --model meta-llama/Llama-3.2-3B-Instruct --dtype half --max-model-len 4096
```

Models: any HuggingFace model vLLM supports (see the [vLLM docs](https://docs.vllm.ai/en/latest/models/supported_models.html)).

For constrained output, set one of these under `additional_config`. `guided_json` must be a YAML mapping:

```yaml
additional_config:
  guided_json:
    type: object
    properties:
      sentiment: {type: string, enum: [positive, negative, neutral]}
    required: [sentiment]
  # guided_regex: "^[0-9]{3}-[0-9]{4}$"
  # guided_choice: ["yes", "no", "maybe"]
```

### OpenAI-compatible gateways

OpenRouter, Groq, Together AI, LiteLLM and self-hosted proxies work with `type: openai`, a `base_url` and an optional `headers` map.

`headers` adds HTTP headers to every request the provider sends. A header that collides with a built-in one (`Authorization`, `Content-Type`) fails the request. Use `credential` for secrets.

```yaml
# providers/openrouter.yaml
spec:
  type: openai
  model: anthropic/claude-sonnet-4-20250514
  base_url: https://openrouter.ai/api/v1
  headers:
    HTTP-Referer: https://myapp.com   # recommended by OpenRouter
    X-Title: My App
  credential:
    credential_env: OPENROUTER_API_KEY
```

Other gateways differ only in `base_url`, `model` and the key variable:

```yaml
# providers/groq.yaml
spec:
  type: openai
  model: llama-3.3-70b-versatile
  base_url: https://api.groq.com/openai/v1
  credential:
    credential_env: GROQ_API_KEY
```

```yaml
# providers/together.yaml
spec:
  type: openai
  model: meta-llama/Llama-3.3-70B-Instruct-Turbo
  base_url: https://api.together.xyz/v1
  credential:
    credential_env: TOGETHER_API_KEY
```

```yaml
# providers/litellm.yaml
spec:
  type: openai
  model: gpt-5.4-mini      # whatever LiteLLM routes this name to
  base_url: http://litellm.internal:4000
  headers:
    X-Internal-Auth: shared-gateway-token
```

### Inference providers

Providers with `role: inference` back the classifier and embedding handlers (for example `audio_emotion`) and do not join the LLM run matrix.

```yaml
# providers/hf.yaml
spec:
  type: huggingface
  role: inference
  credential:
    credential_env: HF_TOKEN   # or HUGGING_FACE_HUB_TOKEN
```

Add `base_url` and `additional_config.dedicated: true` for a HuggingFace Inference Endpoint. The client then skips the `/models/{owner}/{name}` path suffix.

Reference the provider in `config.arena.yaml` and name it as the default classifier:

```yaml
spec:
  providers:
    - file: providers/hf.yaml
  defaults:
    inference:
      audio_classifier: hf   # provider id; used when an assertion omits `provider`
      text_classifier: hf
      image_classifier: hf
      video_classifier: hf
      embedder: hf
```

Use it in a scenario by wrapping the handler in `type: assertion`:

```yaml
conversation_assertions:
  - type: assertion
    params:
      eval_type: audio_emotion
      eval_params:
        model: superb/wav2vec2-base-superb-er
        expected_label: angry
      min_score: 0.6
```

A 503 "model is loading" response records the assertion as skipped. See [Inference-backed checks](https://promptkit.altairalabs.ai/reference/checks/#inference-backed-checks).

## Tune streaming and connections

All of these are optional.

```yaml
spec:
  type: openai
  model: gpt-5.4
  request_timeout: "60s"        # non-streaming call timeout
  stream_idle_timeout: "30s"    # abort a silent stream
  stream_max_concurrent: 50     # 0 = unlimited
  stream_retry:
    enabled: true
    max_attempts: 2             # initial request + 1 retry
    retry_window: pre_first_chunk   # or `always`: also retries mid-stream and re-bills tokens
    budget:
      rate_per_sec: 5
      burst: 10
  http_transport:
    max_conns_per_host: 250     # default 0 (unlimited)
    max_idle_conns_per_host: 250   # default 100
    idle_conn_timeout: "90s"
```

The concurrent-stream ceiling is `max_conns_per_host` times the upstream's HTTP/2 concurrent-stream limit. The `promptkit_http_conns_in_use` gauge tracks pool pressure.

## Reference providers from the arena

List each provider file in `config.arena.yaml`. Entries take `file` and an optional `group`. `spec.defaults` is required.

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: multi-provider-arena
spec:
  defaults:
    temperature: 0.7
    max_tokens: 1000
  prompt_configs:
    - id: support
      file: prompts/support.yaml
  providers:
    - file: providers/openai.yaml
    - file: providers/claude.yaml
  scenarios:
    - file: scenarios/customer-support.yaml
```

To compare variants, such as two models or two temperatures, put each in its own provider file with its own `metadata.name` and list both.

## Set credentials

Export keys in your shell or CI:

```bash
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export GOOGLE_API_KEY="..."
```

`promptarena run` does not read `.env` files. Load one first (never commit it):

```bash
export $(cat .env | xargs) && promptarena run
```

In GitHub Actions, map secrets to the same variables:

```yaml
env:
  OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
  ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

## Select providers for a run

```bash
promptarena run --provider openai-gpt54-mini            # one provider
promptarena run --provider openai-gpt54-mini,claude-sonnet   # several
promptarena run                                         # all configured providers
```

To pin a scenario to specific providers, list their ids in the scenario's `spec.providers`, or set `spec.provider_group`. Run flags such as `--max-tokens` are listed in the [CLI reference](/arena/reference/cli-commands/).

## Verify

```bash
promptarena config-inspect
```

The `Providers (n)` section lists each provider's id, type, model and file. Add `--verbose` for detail.

## Troubleshoot

| Symptom | Check |
|---|---|
| Authentication error | `echo $OPENAI_API_KEY` prints a key; run with `--provider <id> --verbose`. |
| Provider not found | `config-inspect --verbose`; the `file` path in `config.arena.yaml` matches the file on disk. |
| Rate limiting | Lower concurrency: `promptarena run --concurrency 2`, or `--concurrency 1` for sequential runs. |

## Related pages

- [Use Mock Providers](/arena/how-to/providers/use-mock-providers/): test without API calls.
- [Validate Outputs](/arena/how-to/scenarios/validate-outputs/): add assertions.
- [Integrate CI/CD](/arena/how-to/interfaces/run-in-ci/): automate testing.
- [Config Reference](/arena/reference/config-schema/): every configuration option.
- Working provider files: `examples/customer-support/providers/`, `examples/mcp-chatbot/providers/`, `examples/ollama-local/providers/`.
