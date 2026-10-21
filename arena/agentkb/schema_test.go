package agentkb

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaNames_IncludeCoreTypes(t *testing.T) {
	names, err := SchemaNames()
	require.NoError(t, err)
	for _, want := range []string{"scenario", "provider", "promptconfig", "tool"} {
		assert.Contains(t, names, want)
	}
	assert.NotContains(t, names, "common", "common is a $ref dir, not a type")
}

func TestSchema_ReturnsBytesAndRejectsUnknown(t *testing.T) {
	b, err := Schema("scenario")
	require.NoError(t, err)
	assert.NotEmpty(t, b)
	assert.Contains(t, string(b), "$schema")

	_, err = Schema("nope")
	require.Error(t, err)
}

// Parity: the embedded schemas must byte-match the generated source of truth, so
// the binary can never ship a schema that disagrees with its own validate.
func TestSchemas_ByteMatchGeneratedSource(t *testing.T) {
	srcRoot := filepath.Join("..", "..", "schemas", "v1alpha1")
	err := fs.WalkDir(schemasFS, "schemas", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		embedded, err := schemasFS.ReadFile(path)
		require.NoError(t, err)
		rel := strings.TrimPrefix(path, "schemas/")
		src, err := os.ReadFile(filepath.Join(srcRoot, filepath.FromSlash(rel)))
		require.NoError(t, err, "missing source schema for %s", rel)
		assert.Equal(t, string(src), string(embedded),
			"embedded %s drifted from schemas/v1alpha1; run `make schemas`", rel)
		return nil
	})
	require.NoError(t, err)

	// And the other way: every source schema is embedded.
	err = filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}
		rel, err := filepath.Rel(srcRoot, path)
		require.NoError(t, err)
		_, statErr := fs.Stat(schemasFS, "schemas/"+filepath.ToSlash(rel))
		assert.NoError(t, statErr, "schemas/v1alpha1/%s is not embedded; run `make schemas`", rel)
		return nil
	})
	require.NoError(t, err)
}
