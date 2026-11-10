package arenaconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// state_store.type: file passed load and then failed at engine build.
func TestCheckStateStore_RejectsFileType(t *testing.T) {
	err := (&Config{StateStore: &config.StateStoreConfig{Type: "file"}}).checkStateStore()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state_store.type: file")

	assert.NoError(t, (&Config{}).checkStateStore())
	assert.NoError(t, (&Config{StateStore: &config.StateStoreConfig{Type: "redis"}}).checkStateStore())
}

func TestIgnoredFieldWarnings(t *testing.T) {
	cfg := &Config{
		Defaults:   Defaults{FailOn: []string{"provider_error"}},
		StateStore: &config.StateStoreConfig{Type: "memory", File: &config.FileStateStoreConfig{Root: "x"}},
		LoadedEvals: map[string]*Eval{
			"b": {Mode: "instant"},
			"a": {Speed: 2},
			"c": nil,
			"d": {},
		},
	}
	got := cfg.ignoredFieldWarnings()
	require.Len(t, got, 4)
	assert.Contains(t, got[0], "defaults.fail_on is ignored")
	assert.Contains(t, got[1], "state_store.file is ignored")
	assert.Equal(t, "eval a: speed is ignored and will be removed", got[2])
	assert.Equal(t, "eval b: mode is ignored and will be removed", got[3])

	assert.Empty(t, (&Config{}).ignoredFieldWarnings())

	cfg.ValidationWarnings = []string{"existing"}
	cfg.recordIgnoredFields()
	assert.Len(t, cfg.ValidationWarnings, 5, "added to what validate reports")
}

func TestWarnProviderLabels_WarnsOnlyWhenLabelsAreSet(t *testing.T) {
	var out bytes.Buffer
	logger.SetOutput(&out)
	t.Cleanup(func() { logger.SetOutput(nil) })

	dir := t.TempDir()
	withLabels := filepath.Join(dir, "labelled.yaml")
	require.NoError(t, os.WriteFile(withLabels, []byte("metadata:\n  labels:\n    tier: fast\n"), 0o600))
	withoutLabels := filepath.Join(dir, "plain.yaml")
	require.NoError(t, os.WriteFile(withoutLabels, []byte("metadata:\n  name: p\n"), 0o600))

	warnProviderLabels(withoutLabels)
	warnProviderLabels(filepath.Join(dir, "missing.yaml"))
	assert.Empty(t, out.String(), "no labels, or no file, means no warning")

	warnProviderLabels(withLabels)
	assert.Contains(t, out.String(), "provider metadata.labels is ignored and will be removed")
	assert.Contains(t, out.String(), "labelled.yaml")
}

func TestLoadConfig_RejectsFileStateStore(t *testing.T) {
	path := writeLoaderSchemaFile(t, `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: a
spec:
  state_store:
    type: file
  providers: []
  defaults: {}
`)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state_store.type: file")
}

const runtimeIgnoredSuffix = " is ignored: Arena reads only runtime.evals, runtime.hooks and runtime.sandboxes"

// Arena reads only runtime.evals, runtime.hooks and runtime.sandboxes; every
// other set runtime field is warned about by name, and the read ones are not.
func TestIgnoredRuntimeFieldWarnings(t *testing.T) {
	assert.Empty(t, (&Config{}).ignoredRuntimeFieldWarnings())
	assert.Empty(t, (&Config{Runtime: &config.RuntimeConfigSpec{
		Evals:     map[string]*config.ExecBinding{"probe": {Command: "x"}},
		Hooks:     map[string]*config.ExecHook{"h": {Hook: "session"}},
		Sandboxes: map[string]*config.SandboxConfig{"s": {Mode: "direct"}},
		// Present but empty counts as unset.
		Providers: []config.Provider{},
	}}).ignoredRuntimeFieldWarnings())

	cfg := &Config{Runtime: &config.RuntimeConfigSpec{
		Evals:        map[string]*config.ExecBinding{"probe": {Command: "x"}},
		Providers:    []config.Provider{{ID: "p"}},
		StateStore:   &config.StateStoreConfig{Type: "memory"},
		ToolSelector: "narrow",
	}}
	assert.Equal(t, []string{
		"runtime.providers" + runtimeIgnoredSuffix,
		"runtime.state_store" + runtimeIgnoredSuffix,
		"runtime.tool_selector" + runtimeIgnoredSuffix,
	}, cfg.ignoredRuntimeFieldWarnings())
}

func TestValidator_WarnsOnIgnoredRuntimeFields(t *testing.T) {
	cfg := &Config{Runtime: &config.RuntimeConfigSpec{Logging: &config.LoggingConfigSpec{}}}
	v := NewConfigValidatorWithPath(cfg, "")
	_ = v.Validate()
	assert.Contains(t, v.GetWarnings(), "runtime.logging"+runtimeIgnoredSuffix)
}

func TestLoadConfig_WarnsOnIgnoredRuntimeFields(t *testing.T) {
	path := writeLoaderSchemaFile(t, `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: a
spec:
  providers: []
  defaults: {}
  runtime:
    state_store:
      type: memory
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err, "ignored runtime fields warn; they do not fail the load")
	assert.Contains(t, cfg.ValidationWarnings, "runtime.state_store"+runtimeIgnoredSuffix)
}
