---
title: Go API Reference
description: Exported types, functions and methods for using Arena as a Go library
verified:
  commit: 41e16490ec749312f63e711caf8cbd08bad51c64
  sources:
    - arena/arenaconfig/deprecated.go
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/deploy/flow/sessions.go
    - arena/engine/builder_integration.go
    - arena/engine/engine.go
    - arena/engine/execution.go
    - arena/engine/types.go
    - arena/generate/adapter.go
    - arena/generate/converter.go
    - arena/generate/decision.go
    - arena/generate/expectation.go
    - arena/generate/generate.go
    - arena/generate/sources/recordings.go
    - arena/selfplay/registry.go
    - arena/statestore/metadata.go
    - arena/statestore/store.go
---

Exported types, functions and methods of the Arena Go packages. For a walkthrough, see [Programmatic Usage](/arena/tutorials/07-programmatic-usage/).

## Core Packages

### `github.com/AltairaLabs/promptarena/v2/arena/engine`

The main package for programmatic Arena usage.

#### `BuildEngineComponents`

```go
func BuildEngineComponents(cfg *arenaconfig.Config, providerFilter []string) (
    providerRegistry *providers.Registry,
    promptRegistry *prompt.Registry,
    mcpRegistry *mcp.RegistryImpl,
    convExecutor ConversationExecutor,
    adapterRegistry *adapters.Registry,
    a2aCleanup func(),
    toolRegistry *tools.Registry,
    skillFactory *SkillsFactory,
    err error,
)
```

Builds all engine components from a loaded config. `providerFilter` limits which
providers have credentials resolved; `nil` or empty initializes all of them.

**Returns:**
- `providerRegistry` - Registry containing all configured LLM providers
- `promptRegistry` - Registry containing all prompt configurations (`nil` when the config loads none)
- `mcpRegistry` - Registry for Model Context Protocol servers (`nil` when none are configured)
- `convExecutor` - Executor for running conversations, with the eval orchestrator already injected
- `adapterRegistry` - Recording-format readers used by eval scenarios
- `a2aCleanup` - Releases any A2A agent connections (`nil` when there are none). `NewEngine` does not take it, so when you build the engine with `NewEngine`, call it yourself when you are done; `NewEngineFromConfig` stores it and `Close` calls it
- `toolRegistry` - Registry of tools the runtime can execute
- `skillFactory` - Factory for skill activation (`nil` when no skill sources are loaded)
- `err` - Error if component building fails

**Example:**
```go
cfg, err := arenaconfig.LoadConfig("config.arena.yaml")
if err != nil {
    log.Fatal(err)
}

providerReg, promptReg, mcpReg, executor, adapterReg, cleanup, toolReg, _, err := engine.BuildEngineComponents(cfg, nil)
if err != nil {
    log.Fatal(err)
}
if cleanup != nil {
    defer cleanup()
}
```

---

#### `NewEngine`

```go
func NewEngine(
    cfg *arenaconfig.Config,
    providerRegistry *providers.Registry,
    promptRegistry *prompt.Registry,
    mcpRegistry *mcp.RegistryImpl,
    convExecutor ConversationExecutor,
    adapterRegistry *adapters.Registry,
    toolRegistry *tools.Registry,
) (*Engine, error)
```

Creates a new Engine from pre-built components. The engine adopts the eval
orchestrator that `BuildEngineComponents` injected into `convExecutor`, so an
engine built this way wires evals identically to `NewEngineFromConfig`:
`SetEventBus` forwards the bus to the evals and `eval.completed`
events reach subscribers.

`NewEngine` does not initialize the workflow state machine, the memory
subsystem or runtime hooks from the config; `NewEngineFromConfig` does.

**Parameters:**
- `cfg` - Configuration object
- `providerRegistry`, `promptRegistry`, `mcpRegistry`, `convExecutor`, `adapterRegistry`, `toolRegistry` - The components returned by `BuildEngineComponents`

**Returns:**
- `*Engine` - Initialized engine ready for execution

---

#### `NewEngineFromConfig`

```go
func NewEngineFromConfig(cfg *arenaconfig.Config, providerFilter ...string) (*Engine, error)
```

Builds the components and the engine from a pre-loaded config in one call, then
initializes the workflow state machine, the memory subsystem and runtime hooks
the config declares. This is the constructor the CLI uses. Changes made to
`cfg` before the call override what was loaded from disk.

