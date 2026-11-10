package promptrepo

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/persistence"
	"github.com/AltairaLabs/PromptKit/runtime/v2/persistence/repotest"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

func promptWith(taskType string, refs ...prompt.FragmentRef) *prompt.Config {
	return &prompt.Config{
		APIVersion: "promptkit.altairalabs.ai/v1alpha1",
		Kind:       "PromptConfig",
		Spec:       prompt.Spec{TaskType: taskType, SystemTemplate: "hi", Fragments: refs},
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestAdd_RecordsPromptDir(t *testing.T) {
	r := New()
	require.NoError(t, r.Add(promptWith("support"), "/packs/support/prompts"))

	dir, err := r.PromptDir("support")
	require.NoError(t, err)
	assert.Equal(t, "/packs/support/prompts", dir)

	_, err = r.PromptDir("unknown")
	assert.Error(t, err)
}

func TestAdd_RejectsInvalidConfig(t *testing.T) {
	assert.Error(t, New().Add(nil, "/x"))
	assert.Error(t, New().Add(promptWith(""), "/x"))
}

func TestLoadFragment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fragments", "tone.yaml"), "content: Be kind.\n")
	r := New()

	frag, err := r.LoadFragment("tone", "fragments/tone.yaml", dir)
	require.NoError(t, err)
	assert.Equal(t, "Be kind.", frag.Content)
	assert.Equal(t, filepath.Join(dir, "fragments", "tone.yaml"), frag.SourceFile)

	_, err = r.LoadFragment("tone", "", dir)
	assert.ErrorContains(t, err, `fragment "tone" has no path`)

	// No path, with a base: fragments/<name>.yaml under it.
	byName, err := NewWithBase(dir).LoadFragment("tone", "", "")
	require.NoError(t, err)
	assert.Equal(t, "Be kind.", byName.Content)

	// A path may not climb back out past where it started.
	writeFile(t, filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-secret.yaml"), "content: secret\n")
	_, err = r.LoadFragment("tone", "x/../../"+filepath.Base(dir)+"-secret.yaml", dir)
	assert.ErrorIs(t, err, persistence.ErrFragmentPathOutsideBase)

	_, err = r.LoadFragment("tone", "missing.yaml", dir)
	assert.True(t, errors.Is(err, fs.ErrNotExist), "a missing file keeps fs.ErrNotExist: %v", err)
}

func TestLoadFragments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tone.yaml"), "content: Be kind.\n")

	r := New()
	require.NoError(t, r.Add(promptWith("a",
		prompt.FragmentRef{Name: "tone", Path: "tone.yaml", Required: true},
		prompt.FragmentRef{Name: "extra", Path: "extra.yaml", Required: false},
		prompt.FragmentRef{Name: "persona", Path: "{{region}}.yaml", Required: true},
	), dir))

	warnings, err := r.LoadFragments(prompt.NewRegistryWithRepository(r))
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], `optional fragment "extra" was not loaded and renders empty`)

	require.NoError(t, r.Add(promptWith("b",
		prompt.FragmentRef{Name: "gone", Path: "gone.yaml", Required: true},
	), dir))
	_, err = r.LoadFragments(prompt.NewRegistryWithRepository(r))
	assert.ErrorContains(t, err, `prompt "b": required fragment "gone"`)
}

// TestRepository_FragmentContract runs PromptKit's fragment repository
// contract, the behaviour FragmentResolver relies on.
func TestRepository_FragmentContract(t *testing.T) {
	assertFragmentContract(t, func(_ *testing.T, root string) prompt.FragmentRepository {
		return NewWithBase(root)
	})
}

// assertFragmentContract fails t for every contract check the repository
// newRepo builds does not pass; each check is a subtest.
func assertFragmentContract(t *testing.T, newRepo func(t *testing.T, root string) prompt.FragmentRepository) {
	t.Helper()
	repotest.RunFragmentRepositoryContract(t, newRepo)
}
