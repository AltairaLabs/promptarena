package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// installUserSkill installs @org/name at the user level under a fresh
// XDG_CONFIG_HOME, the way `promptarena skill install` lays it out.
func installUserSkill(t *testing.T, org, name string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	dir := filepath.Join(home, "promptkit", "skills", org, name)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte("---\nname: "+name+"\ndescription: installed\n---\nDo it.\n"), 0o600))
	return dir
}

// An installed @org/name skill used to fail compile with `skills: directory
// "@org/name" does not exist`. It now compiles, and the pack keeps the
// reference rather than this machine's install path.
func TestCompile_KeepsInstalledSkillRef(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	installUserSkill(t, "acme", "pdf")
	t.Chdir(t.TempDir())
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/chat.yaml", minimalPromptYAML)
	configFile := writeFixture(t, dir, "config.arena.yaml", `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
    - id: greeting
      file: prompts/chat.yaml
  providers: []
  skills:
    - path: "@acme/pdf"
  defaults:
    concurrency: 1
`)

	result, err := Compile(configFile, WithPackID("ref-pack"), WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation())
	require.NoError(t, err)
	require.Len(t, result.Pack.Skills, 1)
	assert.Equal(t, "@acme/pdf", prompt.SkillPath(result.Pack.Skills[0]))
	assert.Contains(t, string(result.JSON), `"@acme/pdf"`)
}

func TestValidateSkillErrors_SkillRef(t *testing.T) {
	installUserSkill(t, "acme", "pdf")
	t.Chdir(t.TempDir())
	packDir := t.TempDir()

	ok := &prompt.Pack{Pack: packspec.Pack{Skills: []*prompt.SkillSourceConfig{{Path: "@acme/pdf"}}}}
	assert.Empty(t, ValidateSkillErrors(ok, packDir))
	assert.Empty(t, ValidateSkills(ok, packDir))

	missing := &prompt.Pack{Pack: packspec.Pack{Skills: []*prompt.SkillSourceConfig{{Path: "@acme/missing"}}}}
	errs := ValidateSkillErrors(missing, packDir)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "not installed")
}

func TestResolveSkillDir(t *testing.T) {
	packDir := t.TempDir()
	got, err := ResolveSkillDir("skills", packDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(packDir, "skills"), got)

	abs := filepath.Join(packDir, "abs")
	got, err = ResolveSkillDir(abs, packDir)
	require.NoError(t, err)
	assert.Equal(t, abs, got)

	_, err = ResolveSkillDir("../outside", packDir)
	require.ErrorIs(t, err, errPathTraversal)
}