---

#### `NewEngineFromConfigFile`

```go
func NewEngineFromConfigFile(configPath string) (*Engine, error)
```

Creates a new Engine by loading configuration from a file.

**Parameters:**
- `configPath` - Path to arena.yaml configuration file

**Returns:**
- `*Engine` - Initialized engine
- `error` - Error if loading or initialization fails

**Example:**
```go
eng, err := engine.NewEngineFromConfigFile("config.arena.yaml")
if err != nil {
    log.Fatal(err)
}
defer eng.Close()
```

---

### Engine Methods

#### `GenerateRunPlan`

```go
func (e *Engine) GenerateRunPlan(
    regionFilter,
    providerFilter,
    scenarioFilter,
    evalFilter []string,
) (*RunPlan, error)
```

Creates a test execution plan from filter criteria.

**Parameters:**
- `regionFilter` - Regions to include (nil = all)
- `providerFilter` - Providers to include (nil = all)
- `scenarioFilter` - Scenarios to include (nil = all)
- `evalFilter` - Evals to include (nil = all)

**Returns:**
- `*RunPlan` - Execution plan with all matching combinations
- `error` - Error if plan generation fails

**Example:**
```go
// Test only OpenAI and Claude providers
plan, err := eng.GenerateRunPlan(
    nil,                           // all regions
    []string{"openai", "claude"},  // specific providers
    nil,                           // all scenarios
    nil,                           // all evals
)
```

---

#### `ExecuteRuns`

```go
func (e *Engine) ExecuteRuns(
    ctx context.Context,
    plan *RunPlan,
    concurrency int,
) ([]string, error)
```

Executes all runs in the plan concurrently.

**Parameters:**
- `ctx` - Context for cancellation
- `plan` - RunPlan from `GenerateRunPlan`
- `concurrency` - Maximum concurrent executions

**Returns:**
- `[]string` - Run IDs in same order as plan combinations
- `error` - Error if execution setup fails

**Example:**
```go
ctx := context.Background()
runIDs, err := eng.ExecuteRuns(ctx, plan, 4)
if err != nil {
    log.Fatal(err)
}
```

---

#### `GetStateStore`

```go
func (e *Engine) GetStateStore() statestore.Store
```

Returns the engine's state store for accessing results.

**Returns:**
- `statestore.Store` - State store interface

**Example:**
```go
arenaStore, ok := eng.GetStateStore().(*statestore.ArenaStateStore)
if !ok {
    log.Fatal("Expected ArenaStateStore")
}
```

---

#### `Close`

```go
func (e *Engine) Close() error
```

Shuts down the engine and cleans up resources. It closes the provider, MCP and
self-play registries the engine was built with, including ones you passed to
`NewEngine` from `BuildEngineComponents`, so do not share one set of components
between two engines. Call `BuildEngineComponents` again for each engine.

**Returns:**
- `error` - Error if cleanup fails

**Example:**
```go
defer eng.Close()
```

---

#### `EnableMockProviderMode`

```go
func (e *Engine) EnableMockProviderMode(mockConfigPath string) error
```

Replaces all providers, including the providers behind self-play roles, with mock providers for testing. TTS and STT providers are not swapped. The file is decoded
strictly, so a misspelled key returns an error naming it and the valid keys.

**Parameters:**
- `mockConfigPath` - Path to mock config YAML. An empty string keeps each provider's own `mock_config`, and a provider without one gets default responses

**Returns:**
- `error` - Error if mock configuration is invalid

**Example:**
```go
err := eng.EnableMockProviderMode("mock-responses.yaml")
```

---

#### `EnableSessionRecording`

```go
func (e *Engine) EnableSessionRecording(recordingDir string) error
```

Enables session recording for all runs.

**Parameters:**
- `recordingDir` - Directory to store recording files

**Returns:**
- `error` - Error if directory cannot be created

---

### Types

#### `RunPlan`

```go
type RunPlan struct {
    Combinations []RunCombination
}
```

Represents a test execution plan.

**Fields:**
- `Combinations` - All test combinations to execute

---

#### `RunCombination`

```go
type RunCombination struct {
    Region     string
    ScenarioID string
    ProviderID string
}
```

Represents a single test execution.

**Fields:**
- `Region` - Region identifier
- `ScenarioID` - Scenario to execute
- `ProviderID` - Provider to use

---

#### `RunResult`

