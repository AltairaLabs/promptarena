// Package promptrepo provides the prompt repository that both `promptarena run`
// and `packc compile` build their prompt registry from, so the two resolve a
// prompt's fragments the same way.
//
// Prompt configs are held in memory, as the arena config loader has already
// read and parsed them. Fragments are read from disk: a relative fragment path
// resolves against the directory of the prompt config that references it.
package promptrepo

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/AltairaLabs/PromptKit/runtime/v2/persistence/common"
	"github.com/AltairaLabs/PromptKit/runtime/v2/persistence/memory"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// Compile-time interface checks.
var (
	_ prompt.Repository        = (*Repository)(nil)
	_ prompt.ConfigDirResolver = (*Repository)(nil)
)

// Repository is an in-memory prompt repository that loads fragments from files.
type Repository struct {
	*memory.PromptRepository
	dirs map[string]string // task type -> the prompt config's directory
	base string            // where a fragment referenced by name alone is read from
}

// New returns an empty Repository. A fragment reference with no path names no
// file, so it cannot be loaded.
func New() *Repository {
	return NewWithBase("")
}

// NewWithBase returns an empty Repository whose fragments referenced by name
// alone are read from fragments/<name>.yaml under base.
func NewWithBase(base string) *Repository {
	return &Repository{
		PromptRepository: memory.NewPromptRepository(),
		dirs:             make(map[string]string),
		base:             base,
	}
}

// Add registers a prompt config whose relative fragment paths resolve against
// dir, the directory the config was loaded from.
func (r *Repository) Add(config *prompt.Config, dir string) error {
	if err := r.SavePrompt(config); err != nil {
		return err
	}
	r.dirs[config.Spec.TaskType] = dir
	return nil
}

// PromptDir returns the directory of the prompt config registered for
// taskType. It implements prompt.ConfigDirResolver, which is how the registry
// learns the base a relative fragment path resolves against.
func (r *Repository) PromptDir(taskType string) (string, error) {
	dir, ok := r.dirs[taskType]
	if !ok {
		return "", fmt.Errorf("prompt not found: %s", taskType)
	}
	return dir, nil
}

// LoadFragment reads a fragment file. The read goes through PromptKit's
// common.ReadFragmentFile, so it follows the fragment repository contract
// (runtime/persistence/repotest):
//   - a relative path resolves against baseDir, and may climb only as far as
//     its own leading "../" segments; one that climbs further, directly or
//     through a symlink, is persistence.ErrFragmentPathOutsideBase;
//   - an absolute path is read as it is;
//   - no path reads fragments/<name>.yaml under the base given to NewWithBase.
//     With no base (New), a reference with no path cannot be loaded;
//   - a missing file satisfies fs.ErrNotExist, which is what the resolver's
//     fall back to the working directory keys on.
func (r *Repository) LoadFragment(name, relativePath, baseDir string) (*prompt.Fragment, error) {
	if relativePath == "" && r.base == "" {
		return nil, fmt.Errorf("fragment %q has no path, so there is no file to load it from: %w",
			name, fs.ErrNotExist)
	}
	data, path, err := common.ReadFragmentFile(r.base, name, relativePath, baseDir, ".yaml")
	if err != nil {
		return nil, fmt.Errorf("reading fragment file %s: %w", fragmentFileName(r.base, name, relativePath, baseDir), err)
	}
	var fragment prompt.Fragment
	if err := yaml.Unmarshal(data, &fragment); err != nil {
		return nil, fmt.Errorf("parsing fragment file %s: %w", path, err)
	}
	fragment.SourceFile = path
	return &fragment, nil
}

// fragmentFileName is the file a lookup names, for error messages: the read's
// own error can name it relative to the directory it was opened in.
func fragmentFileName(base, name, relativePath, baseDir string) string {
	switch {
	case filepath.IsAbs(relativePath):
		return relativePath
	case relativePath == "":
		return filepath.Join(base, "fragments", name+".yaml")
	}
	return filepath.Join(baseDir, relativePath)
}

// LoadFragments loads every static fragment the repository's prompts
// reference, through registry, which must have been built on this repository.
//
// A required fragment that cannot be loaded is an error. An optional one is
// returned as a warning: PromptKit leaves it out of the prompt without saying
// so. A reference whose name or path holds a {{variable}} is chosen at runtime
// and is not loaded here.
//
// Loading them up front also fills the registry's fragment cache, so the
// concurrent runs that later assemble these prompts only read it.
func (r *Repository) LoadFragments(registry *prompt.Registry) ([]string, error) {
	taskTypes, err := r.ListPrompts()
	if err != nil {
		return nil, err
	}
	sort.Strings(taskTypes)

	var warnings []string
	for _, taskType := range taskTypes {
		config, err := r.LoadPrompt(taskType)
		if err != nil {
			return nil, err
		}
		w, err := loadPromptFragments(registry, taskType, config.Spec.Fragments)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, w...)
	}
	return warnings, nil
}

// loadPromptFragments loads one prompt's static fragment references.
func loadPromptFragments(registry *prompt.Registry, taskType string, refs []prompt.FragmentRef) ([]string, error) {
	var warnings []string
	for _, ref := range refs {
		if isDynamic(ref) {
			continue
		}
		_, err := registry.LoadPromptFragment(taskType, ref)
		switch {
		case err == nil:
		case ref.Required:
			return nil, fmt.Errorf("prompt %q: required fragment %q: %w", taskType, ref.Name, err)
		default:
			warnings = append(warnings, fmt.Sprintf(
				"prompt %q: optional fragment %q was not loaded and renders empty: %v", taskType, ref.Name, err))
		}
	}
	return warnings, nil
}

// isDynamic reports whether a fragment reference is chosen by a template
// variable, matching PromptKit's pack compiler.
func isDynamic(ref prompt.FragmentRef) bool {
	return strings.Contains(ref.Name, "{{") || strings.Contains(ref.Path, "{{")
}
