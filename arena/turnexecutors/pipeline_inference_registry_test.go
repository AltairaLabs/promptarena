package turnexecutors

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/inference"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
)

// A classifier-backed guardrail that names no provider (a topic_policy with no
// provider param) resolves the default classifier from the inference registry
// on the pipeline's context. The pipeline only attaches the registry its config
// carries, so without this the guardrail fails closed on every turn.
func TestPipelineConfig_CarriesTheInferenceRegistry(t *testing.T) {
	reg := inference.NewRegistry()
	e := &PipelineExecutor{}
	e.SetInferenceRegistry(reg)

	if got := e.pipelineConfig(mock.NewProvider("p", "m", false)).InferenceRegistry; got != reg {
		t.Fatalf("InferenceRegistry = %p, want the executor's registry %p", got, reg)
	}
	if got := (&PipelineExecutor{}).pipelineConfig(nil).InferenceRegistry; got != nil {
		t.Fatalf("an executor with no registry must leave it nil, got %p", got)
	}
}
