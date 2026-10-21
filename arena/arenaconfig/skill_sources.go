package arenaconfig

import (
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
