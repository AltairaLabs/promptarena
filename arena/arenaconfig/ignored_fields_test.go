package arenaconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
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

func TestWarnProviderLabels_DoesNotFail(t *testing.T) {
	dir := t.TempDir()
	withLabels := filepath.Join(dir, "p.yaml")
	require.NoError(t, os.WriteFile(withLabels, []byte("metadata:\n  labels:\n    tier: fast\n"), 0o600))
	warnProviderLabels(withLabels)
	warnProviderLabels(filepath.Join(dir, "missing.yaml"))
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
