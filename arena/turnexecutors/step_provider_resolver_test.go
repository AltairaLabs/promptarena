package turnexecutors

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/composition"
	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// keyBinding binds logical keys to providers, and nothing else.
type keyBinding map[string]providers.Provider

func (b keyBinding) LLM(key string) (providers.Provider, error) {
	if p, ok := b[key]; ok {
		return p, nil
	}
	return nil, evals.ErrUnboundKey
}

func (b keyBinding) Classifier(string) (any, error) { return nil, evals.ErrUnboundKey }

// Each composition step runs on the provider its RFC 0017 key is bound to:
// the step's own key, else its prompt's, else the scenario's provider
// (promptarena#298). Before, every step ran on the scenario's provider.
func TestStepProviderResolver_RoutesByKey(t *testing.T) {
	scenario := mock.NewProvider("scenario", "m", false)
	judge := mock.NewProvider("judge-model", "m", false)
	writer := mock.NewProvider("writer-model", "m", false)

	var pack prompt.Pack
	require.NoError(t, json.Unmarshal([]byte(`{"id":"p","name":"p","version":"1","prompts":{
		"draft":{"id":"draft","name":"d","version":"1","system_template":"d","provider":"writer"},
		"plain":{"id":"plain","name":"p","version":"1","system_template":"p"}}}`), &pack))

	e := NewPipelineExecutor(nil, nil)
	e.SetProviderBinding(keyBinding{"judge": judge, "writer": writer})
	resolve := e.stepProviderResolver(&TurnRequest{Provider: scenario, CallPack: &pack})

	got, err := resolve(&composition.Step{ID: "s", PromptTask: "plain", Provider: "judge"})
	require.NoError(t, err)
	assert.Same(t, judge, got, "the step's own key")

	got, err = resolve(&composition.Step{ID: "s", PromptTask: "draft"})
	require.NoError(t, err)
	assert.Same(t, writer, got, "the prompt's key")

	got, err = resolve(&composition.Step{ID: "s", PromptTask: "plain"})
	require.NoError(t, err)
	assert.Same(t, scenario, got, "no key: the scenario's provider")

	_, err = resolve(&composition.Step{ID: "s", PromptTask: "plain", Provider: "missing"})
	assert.ErrorIs(t, err, evals.ErrUnboundKey)
	assert.ErrorContains(t, err, `"missing"`)

	unbound := NewPipelineExecutor(nil, nil).stepProviderResolver(&TurnRequest{Provider: scenario})
	_, err = unbound(&composition.Step{ID: "s", Provider: "judge"})
	assert.ErrorIs(t, err, evals.ErrNoBinding)
}

// countingProvider counts the calls it serves.
type countingProvider struct {
	*mock.Provider
	calls atomic.Int32
}

func (p *countingProvider) Predict(ctx context.Context, req providers.PredictionRequest) (providers.PredictionResponse, error) {
	p.calls.Add(1)
	return p.Provider.Predict(ctx, req)
}

func (p *countingProvider) PredictStream(
	ctx context.Context, req providers.PredictionRequest,
) (<-chan providers.StreamChunk, error) {
	p.calls.Add(1)
	return p.Provider.PredictStream(ctx, req)
}

// A composition turn runs a prompt step on the provider its key is bound to,
// not on the scenario's: the resolver is wired into the composition stage.
func TestCompositionTurn_RunsStepOnItsBoundProvider(t *testing.T) {
	scenario := &countingProvider{Provider: mock.NewProvider("scenario", "m", false)}
	judge := &countingProvider{Provider: mock.NewProvider("judge-model", "m", false)}

	e := NewPipelineExecutor(tools.NewRegistry(), nil)
	e.SetProviderBinding(keyBinding{"judge": judge})
	req := &TurnRequest{
		Provider:       scenario,
		Scenario:       &arenaconfig.Scenario{ID: "sc", TaskType: "chat"},
		PromptRegistry: registryWithValidators(nil),
		BaseDir:        t.TempDir(),
		ActiveComposition: &composition.Composition{Version: 1, Steps: []*composition.Step{
			{ID: "grade", Kind: composition.KindPrompt, PromptTask: "chat", Provider: "judge",
				Input: &packspec.StepInput{String: "${input}"}},
		}},
	}
	pipe, err := e.buildStagePipeline(req, map[string]string{})
	require.NoError(t, err)
	_, err = pipe.ExecuteSync(context.Background(),
		stage.StreamElement{Message: &types.Message{Role: "user", Content: "hi"}})
	require.NoError(t, err)

	assert.Equal(t, int32(1), judge.calls.Load(), "the step ran on the provider bound to its key")
	assert.Zero(t, scenario.calls.Load(), "not on the scenario's provider")
}
