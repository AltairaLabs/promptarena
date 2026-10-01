# Config Fields

Compact cheat-sheet generated from the embedded schemas. Run `promptarena schema <type>` for the full schema; `promptarena validate` enforces the binary-embedded copy.

## arena

Fields under `spec`:

| field | type | required | description |
|-------|------|----------|-------------|
| `a2a_agents` | array |  | A2AAgents configures agent-to-agent (A2A) endpoints. |
| `agents` | object |  | Agents configures named agents: the entry agent and each member's description, tags and delegation rules. |
| `compositions` | object |  | Compositions declares named composition step graphs (RFC 0010), keyed by name, that a workflow state with `orchestration: composition` runs. |
| `defaults` | object | ✓ | Defaults holds global defaults applied when a scenario does not specify its own (temperature, max_tokens, concurrency, output, fail_on, …). |
| `deploy` | object |  | Deploy configures `promptarena deploy`: the target provider plus base and per-environment adapter config. |
| `embedding_providers` | array |  | — |
| `eval_specs` | object |  | — |
| `evals` | array |  | Evals references saved-conversation evaluation files. |
| `globals` | object |  | Globals holds arena-level cross-cutting config that applies to every scenario in addition to its own definitions. Distinct from Defaults (which are "values when unspecified") — Globals is for "always-additive" entries. |
| `image_providers` | array |  | — |
| `judge_defaults` | object |  | JudgeDefaults sets the default judge prompt and prompt registry used by LLM-as-judge assertions. |
| `judge_specs` | object |  | — |
| `judges` | array |  | Judges maps a judge name to a provider for LLM-as-judge assertions. |
| `mcp_servers` | array |  | MCPServers configures MCP (Model Context Protocol) servers whose tools the LLM can call. |
| `memory` | — |  | Memory configures the memory capability (auto-registers the memory tools). |
| `pack_evals` | array |  | PackEvals lists pack-level eval definitions. |
| `pack_file` | string |  | PackFile is the path to a pre-compiled pack (*.pack.json) to deploy instead of compiling from this config. |
| `pack_metadata` | object |  | PackMetadata describes the pack this config compiles to: domain, language, tags, cost_estimate and governance. |
| `prompt_configs` | array |  | PromptConfigs references prompt configuration files, each binding an id to a PromptConfig file (with optional per-file variable overrides). |
| `prompt_specs` | object |  | — |
| `provider_specs` | object |  | Inline resource specs (alternative to file refs, merged into LoadedX during load) |
| `providers` | array | ✓ | Providers lists the LLM provider configurations (file references); the loader routes each into the right role slot based on its role. |
| `runtime` | object |  | Runtime carries a runtime configuration spec passed straight through to the runtime layer (hooks, sandboxes, …). Arena wraps the runtime, so anything the runtime config supports is available here under `runtime:` without Arena needing a bespoke field for each. |
| `scenario_specs` | object |  | — |
| `scenarios` | array |  | Scenarios references the test scenario files to run. |
| `self_play` | object |  | SelfPlay configures self-play: personas and roles that drive the user side of a conversation. |
| `skills` | array |  | Skills lists skill sources made available to the run. |
| `state_store` | object |  | StateStore configures conversation state persistence (memory, redis, or file). |
| `stt_providers` | array |  | — |
| `tool_specs` | object |  | — |
| `tools` | array |  | Tools references tool (function) definition files the LLM may call. |
| `tts_providers` | array |  | TTSProviders / STTProviders / EmbeddingProviders / ImageProviders are the legacy role-specific slots. They still load correctly — every entry's `role:` is validated against the slot — but the preferred shape is a single unified `providers:` list where the loader routes each provider into the right Loaded* map based on its `role:` value. Mixing the legacy slots and the unified list is supported during migration; both populate the same Loaded* maps. |
| `voices` | array |  | Voices binds voice IDs to loaded TTS provider IDs. Personas reference voice IDs (not provider IDs) so the same persona can run against a real Cartesia voice in recording mode and a mock TTS provider in CI just by editing this list. |
| `workflow` | object |  | Workflow configures a workflow state machine (auto-registers the workflow tool): the entry state, each state's prompt or composition, its event transitions, turn control, visit limits and artifacts. |

### `spec.workflow`

| field | type | required | description |
|-------|------|----------|-------------|
| `engine` | object |  | Optional runtime engine configuration for workflow execution. Hosts standardized fields like 'budget' for resource limits, alongside runtime-specific hints (timeout, concurrency, etc.). |
| `entry` | string | ✓ | Name of the initial state. Must match a key in the states object. |
| `states` | object | ✓ | Map of state name to state definition. Each state references a prompt and declares transitions. |
| `version` | integer | ✓ | Workflow schema version. Use 1 for the current stable format. |

