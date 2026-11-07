package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSourceTemplateRepo lays out a local template repo whose template pulls
// its only file in through files[].source, and returns the repo root.
func writeSourceTemplateRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	tplDir := filepath.Join(root, "templates", "demo")
	require.NoError(t, os.MkdirAll(filepath.Join(tplDir, "files"), 0o750))
	index := "apiVersion: promptkit.altairalabs.ai/v1\nkind: TemplateIndex\nspec:\n  entries:\n" +
		"    - name: demo\n      version: \"1.0.0\"\n      description: from source\n" +
		"      source: templates/demo/template.yaml\n"
	tpl := "apiVersion: promptkit.altairalabs.ai/v1\nkind: Template\nmetadata:\n  name: demo\n" +
		"spec:\n  files:\n    - path: README.md\n      source: files/readme.md.tmpl\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.yaml"), []byte(index), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tplDir, "template.yaml"), []byte(tpl), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tplDir, "files", "readme.md.tmpl"), []byte("Hello {{.name}}"), 0o600))
	return root
}

func resetTemplateFlags(t *testing.T, dir string) {
	t.Helper()
	repoConfigPath = filepath.Join(dir, "repos.yaml")
	templateCache = filepath.Join(dir, "cache")
	templateFile = ""
	valuesFile = ""
	promptMissing = false
	templateValues = map[string]string{}
}

// A relative path-form --index used to become the cache repo name, so fetch
// tried to mkdir under "../.../index.yaml" and failed. The template also uses
// files[].source, which the cache never held, so render could not find it.
func TestTemplatesFetchRender_RelativePathIndexWithSourceFiles(t *testing.T) {
	root := writeSourceTemplateRepo(t)
	work := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(work, 0o750))
	t.Chdir(work)
	resetTemplateFlags(t, t.TempDir())

	relIndex, err := filepath.Rel(work, filepath.Join(root, "index.yaml"))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(relIndex, ".."))

	templateIndex = relIndex
	templateName = "demo"
	templateVersion = "1.0.0"
	buf := &bytes.Buffer{}
	templatesFetchCmd.SetOut(buf)
	require.NoError(t, templatesFetchCmd.RunE(templatesFetchCmd, nil))
	cached := strings.TrimSpace(strings.TrimPrefix(buf.String(), "Cached demo@1.0.0 at "))
	assert.True(t, strings.HasPrefix(cached, templateCache+string(filepath.Separator)),
		"cached template %q must stay inside the cache dir %q", cached, templateCache)

	out := filepath.Join(t.TempDir(), "out")
	outputDir = out
	templateValues = map[string]string{"name": "world"}
	templatesRenderCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, templatesRenderCmd.RunE(templatesRenderCmd, nil))
	data, err := os.ReadFile(filepath.Join(out, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "Hello world", string(data))
}

// The same absolute --index must land on the same cache entry for fetch and
// render; it used to nest the whole path under the cache on fetch only.
func TestTemplatesFetchRender_AbsolutePathIndex(t *testing.T) {
	root := writeSourceTemplateRepo(t)
	resetTemplateFlags(t, t.TempDir())
	templateIndex = filepath.Join(root, "index.yaml")
	templateName = "demo"
	templateVersion = "1.0.0"
	templatesFetchCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, templatesFetchCmd.RunE(templatesFetchCmd, nil))

	outputDir = filepath.Join(t.TempDir(), "out")
	templateValues = map[string]string{"name": "abs"}
	templatesRenderCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, templatesRenderCmd.RunE(templatesRenderCmd, nil))
	data, err := os.ReadFile(filepath.Join(outputDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "Hello abs", string(data))
}

// A cached template whose source file is missing is not a cache miss; the
// error must name the missing source, not tell the user to fetch again.
func TestTemplatesRender_MissingSourceIsNotACacheMiss(t *testing.T) {
	root := writeSourceTemplateRepo(t)
	resetTemplateFlags(t, t.TempDir())
	templateIndex = filepath.Join(root, "index.yaml")
	templateName = "demo"
	templateVersion = "1.0.0"
	templatesFetchCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, templatesFetchCmd.RunE(templatesFetchCmd, nil))

	cachedSources, err := filepath.Glob(filepath.Join(templateCache, "*", "demo", "1.0.0", "files", "readme.md.tmpl"))
	require.NoError(t, err)
	require.Len(t, cachedSources, 1)
	require.NoError(t, os.Remove(cachedSources[0]))

	outputDir = filepath.Join(t.TempDir(), "out")
	err = templatesRenderCmd.RunE(templatesRenderCmd, nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "not found in cache")
	assert.Contains(t, err.Error(), "readme.md.tmpl")
}

// `templates list` must show the embedded built-ins, including init's default.
func TestTemplatesList_ShowsBuiltIns(t *testing.T) {
	root := writeSourceTemplateRepo(t)
	resetTemplateFlags(t, t.TempDir())
	templateIndex = filepath.Join(root, "index.yaml")
	buf := &bytes.Buffer{}
	templatesListCmd.SetOut(buf)
	require.NoError(t, templatesListCmd.RunE(templatesListCmd, nil))
	out := buf.String()
	assert.Contains(t, out, builtInRepoLabel)
	assert.Contains(t, out, "quick-start")
	// A raw index path is not a repo name, so its entries are listed bare.
	assert.NotContains(t, out, "index.yaml/demo")
	assert.Contains(t, out, "demo")
}

// When the index cannot be loaded the built-ins are still printed.
func TestTemplatesList_BuiltInsSurviveIndexFailure(t *testing.T) {
	resetTemplateFlags(t, t.TempDir())
	templateIndex = filepath.Join(t.TempDir(), "missing.yaml")
	buf := &bytes.Buffer{}
	templatesListCmd.SetOut(buf)
	require.Error(t, templatesListCmd.RunE(templatesListCmd, nil))
	assert.Contains(t, buf.String(), "quick-start")
}
