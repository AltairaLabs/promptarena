package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReferenceFilesUpToDate guards the committed agentkb/reference/*.md against
// drift from their generators. Regenerate after changing handlers/schemas/CLI:
//
//	go run ./arena/cmd/promptarena gen-reference
//
// Run it from the repo root: --out defaults to arena/agentkb/reference.
func TestReferenceFilesUpToDate(t *testing.T) {
	dir := filepath.Join("..", "..", "agentkb", "reference")
	docs, err := referenceDocs()
	require.NoError(t, err)

	for name, want := range docs {
		got, readErr := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, readErr,
			"missing %s — run: go run ./arena/cmd/promptarena gen-reference --out arena/agentkb/reference", name)
		require.Equal(t, string(want), string(got),
			"%s is stale — run: go run ./arena/cmd/promptarena gen-reference --out arena/agentkb/reference", name)
	}
}

// TestGenReferenceDefaultOutIsCommittedDir pins the --out default to the
// directory the committed reference docs live in (relative to the repo root),
// so a bare `promptarena gen-reference` refreshes them instead of writing a
// stray tree.
func TestGenReferenceDefaultOutIsCommittedDir(t *testing.T) {
	def := genReferenceCmd.Flags().Lookup("out").DefValue
	require.Equal(t, filepath.Join("arena", "agentkb", "reference"), def)
	info, err := os.Stat(filepath.Join("..", "..", "..", def))
	require.NoError(t, err)
	require.True(t, info.IsDir())
}