### `spec.workflow.engine.budget`

| field | type | required | description |
|-------|------|----------|-------------|
| `max_tool_calls` | integer |  | Maximum total tool calls across all states in the workflow. |
| `max_total_visits` | integer |  | Maximum total state visits across all states in the workflow. This is a global safety net independent of per-state max_visits. |
| `max_wall_time_sec` | integer |  | Maximum wall-clock time in seconds for the entire workflow execution. |

### `spec.workflow.states.<state>`

| field | type | required | description |
|-------|------|----------|-------------|
| `artifacts` | object |  | Named artifact slots for lightweight, structured metadata that flows across state visits. Artifacts should be pointers (commit SHAs, URIs), compact representations (schemas, summaries, diffs), or small structured results — not bulk data. Artifact values are available to the prompt as template variables under the 'artifacts' namespace (e.g., {{artifacts.commit_sha}}). |
| `composition` | string |  | Reference to a composition key defined in the pack's compositions object (RFC 0010). Required when orchestration is 'composition'; absent otherwise. |
| `control` | string |  | Who holds the next turn after entering this state (RFC 0014). 'user' yields the conversation to the user (default, and the behavior of every state before v1.7.0). 'agent' runs another agent round in this state without yielding, for transient routing or processing states. Orthogonal to 'orchestration', which declares who initiates a transition rather than who holds the turn after one; inert on states reached via 'external' orchestration. Bounded by terminal states, max_visits and the workflow budget — it introduces no new limits. |
| `description` | string |  | Human-readable description of this state's purpose. |
| `max_visits` | integer |  | Maximum number of times this state can be entered during a single workflow execution. When the limit is reached, the workflow transitions to the state named in on_max_visits. If on_max_visits is not set, the workflow terminates. |
| `on_event` | object |  | Map of event name to target state name. When the named event fires, the workflow transitions to the target state. |
| `on_max_visits` | string |  | Target state to transition to when max_visits is reached. Must reference a key in the states object. If omitted and max_visits is reached, the workflow terminates with a budget-exhausted status. |
| `orchestration` | string |  | How the state is orchestrated. 'internal' = agent controls transitions (default). 'external' = system controls transitions. 'hybrid' = both. 'composition' = the referenced composition fully handles the state's orchestration (work + transitions): the composition runs end-to-end, and on completion its output may map to on_event transitions or terminate the state. The composition mode is exclusive; it is not mixed with internal/external/hybrid on the same state. |
| `persistence` | string |  | Whether conversation context is kept (persistent) or reset (transient) on entry. |
| `prompt_task` | string |  | Reference to a prompt key defined in the pack's prompts object. Required for orchestration modes 'internal', 'external', 'hybrid' (or when orchestration is omitted, default 'internal'); not used in 'composition' mode. |
| `skills` | string |  | Skill filter for this workflow state. A path to a skill directory/file that scopes which skills are available in this state, or the literal 'none' to disable skills. |
| `terminal` | boolean |  | If true, this state is a terminal state. The workflow completes after this state's prompt executes. Terminal states should not declare on_event transitions. |

### `spec.workflow.states.<state>.artifacts.<name>`

| field | type | required | description |
|-------|------|----------|-------------|
| `description` | string |  | Human-readable description of what this artifact contains and how it's used. |
| `mode` | string |  | How the artifact is updated across visits. 'replace' overwrites the previous value on each visit. 'append' accumulates content across visits (e.g., a log). Defaults to 'replace'. |
| `type` | string | ✓ | MIME type indicating the artifact's content type. Used by runtimes to determine serialization and presentation. |

### `spec.compositions.<name>`

| field | type | required | description |
|-------|------|----------|-------------|
| `description` | string |  | Human-readable description of what this composition does. |
| `engine` | object |  | Runtime-specific configuration (e.g. budgets, telemetry, scheduling hints). Opaque escape hatch with no schema enforcement. |
| `input_schema` | string |  | Reference to a JSON Schema declaring the structured input shape. Path or fragment reference. |
| `output` | string |  | Step ID whose output is the composition's output. If omitted, runtimes should treat the last step's output as the composition output. |
| `output_schema` | string |  | Reference to a JSON Schema declaring the structured output shape. |
| `steps` | array | ✓ | Ordered array of step definitions. Order is logical; control flow is determined by the steps themselves (sequential by default; branches and parallels alter flow). |
| `version` | integer | ✓ | Composition format version. Currently 1. |

### `spec.compositions.<name>.steps[]`

