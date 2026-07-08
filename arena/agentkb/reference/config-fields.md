# Config Fields

Compact cheat-sheet generated from the embedded schemas. Run `promptarena schema <type>` for the full schema; `promptarena validate` enforces the binary-embedded copy.

## arena

Fields under `spec`:

| field | type | required | description |
|-------|------|----------|-------------|
| `a2a_agents` | array |  | — |
| `agents` | — |  | — |
| `compositions` | — |  | — |
| `defaults` | object | ✓ | — |
| `deploy` | object |  | — |
| `embedding_providers` | array |  | — |
| `eval_specs` | object |  | — |
| `evals` | array |  | — |
| `globals` | object |  | Globals holds arena-level cross-cutting config that applies to every scenario in addition to its own definitions. Distinct from Defaults (which are "values when unspecified") — Globals is for "always-additive" entries. |
| `image_providers` | array |  | — |
| `judge_defaults` | object |  | — |
| `judge_specs` | object |  | — |
| `judges` | array |  | — |
| `mcp_servers` | array |  | — |
| `memory` | — |  | — |
| `pack_evals` | array |  | — |
| `pack_file` | string |  | — |
| `prompt_configs` | array |  | File references for YAML serialization |
| `prompt_specs` | object |  | — |
| `provider_specs` | object |  | Inline resource specs (alternative to file refs, merged into LoadedX during load) |
| `providers` | array | ✓ | — |
| `runtime` | object |  | Runtime carries a runtime configuration spec passed straight through to the runtime layer (hooks, sandboxes, …). Arena wraps the runtime, so anything the runtime config supports is available here under `runtime:` without Arena needing a bespoke field for each. |
| `scenario_specs` | object |  | — |
| `scenarios` | array |  | — |
| `self_play` | object |  | — |
| `skills` | array |  | — |
| `state_store` | object |  | — |
| `stt_providers` | array |  | — |
| `tool_specs` | object |  | — |
| `tools` | array |  | — |
| `tts_providers` | array |  | TTSProviders / STTProviders / EmbeddingProviders / ImageProviders are the legacy role-specific slots. They still load correctly — every entry's `role:` is validated against the slot — but the preferred shape is a single unified `providers:` list where the loader routes each provider into the right Loaded* map based on its `role:` value. Mixing the legacy slots and the unified list is supported during migration; both populate the same Loaded* maps. |
| `voices` | array |  | Voices binds voice IDs to loaded TTS provider IDs. Personas reference voice IDs (not provider IDs) so the same persona can run against a real Cartesia voice in recording mode and a mock TTS provider in CI just by editing this list. |
| `workflow` | — |  | — |

## eval

Fields under `spec`:

| field | type | required | description |
|-------|------|----------|-------------|
| `conversation_assertions` | array |  | Conversation-level assertions |
| `description` | string | ✓ | Human-readable description |
| `id` | string |  | Unique identifier for this evaluation |
| `mode` | string |  | Replay timing mode (instant |
| `recording` | object | ✓ | Recording source |
| `speed` | number |  | Playback speed (default 1.0) |
| `tags` | array |  | Tags for categorization |
| `turns` | array |  | Turn-level assertions |

## logging

Fields under `spec`:

| field | type | required | description |
|-------|------|----------|-------------|
| `commonFields` | object |  | Key-value pairs added to every log entry |
| `defaultLevel` | string |  | Default log level for all modules |
| `format` | string |  | Log output format |
| `modules` | array |  | Per-module logging configuration |

## persona

Fields under `spec`:

| field | type | required | description |
|-------|------|----------|-------------|
| `constraints` | array | ✓ | — |
| `defaults` | object |  | — |
| `description` | string | ✓ | — |
| `fragments` | array |  | NEW: Template system (preferred) |
| `goals` | array | ✓ | — |
| `id` | string |  | — |
| `optional_vars` | object |  | Variables with default values |
| `prompt_activity` | string |  | DEPRECATED: Legacy prompt builder reference |
| `required_vars` | array |  | Variables that must be provided |
| `style` | object |  | — |
| `system_prompt` | string |  | LEGACY: Backward compatibility |
| `system_template` | string |  | Template with {{variables}} |
| `voice` | string |  | Voice references an arena-level voice id (see Config.Voices). When set, selfplay synthesis routes this persona's text through the bound TTS provider. Empty means the arena falls back to its default voice or fails fast if no default is configured. |

## promptconfig

Fields under `spec`:

| field | type | required | description |
|-------|------|----------|-------------|
| `allowed_tools` | array |  | — |
| `compilation` | object |  | — |
| `description` | string | ✓ | — |
| `evals` | array |  | — |
| `fragments` | array |  | — |
| `media` | object |  | — |
| `metadata` | object |  | — |
| `model_overrides` | object |  | — |
| `parameters` | object |  | — |
| `system_template` | string | ✓ | — |
| `task_type` | string | ✓ | — |
| `template_engine` | object |  | — |
| `tested_models` | array |  | — |
| `tool_policy` | object |  | — |
| `validators` | array |  | — |
| `variables` | array |  | — |
| `version` | string | ✓ | — |

## provider

Fields under `spec`:

| field | type | required | description |
|-------|------|----------|-------------|
| `additional_config` | object |  | — |
| `audio_files` | array |  | — |
| `base_url` | string |  | — |
| `capabilities` | array |  | — |
| `credential` | object |  | — |
| `defaults` | object |  | — |
| `headers` | object |  | — |
| `http_transport` | object |  | — |
| `id` | string |  | — |
| `include_raw_output` | boolean |  | — |
| `model` | string |  | — |
| `platform` | object |  | — |
| `pricing` | object |  | — |
| `pricing_correct_at` | string |  | — |
| `rate_limit` | object |  | — |
| `request_timeout` | string |  | — |
| `role` | string |  | — |
| `sample_rate` | integer |  | — |
| `stream_idle_timeout` | string |  | — |
| `stream_max_concurrent` | integer |  | — |
| `stream_retry` | object |  | — |
| `type` | string | ✓ | — |
| `unsupported_params` | array |  | — |
| `voice` | string |  | — |

## runtime-config

Fields under `spec`:

| field | type | required | description |
|-------|------|----------|-------------|
| `embedding_providers` | array |  | Embedding provider configurations |
| `evals` | object |  | External eval process bindings keyed by eval type name |
| `hooks` | object |  | External hook process configurations |
| `inference_providers` | array |  | Inference (classify) provider configurations |
| `logging` | object |  | Logging configuration |
| `mcp_servers` | array |  | MCP server configurations |
| `providers` | array |  | LLM provider configurations |
| `sandboxes` | object |  | Named sandbox backends for exec-hook subprocess launch |
| `selectors` | object |  | External selector processes narrowing skill and tool candidate sets |
| `skills` | object |  | Runtime skill configuration |
| `state_store` | object |  | Conversation state persistence configuration |
| `stt_providers` | array |  | Speech-to-text provider configurations |
| `tool_selector` | string |  | Name of a selector declared under spec.selectors used to narrow the LLM-visible tool set per turn |
| `tools` | object |  | Tool implementation bindings keyed by tool name |
| `tts_providers` | array |  | Text-to-speech provider configurations |

## scenario

Fields under `spec`:

| field | type | required | description |
|-------|------|----------|-------------|
| `constraints` | object |  | — |
| `context` | object |  | — |
| `context_metadata` | object |  | — |
| `context_policy` | object |  | Context management policy for long conversations. |
| `conversation_assertions` | array |  | Assertions evaluated after the entire conversation completes. |
| `description` | string | ✓ | — |
| `duplex` | object |  | Duplex enables bidirectional streaming mode for voice/audio scenarios. |
| `id` | string |  | — |
| `labels` | object |  | Labels are key/value tags copied from the scenario manifest's metadata.labels (K8s-style). Used for stratified reporting in arena. Populated by LoadScenario; not read from the spec body itself. |
| `mode` | string |  | — |
| `provider_group` | string |  | — |
| `providers` | array |  | ProvidersOverride: If empty, uses all arena providers. |
| `required_capabilities` | array |  | RequiredCapabilities filters providers to only those supporting all listed capabilities. Valid values: text, streaming, vision, tools, json, audio, video, documents |
| `seed_memories` | array |  | SeedMemories pre-populates the memory store before the first turn. Uses the same fields as memory__remember: content (required), type, confidence, metadata. |
| `streaming` | boolean |  | Enable streaming for all turns by default. |
| `task_type` | string |  | — |
| `tool_policy` | object |  | — |
| `trials` | integer |  | Number of times to run this scenario for statistical evaluation |
| `turns` | array |  | — |
| `variables` | object |  | Template variables to inject into the pack |
| `voice` | string |  | Voice references an arena-level voice id (see Config.Voices) used to synthesize this scenario's scripted-text user turns. The duplex executor resolves the voice via Config.ResolveVoice.  For selfplay scenarios (turns with role: selfplay-user) the persona owns voice choice; Scenario.Voice is ignored. |

## tool

Fields under `spec`:

| field | type | required | description |
|-------|------|----------|-------------|
| `client` | object |  | — |
| `description` | string | ✓ | — |
| `exec` | object |  | — |
| `http` | object |  | — |
| `input_schema` | — | ✓ | — |
| `mock_parts` | array |  | — |
| `mock_result` | — |  | Static mock response returned regardless of tool-call args. Use when the response does not depend on inputs. Mutually exclusive with mock_template. |
| `mock_template` | string |  | Go text/template rendered against tool-call args (parsed as a JSON map). Rendered output is parsed back as JSON. Use this instead of mock_result when the response should depend on inputs (e.g. branching on order_id with {{ if eq .order_id "X" }}...{{ end }}). Mutually exclusive with mock_result. |
| `mode` | string | ✓ | Execution mode. One of: 'mock' (use mock_result or mock_template) - 'live' (HTTP via 'http') - 'mcp' (MCP server) - 'exec' (subprocess) - 'client' (client-side handler). |
| `name` | string |  | — |
| `output_schema` | — | ✓ | — |
| `timeout_ms` | integer |  | — |