```go
type RunResult struct {
    RunID      string
    ProviderID string
    ScenarioID string
    Messages   []types.Message
    Cost       types.CostInfo
    Duration   time.Duration
    Error      string
    // ... additional fields
}
```

Contains complete results of a test execution.

**Key Fields:**
- `RunID` - Unique identifier for this run
- `ProviderID` - Provider that executed the test
- `ScenarioID` - Scenario that was executed
- `Messages` - Full conversation history
- `Cost` - Token usage and cost information
- `Duration` - Execution time
- `Error` - Error message if test failed

---

## Configuration Package

### `github.com/AltairaLabs/promptarena/v2/arena/arenaconfig`

#### `Config`

```go
type Config struct {
    LoadedProviders     map[string]*config.Provider
    LoadedScenarios     map[string]*Scenario
    LoadedPromptConfigs map[string]*PromptConfigData
    Defaults            Defaults
    // ... additional fields
}
```

Main Arena configuration structure.

`Workflow` and `Compositions` are deprecated untyped forms of `WorkflowSpec`
and `CompositionSpecs`, kept for Go callers that build a `Config` by hand.
Loading from YAML never sets them. `ResolveDeprecatedFields` copies each one
over when its typed field is unset; `NewEngine` calls it, so callers need not.

---

#### `Provider`

```go
// package config: github.com/AltairaLabs/PromptKit/pkg/v2/config
type Provider struct {
    ID    string
    Type  string  // "openai", "claude", "gemini", "mock"
    Model string
    // ... additional fields
}
```

Provider configuration. `arenaconfig.Config.LoadedProviders` holds `*config.Provider` values.

---

#### `Scenario`

```go
type Scenario struct {
    ID          string
    TaskType    string
    Description string
    Turns       []TurnDefinition
    // ... additional fields
}
```

Test scenario definition.

---

#### `TurnDefinition`

```go
type TurnDefinition struct {
    Role      string  // "user", or a self-play role ID from spec.self_play.roles (e.g. "gemini-user")
    Content   string  // Message content (scripted turns)
    Persona   string  // Persona ID (self-play turns)
    Turns     int     // Number of self-play turns (exact if MaxTurns absent)
    MaxTurns  int     // Upper bound; enables natural termination when > Turns
    // ... additional fields
}
```

Single conversation turn. The assistant's reply is never scripted, so there is no `"assistant"` role. For self-play turns, `Turns` sets the minimum number of
exchanges and `MaxTurns` sets the upper bound. When `MaxTurns > Turns`, natural
termination is enabled: the self-play LLM can signal conversation completion after
the minimum turns are reached. If `MaxTurns` is absent, exactly `Turns` exchanges
are executed (backward compatible).

---

#### `Defaults`

```go
type Defaults struct {
    Temperature float32
    MaxTokens   int
    Output      OutputConfig
    // ... additional fields
}
```

Default execution parameters.

---

## State Store Package

### `github.com/AltairaLabs/promptarena/v2/arena/statestore`

#### `ArenaStateStore`

```go
type ArenaStateStore struct {
    // ... internal fields
}
```

State store implementation for Arena results.

---

#### `GetResult`

```go
func (s *ArenaStateStore) GetResult(
    ctx context.Context,
    runID string,
) (*RunResult, error)
```

Retrieves detailed results for a specific run.

**Parameters:**
- `ctx` - Context for cancellation
- `runID` - Run identifier from `ExecuteRuns`

**Returns:**
- `*RunResult` - Complete run results
- `error` - Error if run not found or retrieval fails

**Example:**
```go
result, err := arenaStore.GetResult(ctx, runID)
if err != nil {
    log.Printf("Failed to get result: %v", err)
    continue
}

fmt.Printf("Scenario: %s\n", result.ScenarioID)
fmt.Printf("Duration: %s\n", result.Duration)
fmt.Printf("Cost: $%.6f\n", result.Cost.TotalCost)
```

---

## Prompt Package

### `github.com/AltairaLabs/PromptKit/runtime/v2/prompt`

#### `Config`

```go
type Config struct {
    Spec Spec
}
```

Prompt configuration wrapper.

---

#### `Spec`

```go
type Spec struct {
    TaskType       string
    Version        string
    Description    string
    SystemTemplate string
    AllowedTools   []string
    // ... additional fields
}
```

Prompt specification.

