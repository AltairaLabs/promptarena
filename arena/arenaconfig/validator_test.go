package arenaconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/config"
)

func TestNewConfigValidator(t *testing.T) {
	cfg := &Config{}
	validator := NewConfigValidatorWithPath(cfg, "")

	if validator == nil {
		t.Fatal("Expected non-nil validator")
	}

	if validator.config != cfg {
		t.Error("Config reference mismatch")
	}

	if validator.errors == nil {
		t.Error("Expected initialized errors slice")
	}

	if validator.warns == nil {
		t.Error("Expected initialized warns slice")
	}
}

func TestConfigValidator_GetWarnings(t *testing.T) {
	cfg := &Config{}
	validator := NewConfigValidatorWithPath(cfg, "")

	warnings := validator.GetWarnings()
	if len(warnings) != 0 {
		t.Error("Expected no warnings initially")
	}

	validator.warns = append(validator.warns, "test warning")
	warnings = validator.GetWarnings()

	if len(warnings) != 1 {
		t.Error("Expected 1 warning")
	}
}

func TestConfigValidator_Validate_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	validator := NewConfigValidatorWithPath(cfg, "")

	err := validator.Validate()

	// Empty config should pass validation (just warnings)
	if err != nil {
		t.Errorf("Expected no error for empty config, got: %v", err)
	}

	warnings := validator.GetWarnings()
	if len(warnings) == 0 {
		t.Error("Expected warnings for empty config")
	}
}

