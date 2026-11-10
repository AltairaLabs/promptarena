package arenaconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

func mountConfig(t *testing.T, mounts []any, scenarios map[string]*Scenario) *Config {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "fixtures", "add"), 0o755))
	return &Config{
		ConfigDir: dir,
		MCPServers: []config.MCPServerConfig{{
			Name: "sandbox", Source: "docker", Scope: "session",
			SourceArgs: map[string]any{"image": "img", "mounts": mounts},
		}},
		LoadedScenarios: scenarios,
	}
}

func mountErrors(cfg *Config) []string {
	v := NewConfigValidatorWithPath(cfg, filepath.Join(cfg.ConfigDir, "config.arena.yaml"))
	v.validateMCPMounts()
	return v.GetErrors()
}

// A broken mount used to pass validate and either fail the run or, for a
// relative path, mount an empty Docker volume.
func TestValidateMCPMounts(t *testing.T) {
	ok := mountConfig(t, []any{map[string]any{"source": "fixtures", "target": "/fixtures"}}, nil)
	assert.Empty(t, mountErrors(ok), "a mount inside the kit is valid")

	bad := mountConfig(t, []any{map[string]any{"source": "missing", "target": "/x"}}, nil)
	errs := mountErrors(bad)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], `mcp server "sandbox"`)
	assert.Contains(t, errs[0], `source "missing" does not exist`)

	// A templated source is checked once per scenario, with that scenario's
	// variables, and the error names the scenario it fails for.
	templated := mountConfig(t,
		[]any{map[string]any{"source": "fixtures/{{scenario.case}}", "target": "/workspace", "mode": "copy"}},
		map[string]*Scenario{
			"good": {ID: "good", Variables: map[string]string{"case": "add"}},
			"bad":  {ID: "bad", Variables: map[string]string{"case": "sub"}},
		})
	errs = mountErrors(templated)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], `scenario bad: mcp server "sandbox"`)
	assert.Contains(t, errs[0], `fixtures/sub" does not exist`)
}
