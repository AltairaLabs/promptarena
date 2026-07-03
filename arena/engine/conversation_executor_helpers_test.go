package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AltairaLabs/PromptKit/runtime/composition"
	"github.com/AltairaLabs/PromptKit/runtime/pipeline/stage"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

func TestResolveTurnTemperature(t *testing.T) {
	t.Run("override wins", func(t *testing.T) {
		temp := 0.7
		req := ConversationRequest{
			Config:      &arenaconfig.Config{Defaults: arenaconfig.Defaults{Temperature: 0.3}},
			Temperature: &temp,
		}
		assert.InDelta(t, 0.7, resolveTurnTemperature(req), 1e-6)
	})

	t.Run("falls back to config default", func(t *testing.T) {
		req := ConversationRequest{
			Config: &arenaconfig.Config{Defaults: arenaconfig.Defaults{Temperature: 0.3}},
		}
		assert.InDelta(t, 0.3, resolveTurnTemperature(req), 1e-6)
	})

	t.Run("zero config default passes zero", func(t *testing.T) {
		req := ConversationRequest{Config: &arenaconfig.Config{}}
		assert.InDelta(t, 0.0, resolveTurnTemperature(req), 1e-6)
	})
}

func TestResolveTurnMaxTokens(t *testing.T) {
	t.Run("override wins", func(t *testing.T) {
		maxT := 128
		req := ConversationRequest{
			Config:    &arenaconfig.Config{Defaults: arenaconfig.Defaults{MaxTokens: 64}},
			MaxTokens: &maxT,
		}
		assert.Equal(t, 128, resolveTurnMaxTokens(req))
	})

	t.Run("falls back to config default", func(t *testing.T) {
		req := ConversationRequest{
			Config: &arenaconfig.Config{Defaults: arenaconfig.Defaults{MaxTokens: 64}},
		}
		assert.Equal(t, 64, resolveTurnMaxTokens(req))
	})
}

func TestBuildTurnPromptVars(t *testing.T) {
	t.Run("nil when no config vars and no scenario vars", func(t *testing.T) {
		req := ConversationRequest{
			Config:   &arenaconfig.Config{},
			Scenario: &arenaconfig.Scenario{TaskType: "task"},
		}
		assert.Nil(t, buildTurnPromptVars(req))
	})

	t.Run("matches config vars by task type", func(t *testing.T) {
		req := ConversationRequest{
			Config: &arenaconfig.Config{
				LoadedPromptConfigs: map[string]*arenaconfig.PromptConfigData{
					"other": {TaskType: "other", Vars: map[string]string{"x": "no"}},
					"task":  {TaskType: "task", Vars: map[string]string{"x": "yes"}},
				},
			},
			Scenario: &arenaconfig.Scenario{TaskType: "task"},
		}
		got := buildTurnPromptVars(req)
		assert.Equal(t, "yes", got["x"])
	})

	t.Run("scenario vars override config vars", func(t *testing.T) {
		req := ConversationRequest{
			Config: &arenaconfig.Config{
				LoadedPromptConfigs: map[string]*arenaconfig.PromptConfigData{
					"task": {TaskType: "task", Vars: map[string]string{"x": "config", "y": "config"}},
				},
			},
			Scenario: &arenaconfig.Scenario{TaskType: "task", Variables: map[string]string{"x": "scenario"}},
		}
		got := buildTurnPromptVars(req)
		assert.Equal(t, "scenario", got["x"])
		assert.Equal(t, "config", got["y"])
	})

	t.Run("does not mutate shared config vars", func(t *testing.T) {
		shared := map[string]string{"x": "config"}
		req := ConversationRequest{
			Config: &arenaconfig.Config{
				LoadedPromptConfigs: map[string]*arenaconfig.PromptConfigData{
					"task": {TaskType: "task", Vars: shared},
				},
			},
			Scenario: &arenaconfig.Scenario{TaskType: "task", Variables: map[string]string{"x": "scenario"}},
		}
		_ = buildTurnPromptVars(req)
		assert.Equal(t, "config", shared["x"], "shared LoadedPromptConfigs vars must not be mutated")
	})
}

func TestResolveActiveComposition(t *testing.T) {
	t.Run("nil resolver returns nil", func(t *testing.T) {
		assert.Nil(t, resolveActiveComposition(ConversationRequest{}))
	})

	t.Run("resolver value is returned", func(t *testing.T) {
		req := ConversationRequest{ActiveCompositionResolver: func() *composition.Composition { return nil }}
		assert.Nil(t, resolveActiveComposition(req))
	})

	t.Run("recorder reset is nil-safe with a real recorder", func(t *testing.T) {
		rec := stage.NewCompositionRecorder()
		req := ConversationRequest{CompositionRecorder: rec}
		assert.NotPanics(t, func() { resolveActiveComposition(req) })
	})
}
