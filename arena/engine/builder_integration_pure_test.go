package engine

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/tools"
	"github.com/AltairaLabs/PromptKit/runtime/types"
)

func TestResolveMockPartsPaths(t *testing.T) {
	t.Run("no-op when configDir empty", func(t *testing.T) {
		fp := "rel/img.png"
		tool := &tools.ToolDescriptor{MockParts: []types.ContentPart{
			{Media: &types.MediaContent{FilePath: &fp}},
		}}
		resolveMockPartsPaths(tool, "")
		assert.Equal(t, "rel/img.png", *tool.MockParts[0].Media.FilePath)
	})

	t.Run("resolves relative path against configDir", func(t *testing.T) {
		tool := &tools.ToolDescriptor{MockParts: []types.ContentPart{
			{Media: &types.MediaContent{FilePath: strPtr("assets/img.png")}},
		}}
		resolveMockPartsPaths(tool, "/base")
		assert.Equal(t, filepath.Join("/base", "assets/img.png"), *tool.MockParts[0].Media.FilePath)
	})

	t.Run("leaves absolute path untouched", func(t *testing.T) {
		abs := filepath.Join("/already", "abs.png")
		tool := &tools.ToolDescriptor{MockParts: []types.ContentPart{
			{Media: &types.MediaContent{FilePath: &abs}},
		}}
		resolveMockPartsPaths(tool, "/base")
		assert.Equal(t, abs, *tool.MockParts[0].Media.FilePath)
	})

	t.Run("ignores parts without media file paths", func(t *testing.T) {
		text := "hello"
		tool := &tools.ToolDescriptor{MockParts: []types.ContentPart{
			{Type: types.ContentTypeText, Text: &text},
			{Media: &types.MediaContent{}},
		}}
		require.NotPanics(t, func() { resolveMockPartsPaths(tool, "/base") })
	})
}

func TestBuildMergedVariables(t *testing.T) {
	de := &DuplexConversationExecutor{}

	t.Run("empty when no region or metadata", func(t *testing.T) {
		got := de.buildMergedVariables(&ConversationRequest{})
		assert.Empty(t, got)
	})

	t.Run("includes region and metadata", func(t *testing.T) {
		req := &ConversationRequest{
			Region:   "us",
			Metadata: map[string]string{"tone": "formal"},
		}
		got := de.buildMergedVariables(req)
		assert.Equal(t, "us", got["region"])
		assert.Equal(t, "formal", got["tone"])
	})
}
