---
title: Use Arena as a Go Library
description: Build a config in Go, run scenarios with the Arena engine and read the results from the state store.
verified:
  commit: 72f35dcda537f9ebff0eaf33808ce728f0c8b3f8
  sources:
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/run_interactive.go
    - arena/engine/builder_integration.go
    - arena/engine/engine.go
    - arena/engine/execution.go
    - arena/statestore/metadata.go
    - arena/statestore/store.go
---

Embed the Arena engine in a Go program to build scenarios in code, run them against providers and read the results. See [Programmatic vs CLI](/arena/explanation/programmatic-vs-cli/) for when to choose the library over the CLI.

## Prerequisites

- Go 1.26 or later.
- An API key in the environment for each real provider you use (for example `OPENAI_API_KEY`). The `mock` provider needs none.

## 1. Install

```bash
go get github.com/AltairaLabs/promptarena/v2/arena/engine
go get github.com/AltairaLabs/PromptKit/pkg/v2/config
go get github.com/AltairaLabs/PromptKit/runtime/v2/prompt
```

## 2. Create the configuration

`arenaconfig.Config` holds the loaded providers, prompts and scenarios. Set every `Loaded*` map before you build the engine components: the engine reads them once and ignores later changes.

```go
import (
	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

promptConfig := &prompt.Config{
	Spec: prompt.Spec{
		TaskType:       "assistant",
		SystemTemplate: "You are a helpful assistant.",
	},
}

cfg := &arenaconfig.Config{
	LoadedProviders: map[string]*config.Provider{
		"openai": {ID: "openai", Type: "openai", Model: "gpt-4o-mini"},
	},
	LoadedPromptConfigs: map[string]*arenaconfig.PromptConfigData{
		"assistant": {Config: promptConfig, TaskType: "assistant"},
	},
	LoadedScenarios: map[string]*arenaconfig.Scenario{
		"test-1": {
			ID:       "test-1",
			TaskType: "assistant",
			Turns: []arenaconfig.TurnDefinition{
				{Role: "user", Content: "Hello"},
			},
		},
	},
	Defaults: arenaconfig.Defaults{Temperature: 0.7, MaxTokens: 500},
}
```

## 3. Build the engine

`BuildEngineComponents` returns nine values. Pass `nil` as the second argument to initialise every provider.

```go
import "github.com/AltairaLabs/promptarena/v2/arena/engine"

providerReg, promptReg, mcpReg, executor, adapterReg, _, toolReg, _, err :=
	engine.BuildEngineComponents(cfg, nil)
if err != nil {
	return err
}

eng, err := engine.NewEngine(cfg, providerReg, promptReg, mcpReg, executor, adapterReg, toolReg)
if err != nil {
	return err
}
defer eng.Close()
```

`engine.NewEngineFromConfig(cfg)` does both calls in one step and wires the A2A cleanup and skills factory for you.

## 4. Run the scenarios

`GenerateRunPlan` takes four filters: regions, providers, scenarios and evals. `nil` selects everything. `ExecuteRuns` takes the number of concurrent workers.

```go
plan, err := eng.GenerateRunPlan(nil, nil, nil, nil)
if err != nil {
	return err
}

ctx := context.Background()
runIDs, err := eng.ExecuteRuns(ctx, plan, 4)
if err != nil {
	return err
}
```

`runIDs` has one entry per planned run. The entry is empty for a run that failed to save.

## 5. Read the results

With the default state store, `GetStateStore` returns a `*statestore.ArenaStateStore`.

```go
import "github.com/AltairaLabs/promptarena/v2/arena/statestore"

store, ok := eng.GetStateStore().(*statestore.ArenaStateStore)
if !ok {
	return fmt.Errorf("unexpected state store %T", eng.GetStateStore())
}

for _, runID := range runIDs {
	result, err := store.GetResult(ctx, runID)
	if err != nil {
		log.Printf("no result for %q: %v", runID, err)
		continue
	}
	fmt.Printf("%s on %s: error=%q\n", result.ScenarioID, result.ProviderID, result.Error)
}
```

An empty `result.Error` means the run finished without an error.

## Generate scenarios from data

Fill `cfg.LoadedScenarios` before step 3. `loadTestCases` and `buildTurns` stand for your own code.

```go
scenarios := make(map[string]*arenaconfig.Scenario)
for _, tc := range loadTestCases() {
	scenarios[tc.ID] = &arenaconfig.Scenario{
		ID:          tc.ID,
		TaskType:    tc.TaskType,
		Description: tc.Description,
		Turns:       buildTurns(tc.Conversations),
	}
}
cfg.LoadedScenarios = scenarios
```

## Compare providers

Define every provider in `cfg.LoadedProviders` before step 3. Each scenario then runs against the providers in its group.

```go
cfg.LoadedProviders = map[string]*config.Provider{
	"openai-gpt4o":     {ID: "openai-gpt4o", Type: "openai", Model: "gpt-4o"},
	"anthropic-claude": {ID: "anthropic-claude", Type: "claude", Model: "claude-sonnet-4-6"},
	"google-gemini":    {ID: "google-gemini", Type: "gemini", Model: "gemini-2.5-flash"},
}
```

