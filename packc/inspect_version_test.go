package main

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

func buildInfo(v string, ok bool) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		if !ok {
			return nil, false
		}
		return &debug.BuildInfo{Main: debug.Module{Version: v}}, true
	}
}

// A `go install …@vX.Y.Z` build is not ldflags-stamped, so it reports the
// module version from its build info instead of "dev" (#425 item 6).
func TestResolveVersion(t *testing.T) {
	assert.Equal(t, "v1.2.3", resolveVersion("v1.2.3", buildInfo("v9.9.9", true)), "ldflags wins")
	assert.Equal(t, "v2.4.0", resolveVersion(devVersion, buildInfo("v2.4.0", true)))
	assert.Equal(t, devVersion, resolveVersion(devVersion, buildInfo("(devel)", true)))
	assert.Equal(t, devVersion, resolveVersion(devVersion, buildInfo("", true)))
	assert.Equal(t, devVersion, resolveVersion(devVersion, buildInfo("", false)))
	assert.NotEmpty(t, packcVersion())
}

// inspect shows pack-level tools, evals, skills and compositions (#425 item 10).
func TestInspectPrintsPackLevelBlocks(t *testing.T) {
	pack := &prompt.Pack{Pack: packspec.Pack{
		Tools: map[string]*prompt.PackTool{
			"zeta":  {Name: "zeta"},
			"alpha": {Name: "alpha", Description: "First tool"},
		},
		Evals: []*packspec.Eval{
			{ID: "tone", Type: "llm_judge", Trigger: "every_turn"},
			nil,
		},
		Skills: []*packspec.SkillSource{
			{Path: "skills/billing"},
			{Name: "inline-skill", Description: "d", Instructions: "i"},
			nil,
		},
		Compositions: map[string]*packspec.Composition{"flow": {}, "another": {}},
	}}
	out, _ := runWithArgs(t, []string{"packc"}, func() {
		printTools(pack)
		printEvals(pack)
		printSkills(pack)
		printCompositions(pack)
	})
	assert.Contains(t, out, "=== Tools (2) ===\n  alpha: First tool\n  zeta\n")
	assert.Contains(t, out, "=== Evals (2) ===\n  tone (type: llm_judge, trigger: every_turn)\n")
	assert.Contains(t, out, "path: skills/billing")
	assert.Contains(t, out, "inline: inline-skill")
	assert.Contains(t, out, "=== Compositions (2) ===\n  another\n  flow\n")

	empty, _ := runWithArgs(t, []string{"packc"}, func() {
		p := &prompt.Pack{}
		printTools(p)
		printEvals(p)
		printSkills(p)
		printCompositions(p)
	})
	assert.Empty(t, empty)
}

func TestValidateCommand_SeveralFiles(t *testing.T) {
	dir, configFile := setupProject(t)
	packPath := buildPackFile(t, dir, configFile)
	second := filepath.Join(dir, "second.pack.json")
	data, err := os.ReadFile(packPath) //nolint:gosec // test path under t.TempDir
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(second, data, 0o600))

	out, errOut := runWithArgs(t, []string{"packc", "validate", packPath, second}, validateCommand)
	assert.Empty(t, errOut)
	assert.Contains(t, out, "Validating pack: "+packPath)
	assert.Contains(t, out, "Validating pack: "+second)
	assert.Contains(t, out, "2 of 2 packs valid")
}

func TestValidateAndInspectHelp(t *testing.T) {
	out, _ := runWithArgs(t, []string{"packc", "validate", "--help"}, validateCommand)
	assert.Contains(t, out, "Usage: packc validate")
	out, _ = runWithArgs(t, []string{"packc", "inspect", "-h"}, inspectCommand)
	assert.Contains(t, out, "Usage: packc inspect")
}

func TestCompilePromptCommand_CreatesOutputDir(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "greeting.yaml")
	require.NoError(t, os.WriteFile(promptFile, []byte(testPromptYAML), 0o600))
	outPath := filepath.Join(dir, "nested", "dir", "greeting.pack.json")

	out, errOut := runWithArgs(t, []string{
		"packc", "compile-prompt", "-p", promptFile, "-o", outPath, "--pack-version", "1.2.0",
	}, compilePromptCommand)
	assert.Empty(t, errOut)
	assert.Contains(t, out, "Pack compiled successfully")
	data, err := os.ReadFile(outPath) //nolint:gosec // test path under t.TempDir
	require.NoError(t, err)
	assert.Contains(t, string(data), `"version": "1.2.0"`)
}
