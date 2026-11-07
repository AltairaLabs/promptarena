package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/persistence/memory"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// buildMemoryRepo registers the loaded prompt configs in a memory repository.
// A test helper: the spec compliance tests drive PromptKit's compiler directly,
// and the production copy lives in the compiler package.
func buildMemoryRepo(cfg *arenaconfig.Config) (*memory.PromptRepository, error) {
	memRepo := memory.NewPromptRepository()
	for _, promptData := range cfg.LoadedPromptConfigs {
		promptConfig, ok := promptData.Config.(*prompt.Config)
		if !ok {
			return nil, fmt.Errorf("prompt config %s has invalid type", promptData.FilePath)
		}
		if err := memRepo.SavePrompt(promptConfig); err != nil {
			return nil, fmt.Errorf("registering prompt %s: %w", promptData.FilePath, err)
		}
	}
	return memRepo, nil
}

// parsePackEvalsFromConfig returns pack-level eval definitions from arena config.
// Kept as test helper after migration to compiler package.
func parsePackEvalsFromConfig(cfg *arenaconfig.Config) []evals.EvalDef {
	return cfg.PackEvals
}

// parseWorkflowFromConfig parses workflow config from arena config.
// Kept as test helper after migration to compiler package.
func parseWorkflowFromConfig(cfg *arenaconfig.Config) *prompt.WorkflowConfig {
	if cfg.WorkflowSpec == nil {
		return nil
	}
	data, err := json.Marshal(cfg.WorkflowSpec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to marshal workflow config: %v\n", err)
		return nil
	}
	var wf prompt.WorkflowConfig
	if err := json.Unmarshal(data, &wf); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to parse workflow config: %v\n", err)
		return nil
	}
	return &wf
}

// parseAgentsFromConfig parses agents config from arena config.
// Kept as test helper after migration to compiler package.
func parseAgentsFromConfig(cfg *arenaconfig.Config) *prompt.AgentsConfig {
	if cfg.Agents == nil {
		return nil
	}
	data, err := json.Marshal(cfg.Agents)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to marshal agents config: %v\n", err)
		return nil
	}
	var ag prompt.AgentsConfig
	if err := json.Unmarshal(data, &ag); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to parse agents config: %v\n", err)
		return nil
	}
	return &ag
}
