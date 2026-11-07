package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchTemplate_CopiesRelativeSources(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "files"), 0o750))
	tpl := "files:\n  - path: a.md\n    source: files/a.tmpl\n  - path: b.md\n    content: inline\n" +
		"  - path: c.md\n    source: https://example.com/c.tmpl\n"
	require.NoError(t, os.WriteFile(filepath.Join(src, "template.yaml"), []byte(tpl), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "files", "a.tmpl"), []byte("A"), 0o600))

	cache := t.TempDir()
	entry := &IndexEntry{Name: "demo", Version: "1.0.0", Source: "template.yaml"}
	dest, err := FetchTemplate(entry, cache, filepath.Join(src, "index.yaml"), "key")
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(dest), "files", "a.tmpl"))
	require.NoError(t, err)
	assert.Equal(t, "A", string(data))
}

func TestFetchTemplate_RejectsEscapingSource(t *testing.T) {
	src := t.TempDir()
	tpl := "spec:\n  files:\n    - path: a.md\n      source: ../secret.txt\n"
	require.NoError(t, os.WriteFile(filepath.Join(src, "template.yaml"), []byte(tpl), 0o600))
	entry := &IndexEntry{Name: "demo", Version: "1.0.0", Source: "template.yaml"}
	_, err := FetchTemplate(entry, t.TempDir(), filepath.Join(src, "index.yaml"), "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes the template directory")
}

func TestFetchTemplate_MissingSourceFails(t *testing.T) {
	src := t.TempDir()
	tpl := "spec:\n  files:\n    - path: a.md\n      source: files/missing.tmpl\n"
	require.NoError(t, os.WriteFile(filepath.Join(src, "template.yaml"), []byte(tpl), 0o600))
	entry := &IndexEntry{Name: "demo", Version: "1.0.0", Source: "template.yaml"}
	_, err := FetchTemplate(entry, t.TempDir(), filepath.Join(src, "index.yaml"), "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing.tmpl")
}

func TestRelativeSources_InvalidYAML(t *testing.T) {
	assert.Nil(t, relativeSources([]byte("files: [")))
}

func TestSiblingLocation(t *testing.T) {
	assert.Equal(t, "https://example.com/t/files/a.tmpl",
		siblingLocation("https://example.com/t/template.yaml", "files/a.tmpl"))
	assert.Equal(t, filepath.Join("dir", "files", "a.tmpl"),
		siblingLocation(filepath.Join("dir", "template.yaml"), "files/a.tmpl"))
}

func TestCacheRepoKey(t *testing.T) {
	cfg := &RepoConfig{}
	cfg.Add("local", "/somewhere/index.yaml")
	assert.Equal(t, "local", CacheRepoKey("local", cfg))

	key := CacheRepoKey("../templates/index.yaml", cfg)
	assert.True(t, strings.HasPrefix(key, "index-"))
	assert.NotContains(t, key, string(filepath.Separator))

	abs, err := filepath.Abs("../templates/index.yaml")
	require.NoError(t, err)
	assert.Equal(t, key, CacheRepoKey(abs, cfg), "a relative and absolute path to one index share a key")

	url := CacheRepoKey("https://example.com/index.yaml", nil)
	assert.True(t, strings.HasPrefix(url, "index-"))
	assert.NotEqual(t, url, key)
}

func TestGetCachePath_FallsBackToRepoName(t *testing.T) {
	r := NewRepoResolver(nil)
	got := r.GetCachePath("/cache", &TemplateRef{RepoName: "local", TemplateName: "demo", Version: "1"})
	assert.Equal(t, filepath.Join("/cache", "local", "demo", "1", "template.yaml"), got)
}

func TestGenerator_SkipsExistingFileUnlessForced(t *testing.T) {
	out := t.TempDir()
	project := filepath.Join(out, "proj")
	require.NoError(t, os.MkdirAll(project, 0o750))
	existing := filepath.Join(project, "a.txt")
	require.NoError(t, os.WriteFile(existing, []byte("mine"), 0o600))

	tmpl := &Template{Spec: TemplateSpec{Files: []FileSpec{{Path: "a.txt", Content: "generated"}}}}
	cfg := &TemplateConfig{ProjectName: "proj", OutputDir: out, Variables: map[string]interface{}{}, Verbose: true}
	res, err := NewGenerator(tmpl, nil).Generate(cfg)
	require.NoError(t, err)
	data, _ := os.ReadFile(existing)
	assert.Equal(t, "mine", string(data))
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "kept existing a.txt")
	assert.Empty(t, res.FilesCreated)

	cfg.Force = true
	res, err = NewGenerator(tmpl, nil).Generate(cfg)
	require.NoError(t, err)
	data, _ = os.ReadFile(existing)
	assert.Equal(t, "generated", string(data))
	assert.Equal(t, []string{"a.txt"}, res.FilesCreated)
}
