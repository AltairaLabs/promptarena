package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/packc/compiler"
)

const testPromptYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: greeting
spec:
  task_type: "greeting"
  version: "v1.0.0"
  description: "A simple greeting prompt"
  system_template: "You are a friendly assistant."
`

const testArenaYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
    - id: prompt0
      file: prompts/greeting.yaml
  providers: []
  defaults:
    temperature: 0.7
    max_tokens: 100
`

// runWithArgs sets os.Args, captures stdout+stderr while fn runs, and restores
// all globals afterwards. Output volume must stay under the OS pipe buffer
// (~64KB); all packc command output easily fits.
func runWithArgs(t *testing.T, args []string, fn func()) (stdout, stderr string) {
	t.Helper()

	origArgs, origOut, origErr := os.Args, os.Stdout, os.Stderr
	defer func() {
		os.Args, os.Stdout, os.Stderr = origArgs, origOut, origErr
	}()

	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)
	rErr, wErr, err := os.Pipe()
	require.NoError(t, err)

	os.Args = args
	os.Stdout = wOut
	os.Stderr = wErr

	fn()

	require.NoError(t, wOut.Close())
	require.NoError(t, wErr.Close())

	var bufOut, bufErr bytes.Buffer
	_, _ = bufOut.ReadFrom(rOut)
	_, _ = bufErr.ReadFrom(rErr)
	return bufOut.String(), bufErr.String()
}

// setupProject writes a minimal, compilable arena project into a temp dir and
// returns the dir plus the config file path.
func setupProject(t *testing.T) (dir, configFile string) {
	t.Helper()
	dir = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "prompts"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "prompts", "greeting.yaml"), []byte(testPromptYAML), 0o644))
	configFile = filepath.Join(dir, "config.arena.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(testArenaYAML), 0o644))
	return dir, configFile
}

// buildPackFile compiles the project in dir and writes a valid pack JSON,
// returning its path.
func buildPackFile(t *testing.T, dir, configFile string) string {
	t.Helper()
	res, err := compiler.Compile(configFile,
		compiler.WithPackID("test-pack"),
		compiler.WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	packPath := filepath.Join(dir, "test.pack.json")
	require.NoError(t, os.WriteFile(packPath, res.JSON, 0o644))
	return packPath
}

func TestPrintUsage(t *testing.T) {
	out, _ := runWithArgs(t, []string{"packc", "help"}, printUsage)
	assert.Contains(t, out, "packc - PromptKit Pack Compiler")
	assert.Contains(t, out, "packc compile")
	assert.Contains(t, out, "packc validate")
	assert.Contains(t, out, "packc completion")
}

func TestGetDefaultPackID(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "My Cool Project")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)

	assert.Equal(t, "my-cool-project", getDefaultPackID())
}

func TestParseCompileFlags(t *testing.T) {
	t.Run("all flags explicit", func(t *testing.T) {
		var flags compileFlags
		runWithArgs(t, []string{
			"packc", "compile", "-c", "custom.yaml", "-o", "out.json", "--id", "my-id",
		}, func() { flags = parseCompileFlags() })

		assert.Equal(t, "custom.yaml", flags.configFile)
		assert.Equal(t, "out.json", flags.outputFile)
		assert.Equal(t, "my-id", flags.packID)
	})

	t.Run("id set derives output default", func(t *testing.T) {
		var flags compileFlags
		runWithArgs(t, []string{"packc", "compile", "--id", "widget"}, func() {
			flags = parseCompileFlags()
		})

		assert.Equal(t, "config.arena.yaml", flags.configFile)
		assert.Equal(t, "widget", flags.packID)
		assert.Equal(t, "widget.pack.json", flags.outputFile)
	})

	t.Run("no flags derives id from cwd", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "auto-project")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		t.Chdir(sub)

		var flags compileFlags
		runWithArgs(t, []string{"packc", "compile"}, func() {
			flags = parseCompileFlags()
		})

		assert.Equal(t, "auto-project", flags.packID)
		assert.Equal(t, "auto-project.pack.json", flags.outputFile)
	})
}

func TestCompletionCommand(t *testing.T) {
	tests := []struct {
		shell  string
		expect string
	}{
		{"bash", "_packc_completions"},
		{"zsh", "#compdef packc"},
		{"fish", "complete -c packc"},
		{"powershell", "Register-ArgumentCompleter"},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			out, _ := runWithArgs(t, []string{"packc", "completion", tt.shell}, completionCommand)
			assert.Contains(t, out, tt.expect)
		})
	}
}

func TestCompileCommand(t *testing.T) {
	dir, configFile := setupProject(t)
	outPath := filepath.Join(dir, "packs", "out.pack.json")

	out, errOut := runWithArgs(t, []string{
		"packc", "compile", "-c", configFile, "-o", outPath, "--id", "compile-cmd",
	}, compileCommand)

	assert.Empty(t, errOut)
	assert.Contains(t, out, "Pack compiled successfully")
	assert.Contains(t, out, "greeting")

	data, err := os.ReadFile(outPath) //nolint:gosec // test path under t.TempDir
	require.NoError(t, err)
	assert.Contains(t, string(data), "compile-cmd")
}

