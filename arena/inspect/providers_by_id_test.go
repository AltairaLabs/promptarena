package inspect_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/inspect"
)

// config-inspect matched a provider file to its loaded provider by file base
// name, so providers/openai.yaml holding metadata.name openai-gpt4o-mini was
// dropped and the providers section printed null.
func TestCollectInspectionData_ProviderFileNameDiffersFromID(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "providers"), 0o750))
	provider := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai-gpt4o-mini
spec:
  id: openai-gpt4o-mini
  type: openai
  model: gpt-4o-mini
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "providers", "openai.yaml"), []byte(provider), 0o600))
	arena := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  providers:
    - file: providers/openai.yaml
  defaults:
    concurrency: 1
`
	configFile := filepath.Join(dir, "config.arena.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(arena), 0o600))

	cfg, err := arenaconfig.LoadConfig(configFile)
	require.NoError(t, err)
	require.Contains(t, cfg.LoadedProviders, "openai-gpt4o-mini")

	data := inspect.CollectInspectionData(cfg, configFile)
	require.Len(t, data.Providers, 1)
	assert.Equal(t, "openai-gpt4o-mini", data.Providers[0].ID)
	assert.Equal(t, "providers/openai.yaml", data.Providers[0].File)
	assert.Equal(t, "gpt-4o-mini", data.Providers[0].Model)
}
