package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// writeSkill creates <root>/<name>/SKILL.md and returns the skill directory.
func writeMountSkill(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o600))
	return dir
}

// A `path:` skill has no name; it used to be skipped, so the sandbox got no
// /skills mount at all.
func TestBuildSkillMounts_PathSkillUsesDirectoryName(t *testing.T) {
	dir := writeMountSkill(t, t.TempDir(), "codegen")
	mounts := buildSkillMounts([]prompt.SkillSourceConfig{{Path: dir}})
	require.Len(t, mounts, 1)
	assert.Equal(t, dir, mounts[0]["source"])
	assert.Equal(t, "/skills/codegen", mounts[0]["target"])
}

func TestBuildSkillMounts_SkillsDirectoryMountsEachSkillOnce(t *testing.T) {
	root := t.TempDir()
	codegen := writeMountSkill(t, root, "codegen")
	review := writeMountSkill(t, root, "review")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "not-a-skill"), 0o755))

	// The codegen-sandbox shape: the directory, and one of its skills again.
	mounts := buildSkillMounts([]prompt.SkillSourceConfig{{Path: root}, {Path: codegen}})
	require.Len(t, mounts, 2)
	assert.Equal(t, codegen, mounts[0]["source"])
	assert.Equal(t, "/skills/codegen", mounts[0]["target"])
	assert.Equal(t, review, mounts[1]["source"])
	assert.Equal(t, "/skills/review", mounts[1]["target"])
}

func TestBuildSkillMounts_SkipsInlineAndMissingSkills(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	assert.Nil(t, buildSkillMounts([]prompt.SkillSourceConfig{{Name: "inline"}, {Path: missing}}))
}

// User mounts used to be replaced wholesale by the skill mounts.
func TestInjectMountsIntoArgs_KeepsUserMounts(t *testing.T) {
	user := map[string]any{"source": "/host/data", "target": "/data"}
	clash := map[string]any{"source": "/host/mine", "target": "/skills/codegen"}
	entry := &config.MCPServerConfig{SourceArgs: map[string]any{"mounts": []any{user, clash}}}
	skill := map[string]any{"source": "/host/codegen", "target": "/skills/codegen", "readonly": true}
	other := map[string]any{"source": "/host/review", "target": "/skills/review", "readonly": true}

	injectMountsIntoArgs(entry, []map[string]any{skill, other})

	assert.Equal(t, []any{user, clash, other}, entry.SourceArgs["mounts"],
		"user mounts come first and win a target clash")
}

func TestInjectMountsIntoArgs_AcceptsTypedUserMounts(t *testing.T) {
	user := map[string]any{"source": "/host/data", "target": "/data"}
	entry := &config.MCPServerConfig{SourceArgs: map[string]any{"mounts": []map[string]any{user}}}
	skill := map[string]any{"source": "/host/codegen", "target": "/skills/codegen"}

	injectMountsIntoArgs(entry, []map[string]any{skill})

	assert.Equal(t, []any{user, skill}, entry.SourceArgs["mounts"])
}
