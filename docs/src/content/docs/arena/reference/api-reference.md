---
title: Go API Reference
description: Complete API reference for using Arena as a Go library
---

Reference documentation for the Arena Go API, covering all exported types, functions, and methods for programmatic usage.

## Core Packages

### `github.com/AltairaLabs/promptarena/arena/engine`

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
    skillExecutor *skills.Executor,
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
- `a2aCleanup` - Releases any A2A agent connections; call it if you do not go on to build an engine (`nil` when there are none)
- `toolRegistry` - Registry of tools the runtime can execute
- `skillExecutor` - Executor for skill activation (`nil` when the config has no skills)
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
calling `SetEventBus` forwards the bus to the evals and `eval.completed`
events reach subscribers.

`NewEngine` does not initialize the workflow state machine, the memory
subsystem or runtime hooks from the config; `NewEngineFromConfig` does. Prefer
`NewEngineFromConfig` unless you are substituting components.

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
the config declares. This is the constructor the CLI uses. Modify `cfg` before
calling it to override what was loaded from disk.

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
    scenarioFilter []string,
) (*RunPlan, error)
```

Creates a test execution plan from filter criteria.

**Parameters:**
- `regionFilter` - Regions to include (nil = all)
- `providerFilter` - Providers to include (nil = all)
- `scenarioFilter` - Scenarios to include (nil = all)

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

Shuts down the engine and cleans up resources.

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

Replaces all providers with mock providers for testing.

**Parameters:**
- `mockConfigPath` - Path to mock config YAML (empty string for default responses)

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

### `github.com/AltairaLabs/promptarena/arena/arenaconfig`

#### `Config`

```go
type Config struct {
    LoadedProviders     map[string]*Provider
    LoadedScenarios     map[string]*Scenario
    LoadedPromptConfigs map[string]*PromptConfigData
    Defaults            Defaults
    // ... additional fields
}
```

Main Arena configuration structure.

---

#### `Provider`

```go
type Provider struct {
    ID    string
    Type  string  // "openai", "claude", "gemini", "mock"
    Model string
    // ... additional fields
}
```

Provider configuration.

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
    Role      string  // "user", "assistant", or self-play role (e.g. "gemini-user")
    Content   string  // Message content (scripted turns)
    Persona   string  // Persona ID (self-play turns)
    Turns     int     // Number of self-play turns (exact if MaxTurns absent)
    MaxTurns  int     // Upper bound; enables natural termination when > Turns
    // ... additional fields
}
```

Single conversation turn. For self-play turns, `Turns` sets the minimum number of
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

### `github.com/AltairaLabs/promptarena/arena/statestore`

#### `ArenaStateStore`

```go
type ArenaStateStore struct {
    // ... internal fields
}
```

State store implementation for Arena results.

---

#### `GetRunResult`

```go
func (s *ArenaStateStore) GetRunResult(
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
result, err := arenaStore.GetRunResult(ctx, runID)
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

### `github.com/AltairaLabs/PromptKit/runtime/prompt`

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

### `github.com/AltairaLabs/promptarena/arena/generate`

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

// 4. Generate plan
plan, err := eng.GenerateRunPlan(nil, nil, nil)
if err != nil {
    return err
}

// 5. Execute
ctx := context.Background()
runIDs, err := eng.ExecuteRuns(ctx, plan, 4)
if err != nil {
    return err
}

// 6. Get results
store := eng.GetStateStore().(*statestore.ArenaStateStore)
for _, runID := range runIDs {
    result, _ := store.GetRunResult(ctx, runID)
    // Process result
}
```

---

## See Also

- [Tutorial: Programmatic Usage](/arena/tutorials/07-programmatic-usage/)
- [How-To: Use as Go Library](/arena/how-to/setup/use-as-go-library/)