The provider `Type` is `openai`, `claude`, `gemini` or `mock`. `anthropic` is not a type.

## Filter a run

Provider filters match provider IDs exactly, and scenario filters match scenario IDs. An unknown scenario ID returns an error.

```go
plan, err := eng.GenerateRunPlan(
	nil,                                          // all regions
	[]string{"openai-gpt4o", "anthropic-claude"}, // only these provider IDs
	[]string{"critical-test-1"},                  // only this scenario
	nil,                                          // all evals
)
```

## Run without API calls

Define a provider of type `mock` before step 3.

```go
cfg.LoadedProviders = map[string]*config.Provider{
	"mock": {ID: "mock", Type: "mock", Model: "test-model"},
}
```

To switch an existing engine to mock responses, pass a mock config file to `EnableMockProviderMode`:

```go
if err := eng.EnableMockProviderMode("mock-config.yaml"); err != nil {
	return err
}
```

## Process results yourself

```go
type TestMetrics struct {
	Provider   string
	Scenario   string
	Duration   time.Duration
	Cost       float64
	TokensUsed int
	Success    bool
}

func processResults(ctx context.Context, runIDs []string, store *statestore.ArenaStateStore) []TestMetrics {
	var metrics []TestMetrics
	for _, runID := range runIDs {
		result, err := store.GetResult(ctx, runID)
		if err != nil {
			continue
		}
		metrics = append(metrics, TestMetrics{
			Provider:   result.ProviderID,
			Scenario:   result.ScenarioID,
			Duration:   result.Duration,
			Cost:       result.Cost.TotalCost,
			TokensUsed: result.Cost.InputTokens + result.Cost.OutputTokens,
			Success:    result.Error == "",
		})
	}
	return metrics
}
```

## Run from `go test`

`createTestConfig` stands for your own function that returns an `*arenaconfig.Config`.

```go
func TestLLMResponses(t *testing.T) {
	cfg := createTestConfig()

	providerReg, promptReg, mcpReg, executor, adapterReg, _, toolReg, _, err :=
		engine.BuildEngineComponents(cfg, nil)
	require.NoError(t, err)

	eng, err := engine.NewEngine(cfg, providerReg, promptReg, mcpReg, executor, adapterReg, toolReg)
	require.NoError(t, err)
	defer eng.Close()

	plan, err := eng.GenerateRunPlan(nil, nil, nil, nil)
	require.NoError(t, err)
	runIDs, err := eng.ExecuteRuns(context.Background(), plan, 4)
	require.NoError(t, err)

	store := eng.GetStateStore().(*statestore.ArenaStateStore)
	for _, runID := range runIDs {
		result, err := store.GetResult(context.Background(), runID)
		require.NoError(t, err)
		assert.Empty(t, result.Error)
	}
}
```

## Exit non-zero on failures

For a CI job, count the failed runs and exit with a status code.

```go
failed := 0
for _, runID := range runIDs {
	result, err := store.GetResult(ctx, runID)
	if err != nil || result.Error != "" {
		failed++
	}
}
if failed > 0 {
	fmt.Printf("%d of %d runs failed\n", failed, len(runIDs))
	os.Exit(1)
}
```

## Good practice

Close the engine with `defer eng.Close()`. `Close` shuts down the MCP registry, provider registry and event store. Build a separate set of components for each engine, because closing one engine closes the registries it was given.

Match the worker count to the provider's rate limit: a low value such as `2` suits rate-limited APIs, and a high value such as `20` suits the `mock` provider.

Log a failed `GetResult` lookup and continue, as step 5 does, so one bad run does not stop the batch.

## Verify

Run the program from step 5 with the `mock` provider:

```bash
go run .
```

The output has one line per run, such as `test-1 on mock: error=""`.

## Troubleshooting

### Package not found

```bash
go mod tidy
go get github.com/AltairaLabs/promptarena/v2/arena/engine@latest
```

### Type assertion fails

`GetStateStore` returns the `statestore.Store` interface. A configured Redis state store returns a different type, so check the assertion:

```go
arenaStore, ok := eng.GetStateStore().(*statestore.ArenaStateStore)
if !ok {
	log.Fatal("expected ArenaStateStore")
}
```

### Provider changes have no effect

The engine copies `cfg.LoadedProviders` and `cfg.LoadedScenarios` when you build it. Set both before `BuildEngineComponents`.

### A filter matches no runs

Provider filters take provider IDs, not provider types. An unmatched provider filter produces an empty plan.

### Memory use with large suites

Process the run IDs in batches:

```go
const batchSize = 100
for i := 0; i < len(runIDs); i += batchSize {
	end := min(i+batchSize, len(runIDs))
	processBatch(runIDs[i:end], store)
}
```

## Related pages

- [Programmatic Usage tutorial](/arena/tutorials/07-programmatic-usage/) walks through a first program step by step.
- [API reference](/arena/reference/api-reference/) lists the engine API.
- [Programmatic vs CLI](/arena/explanation/programmatic-vs-cli/) compares the two ways to run Arena.