func TestCompilePromptCommand(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "greeting.yaml")
	require.NoError(t, os.WriteFile(promptFile, []byte(testPromptYAML), 0o644))
	outPath := filepath.Join(dir, "greeting.pack.json")

	out, errOut := runWithArgs(t, []string{
		"packc", "compile-prompt", "-p", promptFile, "-o", outPath,
	}, compilePromptCommand)

	assert.Empty(t, errOut)
	assert.Contains(t, out, "Pack compiled successfully")

	_, err := os.Stat(outPath)
	require.NoError(t, err)
}

func TestValidateCommand(t *testing.T) {
	dir, configFile := setupProject(t)
	packPath := buildPackFile(t, dir, configFile)

	out, errOut := runWithArgs(t, []string{"packc", "validate", packPath}, validateCommand)

	assert.Empty(t, errOut)
	assert.Contains(t, out, "Schema validation passed")
	assert.Contains(t, out, "Pack structure is valid")
}

func TestInspectCommand(t *testing.T) {
	dir, configFile := setupProject(t)
	packPath := buildPackFile(t, dir, configFile)

	out, errOut := runWithArgs(t, []string{"packc", "inspect", packPath}, inspectCommand)

	assert.Empty(t, errOut)
	assert.Contains(t, out, "Pack Information")
	assert.Contains(t, out, "Prompts")
	assert.Contains(t, out, "greeting")
}

func TestMainDispatch(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		out, _ := runWithArgs(t, []string{"packc", "version"}, main)
		assert.Contains(t, out, "packc")
	})

	t.Run("help", func(t *testing.T) {
		out, _ := runWithArgs(t, []string{"packc", "help"}, main)
		assert.Contains(t, out, "PromptKit Pack Compiler")
	})

	t.Run("--help flag", func(t *testing.T) {
		out, _ := runWithArgs(t, []string{"packc", "--help"}, main)
		assert.Contains(t, out, "PromptKit Pack Compiler")
	})

	t.Run("completion routes", func(t *testing.T) {
		out, _ := runWithArgs(t, []string{"packc", "completion", "bash"}, main)
		assert.Contains(t, out, "_packc_completions")
	})

	t.Run("inspect routes", func(t *testing.T) {
		dir, configFile := setupProject(t)
		packPath := buildPackFile(t, dir, configFile)
		out, _ := runWithArgs(t, []string{"packc", "inspect", packPath}, main)
		assert.Contains(t, out, "Pack Information")
	})

	t.Run("validate routes", func(t *testing.T) {
		dir, configFile := setupProject(t)
		packPath := buildPackFile(t, dir, configFile)
		out, _ := runWithArgs(t, []string{"packc", "validate", packPath}, main)
		assert.Contains(t, out, "Pack structure is valid")
	})

	t.Run("compile routes", func(t *testing.T) {
		dir, configFile := setupProject(t)
		outPath := filepath.Join(dir, "routed.pack.json")
		out, _ := runWithArgs(t, []string{
			"packc", "compile", "-c", configFile, "-o", outPath, "--id", "routed",
		}, main)
		assert.Contains(t, out, "Pack compiled successfully")
	})

	t.Run("compile-prompt routes", func(t *testing.T) {
		dir := t.TempDir()
		promptFile := filepath.Join(dir, "greeting.yaml")
		require.NoError(t, os.WriteFile(promptFile, []byte(testPromptYAML), 0o644))
		outPath := filepath.Join(dir, "cp.pack.json")
		out, _ := runWithArgs(t, []string{
			"packc", "compile-prompt", "-p", promptFile, "-o", outPath,
		}, main)
		assert.Contains(t, out, "Pack compiled successfully")
	})
}

// TestCompileCommandWarnings exercises the warning-emission branch by pointing a
// media reference at a missing file so a media warning is printed to stdout.
func TestCompileCommandWarnings(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "prompts"), 0o755))

	promptWithMedia := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: greeting
spec:
  task_type: "greeting"
  version: "v1.0.0"
  description: "A greeting prompt"
  system_template: "You are a friendly assistant."
  media:
    enabled: true
    supported_types: ["image"]
    examples:
      - name: missing-media
        role: user
        parts:
          - type: media
            media:
              file_path: does-not-exist.jpg
              mime_type: image/jpeg
`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "prompts", "greeting.yaml"), []byte(promptWithMedia), 0o644))
	configFile := filepath.Join(dir, "config.arena.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(testArenaYAML), 0o644))

	outPath := filepath.Join(dir, "warn.pack.json")
	out, _ := runWithArgs(t, []string{
		"packc", "compile", "-c", configFile, "-o", outPath, "--id", "warn-pack",
	}, compileCommand)

	assert.True(t, strings.Contains(out, "Media file not found") ||
		strings.Contains(out, "does-not-exist.jpg"),
		"expected a media warning in output, got: %q", out)
}
