---
title: 'Tutorial 7: Using Arena as a Go Library'
description: Build a Go program that configures PromptArena in code, runs a scenario against the mock provider and reads the results.
verified:
  commit: 72f35dcda537f9ebff0eaf33808ce728f0c8b3f8
  sources:
    - arena/arenaconfig/types.go
    - arena/assertions/types.go
    - arena/engine/builder_integration.go
    - arena/engine/engine.go
    - arena/engine/execution.go
    - arena/engine/types.go
    - arena/statestore/metadata.go
    - arena/statestore/store.go
---

In this tutorial we write a Go program that runs a PromptArena scenario without any YAML files.

## What we will build

A single `main.go` that:

- builds the Arena configuration in Go
- runs one scenario against the mock provider, so no API key is needed
- reads the run result from the state store and prints it

## Prerequisites

- Go 1.26 or later
- Familiarity with scenarios and providers from the earlier tutorials

## Step 1: Create a Go module

```bash
mkdir arena-lib-demo
cd arena-lib-demo
go mod init arena-lib-demo
```

Go prints `go: creating new go.mod: module arena-lib-demo`.

## Step 2: Install the dependencies

```bash
go get github.com/AltairaLabs/promptarena/v2/arena/engine
```

Go prints `go: added ...` lines, including `github.com/AltairaLabs/promptarena/v2` (v2.4.0 or later) and the PromptKit modules `github.com/AltairaLabs/PromptKit/pkg/v2` and `github.com/AltairaLabs/PromptKit/runtime/v2` it depends on.

## Step 3: Write the program

Create `main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/engine"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"
)

func main() {
	// Pick the provider: mock by default, openai when ARENA_PROVIDER=openai.
	provider := &config.Provider{ID: "mock-provider", Type: "mock", Model: "gpt-4"}
	if os.Getenv("ARENA_PROVIDER") == "openai" {
		provider = &config.Provider{ID: "openai-gpt4o-mini", Type: "openai", Model: "gpt-4o-mini"}
	}

	// Create a prompt configuration
	promptConfig := &prompt.Config{
		Spec: prompt.Spec{
			TaskType:    "assistant",
			Version:     "v1.0.0",
			Description: "A helpful AI assistant",
			SystemTemplate: `You are a helpful AI assistant.
Be concise and accurate in your responses.`,
		},
	}

	// Create the Arena configuration
	cfg := &arenaconfig.Config{
		LoadedProviders: map[string]*config.Provider{
			provider.ID: provider,
		},
		LoadedPromptConfigs: map[string]*arenaconfig.PromptConfigData{
			"assistant": {
				Config:   promptConfig,
				TaskType: "assistant",
			},
		},
		LoadedScenarios: map[string]*arenaconfig.Scenario{
			"basic-test": {
				ID:          "basic-test",
				TaskType:    "assistant",
				Description: "Basic conversation test",
				Turns: []arenaconfig.TurnDefinition{
					{Role: "user", Content: "What is 2+2?"},
					{Role: "user", Content: "What's the capital of France?"},
				},
			},
		},
		Defaults: arenaconfig.Defaults{
			Temperature: 0.7,
			MaxTokens:   500,
			Output: arenaconfig.OutputConfig{
				Dir: "out",
			},
		},
	}

	fmt.Println("Building Arena engine...")

	// Build engine components
	providerReg, promptReg, mcpReg, executor, adapterReg, _, toolReg, _, err :=
		engine.BuildEngineComponents(cfg, nil)
	if err != nil {
		log.Fatalf("Failed to build components: %v", err)
	}

	// Create the engine
	eng, err := engine.NewEngine(cfg, providerReg, promptReg, mcpReg, executor, adapterReg, toolReg)
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
	}
	defer eng.Close()

	// Generate execution plan
	plan, err := eng.GenerateRunPlan(nil, nil, nil, nil)
	if err != nil {
		log.Fatalf("Failed to generate plan: %v", err)
	}

	fmt.Printf("Generated %d test combinations\n", len(plan.Combinations))

	// Execute tests
	ctx := context.Background()
	runIDs, err := eng.ExecuteRuns(ctx, plan, 2)
	if err != nil {
		log.Fatalf("Failed to execute: %v", err)
	}

	// Retrieve results
	arenaStore, ok := eng.GetStateStore().(*statestore.ArenaStateStore)
	if !ok {
		log.Fatalf("unexpected state store %T", eng.GetStateStore())
	}

	for i, runID := range runIDs {
		result, err := arenaStore.GetResult(ctx, runID)
		if err != nil {
			log.Printf("Failed to get result: %v", err)
			continue
		}

		fmt.Printf("\nTest %d: %s\n", i+1, result.ScenarioID)
		fmt.Printf("Status: %s\n", getStatus(result.Error))
		fmt.Printf("Duration: %s\n", result.Duration)
		fmt.Printf("Messages: %d\n", len(result.Messages))
	}

	fmt.Println("\nAll tests completed!")
}

func getStatus(errMsg string) string {
	if errMsg == "" {
		return "Success"
	}
	return "Failed: " + errMsg
}
```