| field | type | required | description |
|-------|------|----------|-------------|
| `args` | object |  | Argument bindings. Variables resolved against the composition's input and prior steps' outputs. |
| `branches` | array |  | Parallel steps only: the steps to run concurrently (at least two). |
| `depends_on` | array |  | Optional explicit predecessor step IDs. If omitted, the step sequentially follows the prior step in steps[]. Required when steps run after a branch or parallel and need to declare a join point. |
| `description` | string |  | Human-readable description of what this step does. |
| `else` | string |  | Step ID to execute when the predicate evaluates false. |
| `id` | string | ✓ | Stable identifier for this step. Must be unique within the composition. Used for output references, eval attachment, and trace records. |
| `input` | object |  | Optional input binding. Variables resolved against the composition's input and prior steps' outputs. |
| `kind` | — | ✓ | Step kind. The v1 kinds are suggested; a runtime may support additional vendor-namespaced kinds. |
| `modifiers` | object |  | Optional declarative modifiers (retry, eval attachment). Modifier semantics are runtime-defined. |
| `output_schema` | string |  | Reference to a JSON Schema for the expected output shape. Runtimes parse the LLM response against this schema. |
| `predicate` | object |  | Branch steps only: the condition that selects then or else. |
| `prompt_task` | string |  | Reference to a prompt key defined in the pack's prompts object. |
| `reduce` | object |  | Parallel steps only: how the branch outputs are merged. |
| `termination` | object |  | REQUIRED. The condition under which the bounded loop exits. Without an explicit termination predicate, an agent step is invalid. |
| `then` | string |  | Step ID to execute when the predicate evaluates true. |
| `tool` | string |  | Reference to a tool key defined in the pack's tools object. |
| `tools` | array |  | Subset of the pack's tools available to this agent step. Acts as a per-step scoped tool registry. |

### `steps[].termination`

| field | type | required | description |
|-------|------|----------|-------------|
| `max_steps` | integer |  | Maximum number of LLM rounds the agent step's tool loop may run before it exits. |
| `tool_called` | string |  | Tool name; agent terminates when the LLM successfully invokes this tool. |

### `steps[].predicate`

| field | type | required | description |
|-------|------|----------|-------------|
| `all_of` | array |  | Composite predicate: true when every listed predicate is true. |
| `any_of` | array |  | Composite predicate: true when at least one listed predicate is true. |
| `exists` | boolean |  | For an exists predicate: true if path must resolve to a value, false if it must not. |
| `not` | object |  | Composite predicate: true when the nested predicate is false. |
| `op` | string |  | Comparison operator for a compare predicate (path, op, value). |
| `path` | string |  | Reference to a value via dot-notation against the composition's input and step outputs. Example: '${classify.output.intent}'. |
| `value` | — |  | Literal comparison value (string, number, boolean, or array for in/not_in). |

### `steps[].reduce`

| field | type | required | description |
|-------|------|----------|-------------|
| `into` | string | ✓ | Field name under which the merged result is placed on the parallel step's output. Subsequent steps reference it as ${<parallelStepId>.output.<into>}. |
| `strategy` | — | ✓ | How branch outputs merge: 'append' extends lists, 'replace' keeps the last write, 'barrier' collects every output into a named map. |

### `steps[].modifiers`

| field | type | required | description |
|-------|------|----------|-------------|
| `eval` | array |  | References to eval keys defined in the pack's evals object (RFC 0006). Runtimes may execute these inline or post-Send. |
| `retry` | object |  | Re-runs the step on error, up to max_attempts. |

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
| `action_scope` | object |  | — |
| `client` | object |  | — |
| `description` | string | ✓ | — |
| `exec` | object |  | — |
| `extensions` | object |  | — |
| `http` | object |  | — |
| `input_schema` | — | ✓ | — |
| `mock_parts` | array |  | — |
| `mock_result` | — |  | Static mock response returned regardless of tool-call args. Use when the response does not depend on inputs. Mutually exclusive with mock_template. |
| `mock_template` | string |  | Go text/template rendered against tool-call args (parsed as a JSON map). Rendered output is parsed back as JSON. Use this instead of mock_result when the response should depend on inputs (e.g. branching on order_id with {{ if eq .order_id "X" }}...{{ end }}). Mutually exclusive with mock_result. |
| `mode` | string | ✓ | Execution mode. One of: 'mock' (use mock_result or mock_template) - 'live' (HTTP via 'http') - 'mcp' (MCP server) - 'exec' (subprocess) - 'client' (client-side handler). |
| `name` | string |  | — |
| `output_schema` | — | ✓ | — |
| `timeout_ms` | integer |  | — |
