package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

func TestGetFragmentNames(t *testing.T) {
	t.Run("empty fragments", func(t *testing.T) {
		fragments := map[string]string{}
		names := getFragmentNames(fragments)
		assert.Empty(t, names)
	})

	t.Run("multiple fragments", func(t *testing.T) {
		fragments := map[string]string{
			"intro":  "{{intro}}",
			"outro":  "{{outro}}",
			"middle": "{{middle}}",
		}
		names := getFragmentNames(fragments)
		assert.Len(t, names, 3)
		assert.Contains(t, names, "intro")
		assert.Contains(t, names, "outro")
		assert.Contains(t, names, "middle")
	})
}

func TestMustLoadConfig_Error(t *testing.T) {
	// Test that the underlying function returns error for nonexistent file
	_, err := arenaconfig.LoadConfig("nonexistent-file.yaml")
	assert.Error(t, err)
}

func TestValidateWorkflowInPack(t *testing.T) {
	t.Run("valid workflow no errors", func(t *testing.T) {
		p := &prompt.Pack{Pack: packspec.Pack{
			Prompts: map[string]*prompt.PackPrompt{
				"greeting": {ID: "greeting"},
				"farewell": {ID: "farewell"},
			},
			Workflow: &prompt.WorkflowConfig{
				Version: 1,
				Entry:   "start",
				States: map[string]*prompt.WorkflowState{
					"start": {PromptTask: "greeting", OnEvent: map[string]string{"Done": "end"}},
					"end":   {PromptTask: "farewell"},
				},
			},
		}}
		result := p.ValidateWorkflow()
		assert.False(t, result.HasErrors())
	})

	t.Run("workflow errors are blocking", func(t *testing.T) {
		p := &prompt.Pack{Pack: packspec.Pack{
			Prompts: map[string]*prompt.PackPrompt{
				"greeting": {ID: "greeting"},
			},
			Workflow: &prompt.WorkflowConfig{
				Version: 1,
				Entry:   "start",
				States: map[string]*prompt.WorkflowState{
					"start": {PromptTask: "greeting", OnEvent: map[string]string{"Done": "ghost"}},
				},
			},
		}}
		result := p.ValidateWorkflow()
		assert.True(t, result.HasErrors())
	})

	t.Run("workflow warnings are non-blocking", func(t *testing.T) {
		p := &prompt.Pack{Pack: packspec.Pack{
			Prompts: map[string]*prompt.PackPrompt{
				"greeting": {ID: "greeting"},
				"farewell": {ID: "farewell"},
			},
			Workflow: &prompt.WorkflowConfig{
				Version: 1,
				Entry:   "start",
				States: map[string]*prompt.WorkflowState{
					"start": {PromptTask: "greeting", OnEvent: map[string]string{
						"Done":       "end",
						"not_pascal": "end", // PascalCase warning
					}},
					"end": {PromptTask: "farewell"},
				},
			},
		}}
		result := p.ValidateWorkflow()
		assert.False(t, result.HasErrors())
		assert.NotEmpty(t, result.Warnings)
	})

	t.Run("nil workflow returns empty result", func(t *testing.T) {
		p := &prompt.Pack{}
		result := p.ValidateWorkflow()
		assert.False(t, result.HasErrors())
		assert.Empty(t, result.Warnings)
	})
}

func TestGetFragmentNames_Deterministic(t *testing.T) {
	fragments := map[string]string{
		"zebra":   "z",
		"alpha":   "a",
		"middle":  "m",
		"beta":    "b",
		"omega":   "o",
		"delta":   "d",
		"gamma":   "g",
		"epsilon": "e",
		"zeta":    "z2",
		"iota":    "i",
		"kappa":   "k",
		"lambda":  "l",
		"mu":      "mu",
		"nu":      "nu",
		"omicron": "oi",
		"pi":      "pi",
		"rho":     "rho",
		"sigma":   "sig",
		"tau":     "tau",
		"upsilon": "ups",
	}

	// Run 20 times to catch non-determinism (Go map iteration is random)
	const iterations = 20
	first := getFragmentNames(fragments)
	for i := 1; i < iterations; i++ {
		result := getFragmentNames(fragments)
		assert.Equal(t, first, result, "iteration %d produced different order", i)
	}
}

