package arenaconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
	"github.com/AltairaLabs/PromptKit/runtime/v2/skills"
)

// SkillSources converts the loaded skill sources into the runtime's form. The
// engine discovers the skills a run can activate from these, and validation
// reads the same set to see which tools a skill can grant.
func (c *Config) SkillSources() []skills.SkillSource {
	sources := make([]skills.SkillSource, len(c.LoadedSkillSources))
	for i := range c.LoadedSkillSources {
		s := &c.LoadedSkillSources[i]
		sources[i] = skills.SkillSource{
			Dir:          prompt.SkillPath(s),
			Name:         s.Name,
			Description:  s.Description,
			Instructions: s.Instructions,
			Preload:      packspec.Deref(s.Preload, false),
		}
	}
	return sources
}

// IsSkillRef reports whether a skill path is an installed-skill reference
// (`@org/name`) rather than a directory.
func IsSkillRef(path string) bool {
	return strings.HasPrefix(path, "@")
}

// ResolveSkillRef resolves an `@org/name` skill reference to the directory
// `promptarena skill install` put it in. It looks where the installer writes:
// the project-level .promptkit/skills beside baseDir (the config or pack
// directory), then the project-level directory of the working directory, then
// the user-level directory. The first two differ when a config is run from
// another directory with --config.
func ResolveSkillRef(ref, baseDir string) (string, error) {
	parsed, err := skills.ParseSkillRef(ref)
	if err != nil {
		return "", err
	}
	if baseDir != "" {
		if abs, absErr := filepath.Abs(baseDir); absErr == nil {
			candidate := filepath.Join(abs, ".promptkit", "skills", parsed.Org, parsed.Name)
			if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
				return candidate, nil
			}
		}
	}
	inst, err := skills.NewInstaller()
	if err != nil {
		return "", err
	}
	path, err := inst.Resolve(parsed)
	if err != nil {
		return "", fmt.Errorf("%w (install it with: promptarena skill install @%s)", err, parsed.FullName())
	}
	return path, nil
}
