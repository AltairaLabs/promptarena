package engine

import (
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/composition"
	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
	"github.com/AltairaLabs/PromptKit/runtime/v2/workflow"
)

// minimalEngineForWorkflow builds an Engine directly (bypassing NewEngine's
// side-effecting buildMediaStorage/buildStateStore) with just the fields that
// initWorkflow reads. Used by inline-composition tests only.
func minimalEngineForWorkflow(cfg *arenaconfig.Config) *Engine {
	return &Engine{
		config:       cfg,
		toolRegistry: tools.NewRegistry(),
		// promptRegistry is not used by initWorkflow; nil is safe here.
		promptRegistry: (*prompt.Registry)(nil),
	}
}

// startOnlyWorkflow is a one-state workflow: enough for initWorkflow, which
// returns early when config.Workflow is nil.
func startOnlyWorkflow() *workflow.Spec {
	return &workflow.Spec{
		Version: 1,
		Entry:   "start",
		States:  map[string]*workflow.State{"start": {PromptTask: "start-task"}},
	}
}

// TestInitWorkflow_InlineCompositionsLoaded verifies that when config.Compositions
// is set (the inline Arena path), initWorkflow merges them into
// config.LoadedPack.Compositions so that buildCompositionResolver can find them
// at turn time. This covers the fix in execution_workflow_integration.go.
func TestInitWorkflow_InlineCompositionsLoaded(t *testing.T) {
	// Minimal workflow spec — needed because initWorkflow returns early if
	// config.Workflow is nil.
	orchestration := workflow.OrchestrationComposition
	spec := &workflow.Spec{
		Version: 1,
		Entry:   "start",
		States: map[string]*workflow.State{
			"start": {
				PromptTask:    "start-task",
				Orchestration: &orchestration,
				Composition:   "flow",
			},
		},
	}

	// Inline compositions: one named "flow" with a single tool step.
	comps := map[string]*composition.Composition{
		"flow": {
			Version: 1,
			Steps:   []*composition.Step{{ID: "step1", Kind: composition.KindTool, Tool: "echo"}},
		},
	}

	cfg := &arenaconfig.Config{
		Workflow:     spec,
		Compositions: comps,
	}

	eng := minimalEngineForWorkflow(cfg)

	if err := eng.initWorkflow(); err != nil {
		t.Fatalf("initWorkflow() returned unexpected error: %v", err)
	}

	// LoadedPack must have been created and populated.
	if cfg.LoadedPack == nil {
		t.Fatal("initWorkflow() did not create config.LoadedPack when Compositions was set")
	}
	if cfg.LoadedPack.Compositions == nil {
		t.Fatal("initWorkflow() did not populate config.LoadedPack.Compositions")
	}
	comp, ok := cfg.LoadedPack.Compositions["flow"]
	if !ok || comp == nil {
		t.Fatalf("initWorkflow() did not add 'flow' to LoadedPack.Compositions; got: %v",
			cfg.LoadedPack.Compositions)
	}
	if len(comp.Steps) != 1 || comp.Steps[0].ID != "step1" {
		t.Errorf("composition 'flow' has unexpected steps: %+v", comp.Steps)
	}
}

// TestInitWorkflow_NoCompositions verifies that when config.Compositions is nil,
// initWorkflow does not create a LoadedPack (no-op path).
func TestInitWorkflow_NoCompositions(t *testing.T) {
	spec := startOnlyWorkflow()

	cfg := &arenaconfig.Config{
		Workflow:     spec,
		Compositions: nil,
	}

	eng := minimalEngineForWorkflow(cfg)

	if err := eng.initWorkflow(); err != nil {
		t.Fatalf("initWorkflow() returned unexpected error: %v", err)
	}

	// LoadedPack should not have been created when Compositions is nil.
	if cfg.LoadedPack != nil {
		t.Errorf("initWorkflow() unexpectedly created LoadedPack when Compositions was nil: %+v", cfg.LoadedPack)
	}
}

// TestInitWorkflow_MergesIntoExistingLoadedPack verifies that when config.LoadedPack
// is already populated (e.g. from a compiled pack file), initWorkflow merges inline
// compositions into the existing map without replacing it.
func TestInitWorkflow_MergesIntoExistingLoadedPack(t *testing.T) {
	spec := startOnlyWorkflow()

	comps := map[string]*composition.Composition{
		"inline-flow": {
			Version: 1,
			Steps:   []*composition.Step{{ID: "s1", Kind: composition.KindTool, Tool: "echo"}},
		},
	}

	existingPack := &prompt.Pack{Pack: packspec.Pack{
		ID: "pre-existing",
	}}

	cfg := &arenaconfig.Config{
		Workflow:     spec,
		Compositions: comps,
		LoadedPack:   existingPack,
	}

	eng := minimalEngineForWorkflow(cfg)

	if err := eng.initWorkflow(); err != nil {
		t.Fatalf("initWorkflow() returned unexpected error: %v", err)
	}

	// The existing pack should still be the same pointer.
	if cfg.LoadedPack != existingPack {
		t.Error("initWorkflow() replaced config.LoadedPack instead of merging into it")
	}
	if cfg.LoadedPack.Compositions["inline-flow"] == nil {
		t.Error("initWorkflow() did not merge 'inline-flow' into existing LoadedPack.Compositions")
	}
}