**Example:**
```go
promptConfig := &prompt.Config{
    Spec: prompt.Spec{
        TaskType:       "assistant",
        Version:        "v1.0.0",
        Description:    "Helpful assistant",
        SystemTemplate: "You are a helpful AI assistant.",
        AllowedTools:   []string{"calculator", "search"},
    },
}
```

---

## Generate Package

### `github.com/AltairaLabs/promptarena/v2/arena/generate`

Turns recorded sessions into regression scenarios. This is what `promptarena
generate` runs; it is exposed so a platform can run it in-process.

#### `Generate`

```go
func Generate(ctx context.Context, req Request) (*Result, error)

type Request struct {
    Source  SessionSourceAdapter // required: where sessions come from
    List    ListOptions          // FilterPassed, FilterEvalType, Expectations, Limit
    Convert ConvertOptions       // TaskType, Expectations
    Dedup   bool                 // drop sessions with the same failure fingerprint
    Pack    *packspec.Pack       // supplies eval params and declared thresholds the source did not record
}

type Result struct {
    Scenarios    []*arenaconfig.ScenarioConfig
    Skipped      []Skipped   // sessions that failed to load or convert, with reasons
    Warnings     []string    // e.g. a multi-turn session whose replay may not behave as recorded
    Decisions    []Decision  // one per recorded eval: what it became and why
    Deduplicated int
}
```

Lists sessions from the source, fetches each, optionally deduplicates, and
converts each into a scenario. Nothing is printed and nothing is written to
disk. A session that fails to load or convert is recorded in `Skipped`; a
failure to list is fatal.

Every recorded eval produces one `Decision`: a failed recorded verdict is
asserted with its params; a measurement bounded by the pack's declared
`threshold` or by an `Expectation` is asserted with that bound; a measurement
with no known range is reported, never asserted against an invented bound.

#### `WriteScenarios`

```go
func WriteScenarios(dir string, scenarios []*arenaconfig.ScenarioConfig) ([]string, error)
```

Writes each scenario to `<dir>/<name>.scenario.yaml` and returns the paths.

#### `SessionSourceAdapter`

```go
type SessionSourceAdapter interface {
    Name() string
    List(ctx context.Context, opts ListOptions) ([]SessionSummary, error)
    Get(ctx context.Context, sessionID string) (*SessionDetail, error)
}
```

Implement this to serve sessions from your own store. `SessionDetail` carries
the messages (PromptKit `types.Message`, tool calls inline), pack identity,
variables, the workflow trace and every recorded eval with its kind, score and
verdict. Two implementations ship: `sources.NewRecordingsAdapter(glob)` reads
local run output and session recordings, and `flow.OpenSessionSource` (package
`arena/deploy/flow`) wraps an installed deploy adapter that advertises the
`sessions` capability.

#### `Expectation`

```go
type Expectation struct{ EvalID string; Min, Max *float64 }
func ParseExpectation(s string) (Expectation, error) // "faithfulness>=0.8"
```

A stated range for a measured eval. On `ListOptions` it selects sessions whose
score fell outside the range; on `ConvertOptions` it becomes the generated
assertion's `min_score` / `max_score`. `Generate` copies `List.Expectations`
to `Convert.Expectations` when the latter is empty.

## Common Patterns

### Complete Workflow

```go
// 1. Load configuration (or construct an arenaconfig.Config yourself)
cfg, err := arenaconfig.LoadConfig("config.arena.yaml")
if err != nil {
    return err
}

// 2. Build the engine. NewEngineFromConfig builds the components, the engine,
//    and the workflow, memory and hook subsystems the config declares.
eng, err := engine.NewEngineFromConfig(cfg)
if err != nil {
    return err
}
defer eng.Close()

// 3. Generate plan
plan, err := eng.GenerateRunPlan(nil, nil, nil, nil)
if err != nil {
    return err
}

// 4. Execute
ctx := context.Background()
runIDs, err := eng.ExecuteRuns(ctx, plan, 4)
if err != nil {
    return err
}

// 5. Get results
store := eng.GetStateStore().(*statestore.ArenaStateStore)
for _, runID := range runIDs {
    result, _ := store.GetResult(ctx, runID)
    // Process result
}
```

---

## See Also

- [Tutorial: Programmatic Usage](/arena/tutorials/07-programmatic-usage/)
- [How-To: Use as Go Library](/arena/how-to/setup/use-as-go-library/)
