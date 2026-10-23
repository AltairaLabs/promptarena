package agentkb

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The filesystem arena/configschema validates against is the same set
// `promptarena schema` prints, laid out with <type>.json at its root.
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
