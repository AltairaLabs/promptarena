package agentkb

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

const embeddedTestArenaYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: kit
spec:
  providers: []
  defaults:
    temperature: 0.7
    max_tokens: 100
`

// Config validation must not depend on the network or the working directory:
// with the hosted schemas unreachable, no schemas/ directory nearby and the
// local fallback off, it still validates — against the embedded copy.
func TestUseEmbeddedSchemas_ValidationIsHermetic(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1") // any fetch of the hosted copy fails
	prevFallback := config.SchemaFallbackDisabled.Swap(true)
	t.Cleanup(func() {
		config.SchemaFallbackDisabled.Store(prevFallback)
		config.UseSchemaFS(nil)
	})

	UseEmbeddedSchemas()

	result, err := config.ValidateWithSchema([]byte(embeddedTestArenaYAML), config.ConfigTypeArena)
	require.NoError(t, err)
	assert.True(t, result.Valid, "errors: %+v", result.Errors)

	typo := embeddedTestArenaYAML + "  defaultz: {}\n"
	result, err = config.ValidateWithSchema([]byte(typo), config.ConfigTypeArena)
	require.NoError(t, err)
	assert.False(t, result.Valid, "the embedded schema still rejects unknown keys")
}

// The filesystem handed to pkg/config is the same set `promptarena schema`
// prints, laid out with <type>.json at its root.
func TestSchemaFS_MatchesSchema(t *testing.T) {
	names, err := SchemaNames()
	require.NoError(t, err)
	require.NotEmpty(t, names)
	for _, name := range names {
		want, err := Schema(name)
		require.NoError(t, err)
		got, err := fs.ReadFile(SchemaFS(), name+".json")
		require.NoError(t, err)
		assert.Equal(t, want, got, name)
	}
}
