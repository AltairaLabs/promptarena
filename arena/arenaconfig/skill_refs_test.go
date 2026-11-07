package arenaconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeInstalledSkill(t *testing.T, base, org, name string) string {
	t.Helper()
	dir := filepath.Join(base, org, name)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte("---\nname: "+name+"\ndescription: installed\n---\nDo it.\n"), 0o600))
	return dir
}

func writeSkillRefConfig(t *testing.T, dir, skillsBlock string) string {
	t.Helper()
	content := "apiVersion: promptkit.altairalabs.ai/v1alpha1\nkind: Arena\nmetadata:\n  name: test\n" +
		"spec:\n  providers: []\n  skills:\n" + skillsBlock + "  defaults:\n    concurrency: 1\n"
	path := filepath.Join(dir, "config.arena.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// An @org/name skill installed with `promptarena skill install` used to fail
// with `skills: directory "@org/name" does not exist`.
func TestLoadConfig_SkillRefResolvesUserInstall(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Chdir(t.TempDir())
	installed := writeInstalledSkill(t, filepath.Join(home, "promptkit", "skills"), "acme", "pdf")

	dir := t.TempDir()
	cfg, err := LoadConfig(writeSkillRefConfig(t, dir, "    - path: \"@acme/pdf\"\n"))
	require.NoError(t, err)
	require.Len(t, cfg.LoadedSkillSources, 1)
	assert.Equal(t, installed, prompt.SkillPath(&cfg.LoadedSkillSources[0]))
}

// A project-level install beside the config wins, even when promptarena runs
// from another directory with --config.
func TestLoadConfig_SkillRefPrefersProjectInstallBesideConfig(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Chdir(t.TempDir())
	writeInstalledSkill(t, filepath.Join(home, "promptkit", "skills"), "acme", "pdf")

	dir := t.TempDir()
	projectCopy := writeInstalledSkill(t, filepath.Join(dir, ".promptkit", "skills"), "acme", "pdf")
	cfg, err := LoadConfig(writeSkillRefConfig(t, dir, "    - path: \"@acme/pdf\"\n"))
	require.NoError(t, err)
	require.Len(t, cfg.LoadedSkillSources, 1)
	resolved := prompt.SkillPath(&cfg.LoadedSkillSources[0])
	want, err := filepath.EvalSymlinks(projectCopy)
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(resolved)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestLoadConfig_SkillRefNotInstalled(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	_, err := LoadConfig(writeSkillRefConfig(t, t.TempDir(), "    - path: \"@acme/missing\"\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
	assert.Contains(t, err.Error(), "promptarena skill install @acme/missing")
}

func TestResolveSkillRef_InvalidRef(t *testing.T) {
	_, err := ResolveSkillRef("@nope", "")
	require.Error(t, err)
}

func TestIsSkillRef(t *testing.T) {
	assert.True(t, IsSkillRef("@org/name"))
	assert.False(t, IsSkillRef("skills/org"))
	assert.False(t, IsSkillRef(""))
}
