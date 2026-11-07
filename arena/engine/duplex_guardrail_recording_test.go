package engine

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
)

// oneConfigRepository serves one prompt config for any task type.
type oneConfigRepository struct{ config *prompt.Config }

func (r *oneConfigRepository) LoadPrompt(string) (*prompt.Config, error) { return r.config, nil }
func (r *oneConfigRepository) LoadFragment(_, _, _ string) (*prompt.Fragment, error) {
	return nil, nil
}
func (r *oneConfigRepository) ListPrompts() ([]string, error)  { return nil, nil }
func (r *oneConfigRepository) SavePrompt(*prompt.Config) error { return nil }

func guardedPromptRegistry(validators []prompt.ValidatorConfig) *prompt.Registry {
	return prompt.NewRegistryWithRepository(&oneConfigRepository{config: &prompt.Config{
		APIVersion: "promptkit.altairalabs.ai/v1alpha1",
		Kind:       "PromptConfig",
		Spec: prompt.Spec{
			TaskType:       "voice",
			Version:        "v1.0.0",
			SystemTemplate: "You are a voice agent.",
			Validators:     validators,
		},
	}})
}

// memoryEventStore keeps appended events in memory.
type memoryEventStore struct {
	events.EventStore
	mu  sync.Mutex
	got []*events.Event
}

func (m *memoryEventStore) Append(_ context.Context, e *events.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.got = append(m.got, e)
	return nil
}

func (m *memoryEventStore) messages() []*events.MessageCreatedData {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*events.MessageCreatedData
	for _, e := range m.got {
		if d, ok := e.Data.(*events.MessageCreatedData); ok {
			out = append(out, d)
		}
	}
	return out
}

// TestDuplexConversation_GuardrailsAndRecording runs a real duplex
// conversation through ExecuteConversation and checks the two things the
// duplex pipeline used to lack: the pack's output guardrail enforces on the
// assistant turn (and records the firing guardrail_triggered reads), and the
// session recording holds the assistant message, with the enforced content.
func TestDuplexConversation_GuardrailsAndRecording(t *testing.T) {
	provider := mock.NewStreamingProvider("mock-duplex", "mock-model", false)
	provider.WithAutoRespond("the secret code is 42")

	reg := guardedPromptRegistry([]prompt.ValidatorConfig{
		{Type: "banned_words", Params: map[string]interface{}{"words": []interface{}{"secret"}}},
	})
	executor := NewDuplexConversationExecutor(nil, reg, nil, nil, nil)

	store := statestore.NewArenaStateStore()
	rec := &memoryEventStore{}
	recCfg := stage.DefaultRecordingStageConfig()
	conversationID := "duplex-guard-" + time.Now().Format("150405.000000")
	req := ConversationRequest{
		Provider: provider,
		Scenario: &arenaconfig.Scenario{
			ID:       "guarded",
			TaskType: "voice",
			Duplex:   &arenaconfig.DuplexConfig{Timeout: "10s"},
			Turns: []arenaconfig.TurnDefinition{{
				Role: "user",
				Parts: []arenaconfig.TurnContentPart{{
					Type:  "audio",
					Media: &arenaconfig.TurnMediaContent{FilePath: "testdata/test.pcm", MIMEType: "audio/L16"},
				}},
			}},
		},
		Config:           &arenaconfig.Config{LoadedProviders: map[string]*config.Provider{}},
		RunID:            "run",
		ConversationID:   conversationID,
		StateStoreConfig: &StateStoreConfig{Store: store},
		RecordingConfig:  &recCfg,
		EventStore:       rec,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result := executor.ExecuteConversation(ctx, req)
	if result.Failed {
		t.Fatalf("conversation failed: %s", result.Error)
	}

	state, err := store.Load(ctx, conversationID)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	var assistant int
	for _, m := range state.Messages {
		if m.Role != roleAssistant {
			continue
		}
		assistant++
		if strings.Contains(m.Content, "secret") {
			t.Fatalf("assistant turn kept the banned word: %q", m.Content)
		}
		if len(m.Validations) == 0 || m.Validations[0].ValidatorType != "banned_words" {
			t.Fatalf("assistant turn must record the banned_words firing, got %+v", m.Validations)
		}
	}
	if assistant == 0 {
		t.Fatal("no assistant turn reached the state store")
	}

	var recorded int
	for _, d := range rec.messages() {
		if d.Role != roleAssistant {
			continue
		}
		recorded++
		if strings.Contains(d.Content, "secret") {
			t.Fatalf("recording kept the banned word: %q", d.Content)
		}
	}
	if recorded != assistant {
		t.Fatalf("want every assistant turn recorded once (%d), got %d", assistant, recorded)
	}
}

func TestBuildDuplexGuardrailStage(t *testing.T) {
	provider := mock.NewStreamingProvider("p", "m", false)
	req := &ConversationRequest{
		Scenario: &arenaconfig.Scenario{TaskType: "voice", Duplex: &arenaconfig.DuplexConfig{}},
		Config:   &arenaconfig.Config{},
	}

	none := NewDuplexConversationExecutor(nil, guardedPromptRegistry(nil), nil, nil, nil)
	if s, err := none.buildDuplexGuardrailStage(req, provider, nil, nil); s != nil || err != nil {
		t.Fatalf("no validators: want no stage, got %v, %v", s, err)
	}

	bad := NewDuplexConversationExecutor(nil, guardedPromptRegistry([]prompt.ValidatorConfig{
		{Type: "no_such_validator"},
	}), nil, nil, nil)
	if _, err := bad.buildDuplexGuardrailStage(req, provider, nil, nil); err == nil {
		t.Fatal("an unregistered validator type must fail the pipeline build")
	}
	if _, err := bad.buildDuplexPipeline(req, provider); err == nil {
		t.Fatal("buildDuplexPipeline must surface the guardrail error")
	}

	if s, err := bad.buildDuplexGuardrailStage(&ConversationRequest{}, provider, nil, nil); s != nil || err != nil {
		t.Fatal("no scenario: no task, so no guardrails")
	}
}

func TestBuildDuplexPipelineConfig_AttachesProviderBinding(t *testing.T) {
	orch := &EvalOrchestrator{}
	orch.SetProviderBinding(stubBinding{})
	de := NewDuplexConversationExecutor(nil, nil, nil, nil, orch)
	if de.buildDuplexPipelineConfig(&ConversationRequest{}).ProviderBinding == nil {
		t.Fatal("the eval binding must reach the duplex pipeline so judge-backed guardrails resolve")
	}
	if NewDuplexConversationExecutor(nil, nil, nil, nil, nil).
		buildDuplexPipelineConfig(&ConversationRequest{}).ProviderBinding != nil {
		t.Fatal("no orchestrator: no binding")
	}
}

// stubBinding is an empty provider binding.
type stubBinding struct{}

func (stubBinding) LLM(string) (providers.Provider, error) { return nil, nil }
func (stubBinding) Classifier(string) (any, error)         { return nil, nil }