func TestPrintPrompts_Deterministic(t *testing.T) {
	pack := &prompt.Pack{Pack: packspec.Pack{
		Prompts: map[string]*prompt.PackPrompt{
			"zebra":   {ID: "zebra", Name: "Zebra", Version: "1.0"},
			"alpha":   {ID: "alpha", Name: "Alpha", Version: "1.0"},
			"middle":  {ID: "middle", Name: "Middle", Version: "1.0"},
			"beta":    {ID: "beta", Name: "Beta", Version: "1.0"},
			"delta":   {ID: "delta", Name: "Delta", Version: "1.0"},
			"gamma":   {ID: "gamma", Name: "Gamma", Version: "1.0"},
			"epsilon": {ID: "epsilon", Name: "Epsilon", Version: "1.0"},
		},
	}}

	const iterations = 20
	captureOutput := func() string {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		printPrompts(pack)
		w.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		os.Stdout = old
		return buf.String()
	}

	first := captureOutput()
	for i := 1; i < iterations; i++ {
		result := captureOutput()
		assert.Equal(t, first, result, "iteration %d produced different output", i)
	}
}

func TestVersionIsOverridable(t *testing.T) {
	// version should be a var (not const) so ldflags can override it
	original := version
	version = "v1.2.3-test"
	assert.Equal(t, "v1.2.3-test", version)
	version = original

	// Verify default value
	assert.Equal(t, "dev", original)
}

func TestPrintWorkflow_Deterministic(t *testing.T) {
	pack := &prompt.Pack{Pack: packspec.Pack{
		Workflow: &prompt.WorkflowConfig{
			Version: 1,
			Entry:   "start",
			States: map[string]*prompt.WorkflowState{
				"start":  {PromptTask: "greeting", OnEvent: map[string]string{"Done": "middle", "Error": "end"}},
				"middle": {PromptTask: "process", OnEvent: map[string]string{"Next": "end", "Back": "start"}},
				"end":    {PromptTask: "farewell"},
				"alpha":  {PromptTask: "a"},
				"zebra":  {PromptTask: "z"},
			},
		},
	}}

	const iterations = 20
	captureOutput := func() string {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		printWorkflow(pack)
		w.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		os.Stdout = old
		return buf.String()
	}

	first := captureOutput()
	for i := 1; i < iterations; i++ {
		result := captureOutput()
		assert.Equal(t, first, result, "iteration %d produced different output", i)
	}
}

func TestWritePackFile_CreatesMissingParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "nested", "deeper", "out.pack.json")
	data := []byte(`{"id":"test"}`)

	require.NoError(t, writePackFile(outPath, data))

	got, err := os.ReadFile(outPath) //nolint:gosec // test file under t.TempDir
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestWritePackFile_ExistingDir(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out.pack.json")
	data := []byte(`{"id":"test"}`)

	require.NoError(t, writePackFile(outPath, data))

	got, err := os.ReadFile(outPath) //nolint:gosec // test file under t.TempDir
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestPrintAgents_Deterministic(t *testing.T) {
	pack := &prompt.Pack{Pack: packspec.Pack{
		Agents: &prompt.AgentsConfig{
			Entry: "triage",
			Members: map[string]*prompt.AgentDef{
				"triage":  {Description: "Triage"},
				"billing": {Description: "Billing"},
				"support": {Description: "Support"},
				"alpha":   {Description: "Alpha"},
				"zebra":   {Description: "Zebra"},
			},
		},
	}}

	const iterations = 20
	captureOutput := func() string {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		printAgents(pack)
		w.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		os.Stdout = old
		return buf.String()
	}

	first := captureOutput()
	for i := 1; i < iterations; i++ {
		result := captureOutput()
		assert.Equal(t, first, result, "iteration %d produced different output", i)
	}
}
