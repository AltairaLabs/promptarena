package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

func dataScenario(trials int) *arenaconfig.Scenario {
	return &arenaconfig.Scenario{
		Providers: []string{"p1", "p2"},
		Trials:    trials,
		Labels:    map[string]string{"suite": "geo"},
		Turns:     []arenaconfig.TurnDefinition{{Role: "user", Content: "Capital of {country}?"}},
		Data: &arenaconfig.ScenarioData{
			File: "rows.jsonl",
			Rows: []arenaconfig.DataRow{
				{ID: "fr", Values: map[string]string{"country": "France"}},
				{ID: "jp", Values: map[string]string{"country": "Japan"}},
			},
		},
	}
}

func dataEngine(s *arenaconfig.Scenario) *Engine {
	return &Engine{
		config:           &arenaconfig.Config{},
		scenarios:        map[string]*arenaconfig.Scenario{"capitals": s},
		providers:        make(map[string]*config.Provider),
		providerRegistry: providers.NewRegistry(),
	}
}

func TestGenerateScenarioCombinations_DataRows(t *testing.T) {
	e := dataEngine(dataScenario(3))

	combos, err := e.generateScenarioCombinations("r1", "capitals", nil)
	require.NoError(t, err)
	require.Len(t, combos, 2*2*3, "providers x rows x trials")

	for _, c := range combos {
		assert.Equal(t, 3, c.TotalTrials, "trials count within a row, not across rows")
		assert.Equal(t, -1, c.PerturbationIndex)
		assert.Equal(t, e.scenarios["capitals"].Data.Rows[c.DataRowIndex].ID, c.DataRowID)
	}
	assert.Equal(t, "fr", combos[0].DataRowID)
	assert.Equal(t, "jp", combos[3].DataRowID)
}

func TestGroupTrialRuns_GroupsPerDataRow(t *testing.T) {
	e := dataEngine(dataScenario(2))
	combos, err := e.generateScenarioCombinations("r1", "capitals", nil)
	require.NoError(t, err)
	runIDs := make([]string, len(combos))
	for i := range combos {
		runIDs[i] = generateRunID(combos[i], "batch")
	}

	groups := groupTrialRuns(runIDs, combos)

	require.Len(t, groups, 4, "one group per provider and row")
	for _, g := range groups {
		assert.Len(t, g.runIDs, 2)
		assert.NotEmpty(t, g.key.DataRowID)
	}
}

func TestGenerateRunID_DistinguishesDataRows(t *testing.T) {
	fr := RunCombination{Region: "r1", ScenarioID: "capitals", ProviderID: "p1", DataRowID: "fr"}
	jp := fr
	jp.DataRowID = "jp"
	none := fr
	none.DataRowID = ""

	hash := func(id string) string { return id[len(id)-13 : len(id)-5] }
	assert.NotEqual(t, hash(generateRunID(fr, "b")), hash(generateRunID(jp, "b")))
	assert.NotEqual(t, hash(generateRunID(fr, "b")), hash(generateRunID(none, "b")))
}

func TestDataRowScenario(t *testing.T) {
	s := dataScenario(1)

	got := dataRowScenario(s, RunCombination{DataRowID: "jp", DataRowIndex: 1})
	assert.Equal(t, "Capital of Japan?", got.Turns[0].Content)
	assert.Equal(t, "Capital of {country}?", s.Turns[0].Content)

	assert.Same(t, s, dataRowScenario(s, RunCombination{DataRowID: "x", DataRowIndex: 5}))
}

func TestRunLabelsAndParams_DataRow(t *testing.T) {
	e := dataEngine(dataScenario(1))
	combo := RunCombination{ScenarioID: "capitals", DataRowID: "jp", DataRowIndex: 1}

	assert.Equal(t, map[string]string{"suite": "geo", "data_row": "jp"}, e.runLabels(combo))
	assert.Equal(t, map[string]string{"suite": "geo"}, e.scenarios["capitals"].Labels,
		"run labels must not write into the scenario's labels")
	assert.Equal(t, map[string]interface{}{
		"data_row": "jp",
		"data":     map[string]interface{}{"country": "Japan"},
	}, e.runParams(combo))

	plain := RunCombination{ScenarioID: "capitals"}
	assert.Equal(t, map[string]string{"suite": "geo"}, e.runLabels(plain))
	assert.Nil(t, e.runParams(plain))
}

func TestGenerateScenarioCombinations_DataWithoutRowsIsAnError(t *testing.T) {
	s := dataScenario(1)
	s.Data.Rows = nil
	e := dataEngine(s)

	_, err := e.generateScenarioCombinations("r1", "capitals", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data file rows.jsonl has no loaded rows")
}