func TestConfigValidator_ValidatePromptConfigs_NoDuplicates(t *testing.T) {
	tmpDir := t.TempDir()

	// Create prompt config files
	file1 := filepath.Join(tmpDir, "prompt1.yaml")
	file2 := filepath.Join(tmpDir, "prompt2.yaml")

	for _, f := range []string{file1, file2} {
		if err := os.WriteFile(f, []byte("test: content\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &Config{
		Defaults: Defaults{
			ConfigDir: tmpDir,
		},
		PromptConfigs: []PromptConfigRef{
			{ID: "prompt1", File: "prompt1.yaml"},
			{ID: "prompt2", File: "prompt2.yaml"},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validatePromptConfigs()

	if len(validator.errors) > 0 {
		t.Errorf("Expected no errors, got: %v", validator.errors)
	}
}

func TestConfigValidator_ValidatePromptConfigs_DuplicateIDs(t *testing.T) {
	cfg := &Config{
		PromptConfigs: []PromptConfigRef{
			{ID: "duplicate"},
			{ID: "duplicate"},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validatePromptConfigs()

	if len(validator.errors) == 0 {
		t.Error("Expected error for duplicate IDs")
	}
}

func TestConfigValidator_ValidatePromptConfigs_MissingFile(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{
			ConfigDir: "/nonexistent",
		},
		PromptConfigs: []PromptConfigRef{
			{ID: "test", File: "nonexistent.yaml"},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validatePromptConfigs()

	if len(validator.errors) == 0 {
		t.Error("Expected error for missing file")
	}
}

func TestConfigValidator_ValidatePromptConfigs_NoFile(t *testing.T) {
	cfg := &Config{
		PromptConfigs: []PromptConfigRef{
			{ID: "test"}, // No file specified
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validatePromptConfigs()

	warnings := validator.GetWarnings()
	if len(warnings) == 0 {
		t.Error("Expected warning for missing file")
	}
}

func TestConfigValidator_ValidateProviders_Valid(t *testing.T) {
	tmpDir := t.TempDir()

	providerFile := filepath.Join(tmpDir, "provider.yaml")
	if err := os.WriteFile(providerFile, []byte("test: content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Providers: []ProviderRef{
			{File: providerFile},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateProviders()

	if len(validator.errors) > 0 {
		t.Errorf("Expected no errors, got: %v", validator.errors)
	}
}

func TestConfigValidator_ValidateProviders_MissingFile(t *testing.T) {
	cfg := &Config{
		Providers: []ProviderRef{
			{File: "/nonexistent/provider.yaml"},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateProviders()

	if len(validator.errors) == 0 {
		t.Error("Expected error for missing provider file")
	}
}

func TestConfigValidator_ValidateProviders_DuplicateFiles(t *testing.T) {
	tmpDir := t.TempDir()

	providerFile := filepath.Join(tmpDir, "provider.yaml")
	if err := os.WriteFile(providerFile, []byte("test: content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Providers: []ProviderRef{
			{File: providerFile},
			{File: providerFile}, // Duplicate
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateProviders()

	warnings := validator.GetWarnings()
	if len(warnings) == 0 {
		t.Error("Expected warning for duplicate provider files")
	}
}

func TestConfigValidator_ValidateScenarios_Valid(t *testing.T) {
	tmpDir := t.TempDir()

	scenarioFile := filepath.Join(tmpDir, "scenario.yaml")
	scenarioContent := `id: test
description: Test scenario
turns:
  - type: user
    content: Hello
`
	if err := os.WriteFile(scenarioFile, []byte(scenarioContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Scenarios: []ScenarioRef{
			{File: scenarioFile},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateScenarios()

	if len(validator.errors) > 0 {
		t.Errorf("Expected no errors, got: %v", validator.errors)
	}
}

func TestConfigValidator_ValidateScenarios_MissingFile(t *testing.T) {
	cfg := &Config{
		Scenarios: []ScenarioRef{
			{File: "/nonexistent/scenario.yaml"},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateScenarios()

	if len(validator.errors) == 0 {
		t.Error("Expected error for missing scenario file")
	}
}

func TestConfigValidator_ValidateScenarios_Empty(t *testing.T) {
	cfg := &Config{}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateScenarios()

	warnings := validator.GetWarnings()
	if len(warnings) == 0 {
		t.Error("Expected warning for no scenarios")
	}
}

func TestConfigValidator_ValidatePersonas_Valid(t *testing.T) {
	tmpDir := t.TempDir()

	personaFile := filepath.Join(tmpDir, "persona.yaml")
	personaContent := `id: test
system_prompt: Test
`
	if err := os.WriteFile(personaFile, []byte(personaContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		SelfPlay: &SelfPlayConfig{
			Personas: []PersonaRef{
				{File: personaFile},
			},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validatePersonas()

	if len(validator.errors) > 0 {
		t.Errorf("Expected no errors, got: %v", validator.errors)
	}
}

func TestConfigValidator_ValidatePersonas_MissingFile(t *testing.T) {
	cfg := &Config{
		SelfPlay: &SelfPlayConfig{
			Personas: []PersonaRef{
				{File: "/nonexistent/persona.yaml"},
			},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validatePersonas()

	if len(validator.errors) == 0 {
		t.Error("Expected error for missing persona file")
	}
}

func TestConfigValidator_ValidateSelfPlay_Disabled(t *testing.T) {
	disabled := false
	cfg := &Config{
		SelfPlay: &SelfPlayConfig{
			Enabled: &disabled,
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateSelfPlay()

	// Disabled self-play should not produce errors
	if len(validator.errors) > 0 {
		t.Errorf("Expected no errors for disabled self-play, got: %v", validator.errors)
	}
}

func TestConfigValidator_ValidateSelfPlay_NoRoles(t *testing.T) {
	cfg := &Config{
		SelfPlay: &SelfPlayConfig{
			Roles: []SelfPlayRoleGroup{},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateSelfPlay()

	warnings := validator.GetWarnings()
	if len(warnings) == 0 {
		t.Error("Expected warning for enabled self-play with no roles")
	}
}

func TestConfigValidator_ValidateSelfPlay_DuplicateRoleIDs(t *testing.T) {
	tmpDir := t.TempDir()

	providerFile := filepath.Join(tmpDir, "provider.yaml")
	if err := os.WriteFile(providerFile, []byte("test: content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		SelfPlay: &SelfPlayConfig{
			Roles: []SelfPlayRoleGroup{
				{ID: "role1", Provider: "openai-gpt4o-mini"},
				{ID: "role1", Provider: "claude-haiku"}, // Duplicate ID
			},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateSelfPlay()

	if len(validator.errors) == 0 {
		t.Error("Expected error for duplicate role IDs")
	}
}

func TestConfigValidator_ValidateSelfPlay_MissingProvider(t *testing.T) {
	cfg := &Config{
		SelfPlay: &SelfPlayConfig{
			Roles: []SelfPlayRoleGroup{
				{ID: "role1"}, // Missing Provider
			},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateSelfPlay()

	if len(validator.errors) == 0 {
		t.Error("Expected error for missing provider")
	}
}

func TestConfigValidator_Validate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		PromptConfigs: []PromptConfigRef{
			{ID: "test", File: "/nonexistent1.yaml"},
		},
		Providers: []ProviderRef{
			{File: "/nonexistent2.yaml"},
		},
		Scenarios: []ScenarioRef{
			{File: "/nonexistent3.yaml"},
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	err := validator.Validate()

	// Should have multiple errors
	if err == nil {
		t.Error("Expected error for multiple validation failures")
	}

	if len(validator.errors) < 3 {
		t.Errorf("Expected at least 3 errors, got %d", len(validator.errors))
	}
}

func TestConfigValidator_ValidatePromptConfigs_Empty(t *testing.T) {
	cfg := &Config{}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validatePromptConfigs()

	warnings := validator.GetWarnings()
	if len(warnings) == 0 {
		t.Error("Expected warning for no prompt configs")
	}
}

func TestConfigValidator_ValidateProviders_Empty(t *testing.T) {
	cfg := &Config{}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateProviders()

	warnings := validator.GetWarnings()
	if len(warnings) == 0 {
		t.Error("Expected warning for no providers")
	}
}

func TestConfigValidator_ValidatePersonas_Empty(t *testing.T) {
	cfg := &Config{}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validatePersonas()

	warnings := validator.GetWarnings()
	if len(warnings) == 0 {
		t.Error("Expected warning for no personas")
	}
}

func TestConfigValidator_ValidateSelfPlay_Nil(t *testing.T) {
	cfg := &Config{
		SelfPlay: nil,
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validateSelfPlay()

	// Nil self-play should not produce errors
	if len(validator.errors) > 0 {
		t.Errorf("Expected no errors for nil self-play, got: %v", validator.errors)
	}
}

func TestConfigValidator_ValidatePromptConfigs_MissingID(t *testing.T) {
	cfg := &Config{
		PromptConfigs: []PromptConfigRef{
			{File: "test.yaml"}, // No ID
		},
	}

	validator := NewConfigValidatorWithPath(cfg, "")
	validator.validatePromptConfigs()

	if len(validator.errors) == 0 {
		t.Error("Expected error for missing ID")
	}
}

// stubAllowedTools implements the duck-typed GetAllowedTools() the validator
// looks for on a loaded prompt config.
type stubAllowedTools struct{ tools []string }

func (s stubAllowedTools) GetAllowedTools() []string { return s.tools }

func promptCfgs(tools ...string) map[string]*PromptConfigData {
	return map[string]*PromptConfigData{
		"p": {Config: stubAllowedTools{tools: tools}},
	}
}

func TestValidateAllowedToolsResolve(t *testing.T) {
	staticServer := []config.MCPServerConfig{{Name: "memory", Command: "npx"}}

	t.Run("bare MCP names on a static-only server warn", func(t *testing.T) {
		cfg := &Config{MCPServers: staticServer, LoadedPromptConfigs: promptCfgs("create_entities", "read_graph")}
		v := NewConfigValidator(cfg)
		v.validateAllowedToolsResolve()
		require.Len(t, v.GetWarnings(), 2)
		assert.Contains(t, v.GetWarnings()[0], `mcp__memory__`)
	})

	t.Run("qualified MCP names do not warn", func(t *testing.T) {
		cfg := &Config{MCPServers: staticServer, LoadedPromptConfigs: promptCfgs("mcp__memory__create_entities")}
		v := NewConfigValidator(cfg)
		v.validateAllowedToolsResolve()
		assert.Empty(t, v.GetWarnings())
	})

	t.Run("mcp reference to unconfigured server warns", func(t *testing.T) {
		cfg := &Config{MCPServers: staticServer, LoadedPromptConfigs: promptCfgs("mcp__typo__read_graph")}
		v := NewConfigValidator(cfg)
		v.validateAllowedToolsResolve()
		require.Len(t, v.GetWarnings(), 1)
		assert.Contains(t, v.GetWarnings()[0], "not configured")
	})

	t.Run("source-backed server leaves bare names alone", func(t *testing.T) {
		cfg := &Config{
			MCPServers:          []config.MCPServerConfig{{Name: "sandbox", Source: "docker"}},
			LoadedPromptConfigs: promptCfgs("Read", "Write"),
		}
		v := NewConfigValidator(cfg)
		v.validateAllowedToolsResolve()
		assert.Empty(t, v.GetWarnings())
	})

	t.Run("capability tools never warn", func(t *testing.T) {
		cfg := &Config{MCPServers: staticServer, LoadedPromptConfigs: promptCfgs("memory__recall", "workflow__transition", "image__generate")}
		v := NewConfigValidator(cfg)
		v.validateAllowedToolsResolve()
		assert.Empty(t, v.GetWarnings())
	})

	t.Run("local tool files suppress the bare-name warning", func(t *testing.T) {
		cfg := &Config{
			MCPServers:          staticServer,
			LoadedTools:         []config.ToolData{{FilePath: "tools/list.tool.yaml"}},
			LoadedPromptConfigs: promptCfgs("list_devices"),
		}
		v := NewConfigValidator(cfg)
		v.validateAllowedToolsResolve()
		assert.Empty(t, v.GetWarnings())
	})
}