The program uses the `mock` provider unless `ARENA_PROVIDER=openai` is set. It sets `Defaults.Output.Dir` to `out`, so the engine creates `out/` and `media/` in the current directory.

## Step 4: Run it

`go mod tidy` records the remaining packages `main.go` imports:

```bash
go mod tidy
go run .
```

The program logs `INFO` lines from the engine as it works. With those lines removed, the output is:

```text
Building Arena engine...
Generated 1 test combinations

Test 1: basic-test
Status: Success
Duration: 6.435875ms
Messages: 5

All tests completed!
```

`Messages: 5` counts the system message plus the two user messages and two assistant replies. `Duration` varies from run to run.

## Step 5: Add assertions

Add the `asrt` import to the import block in `main.go`:

```go
asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
```

Replace the `Turns` list of `basic-test` with one turn that carries two `content_includes` assertions:

```go
Turns: []arenaconfig.TurnDefinition{
	{
		Role:    "user",
		Content: "What is 2+2?",
		Assertions: []asrt.AssertionConfig{
			{
				Type:    "content_includes",
				Params:  map[string]interface{}{"patterns": []string{"4"}},
				Message: "Should contain the answer 4",
			},
			{
				Type:    "content_includes",
				Params:  map[string]interface{}{"patterns": []string{"zebra"}},
				Message: "Should contain the word zebra",
			},
		},
	},
},
```

Run it again:

```bash
go run .
```

The mock reply contains `4` but not `zebra`, so the second assertion fails and the run fails. The engine also logs two `ERROR` lines for the failed turn; with those removed, the output is:

```text
Test 1: basic-test
Status: Failed: pipeline execution failed: validation failed: [assertion 1 (content_includes) failed: missing patterns: zebra]
Duration: 4.853875ms
Messages: 3
```

Delete the `zebra` assertion and run again to get `Status: Success`.

## Step 6: Swap in a real provider

Export your OpenAI key. The library does not read `.env` files.

```bash
export OPENAI_API_KEY=sk-...
ARENA_PROVIDER=openai go run .
```

The program switches to `Type: "openai"` with `Model: "gpt-4o-mini"`, and the result has the same shape:

```text
Test 1: basic-test
Status: Success
Duration: 2.45s
Messages: 5
```

## What we have built

A Go program that builds `arenaconfig.Config` in code, runs it through `engine.NewEngine` and `ExecuteRuns`, and reads each run from the `ArenaStateStore`.

## Next steps

- [Use Arena as a Go library](/arena/how-to/setup/use-as-go-library/): generating scenarios from data, comparing providers and processing results
- [API reference](/arena/reference/api-reference/): the full API
