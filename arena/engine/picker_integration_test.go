package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

func TestListProviders(t *testing.T) {
	e := &Engine{
		providers: map[string]*config.Provider{
			"zeta":  {ID: "zeta", Type: "openai", Model: "gpt-4o"},
			"alpha": {ID: "alpha", Type: "anthropic", Model: "claude"},
			"mocky": {ID: "mocky", Type: "mock"},
		},
	}
	got := e.ListProviders()
	require.Len(t, got, 3)
	// Mock providers sort first, then the rest alphabetically by ID.
	assert.Equal(t, "mocky", got[0].ID)
	assert.Equal(t, "alpha", got[1].ID)
	assert.Equal(t, "zeta", got[2].ID)
	assert.Equal(t, "gpt-4o", got[2].Model)
}

func TestListScenarios(t *testing.T) {
	e := &Engine{
		scenarios: map[string]*arenaconfig.Scenario{
			"b": {ID: "b", Description: "second"},
			"a": {ID: "a", Description: "first"},
		},
	}
	got := e.ListScenarios()
	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].ID)
	assert.Equal(t, "first", got[0].Description)
	assert.Equal(t, "b", got[1].ID)
}
