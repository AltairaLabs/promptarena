package engine

import (
	"context"
	"fmt"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"
	"github.com/AltairaLabs/PromptKit/runtime/v2/memory"
)

// initMemory sets up the memory subsystem if config.Memory is set.
// Creates an InMemoryStore and registers the memory executor + tools.
// The actual tool execution is real (not mocked) — the InMemoryStore
// persists data across turns within a run.
func (e *Engine) initMemory() error {
	if e.config.Memory == nil {
		return nil
	}

	store := memory.NewInMemoryStore()

	// One executor for the whole engine, constructed with no scope of its own.
	// From v2.3.0 it reads the conversation's scope from the context, which
	// each run binds via runTools.bindContext — so a single registration is
	// safe even though the registry holds exactly one executor per name.
	// Capturing a run's scope on the executor is what used to make one run's
	// memories answer another's reads.
	e.toolRegistry.RegisterExecutor(memory.NewExecutor(store, nil))
	memory.RegisterMemoryTools(e.toolRegistry)

	e.memoryStore = store
	logger.Info("Memory subsystem initialized", "store", "in-memory")
	return nil
}

// seedMemoriesForRun pre-populates the memory store with seed entries from the
// scenario config. Called with the scope buildRunTools bound to this run,
// before the first turn.
func (e *Engine) seedMemoriesForRun(scenario *arenaconfig.Scenario, scope map[string]string) error {
	if e.memoryStore == nil || len(scenario.SeedMemories) == 0 {
		return nil
	}
	ctx := context.Background()
	for i, entry := range scenario.SeedMemories {
		if entry.Content == "" {
			return fmt.Errorf("seed_memories[%d]: empty content", i)
		}
		memType := entry.Type
		if memType == "" {
			memType = "general"
		}
		confidence := entry.Confidence
		if confidence <= 0 {
			confidence = 0.8
		}
		m := &memory.Memory{
			Type:       memType,
			Content:    entry.Content,
			Confidence: confidence,
			Metadata:   entry.Metadata,
			Scope:      scope,
		}
		m.SetProvenance(memory.ProvenanceOperatorCurated)
		if err := e.memoryStore.Save(ctx, m); err != nil {
			return fmt.Errorf("seed_memories[%d]: %w", i, err)
		}
	}
	logger.Info("Seeded memories for scenario", "scenario", scenario.ID, "count", len(scenario.SeedMemories))
	return nil
}
