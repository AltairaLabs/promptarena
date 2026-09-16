package engine

import (
	"sort"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/skills"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

// SkillsFactory owns the engine-wide skills executor and seeds each run's
// activation state.
//
// From PromptKit v2.3.0 the executor is catalog and policy only — what skills
// exist, what the pack ceiling is, what activating one would grant — and holds
// no conversation state. The state a run owns is a skills.ActiveSet, which the
// run carries on its context. That is what makes a single executor safe to
// register engine-wide even though arena runs many conversations at once; see
// AltairaLabs/PromptKit#2011.
type SkillsFactory struct {
	executor *skills.Executor
	// catalog is the discovered skill registry, kept so callers can enumerate
	// what exists without reaching inside the executor.
	catalog   *skills.Registry
	preloaded []*skills.Skill
}

// preload replays the preload: true skills into a run's set. Best-effort,
// matching PromptKit's SDK capability: a skill that fails to preload can still
// be activated on demand.
func (f *SkillsFactory) preload(set *skills.ActiveSet) {
	for _, sk := range f.preloaded {
		_, _ = f.executor.ActivateIn(set, sk.Name)
	}
}

// skillGrantCeiling returns the tools a skill's allowed-tools may grant.
//
// The SDK's equivalent is the prompt's declared tools, because a pack is always
// the source there. Arena has two sources and usually only the second: a
// compiled pack when the config names one, and the config's own `tools:`
// entries, which the loader parses into the tool registry rather than into
// LoadedPack. Deriving the ceiling from LoadedPack alone leaves grants inert
// for every arena-native config — which is all of examples/.
//
// The registry is the honest ceiling: a tool it does not hold cannot be
// executed, so granting it would offer the model something that can only fail.
// This runs before the skill__, workflow__ and memory__ control tools are
// registered, so those are naturally excluded — a skill has no business
// granting them.
func skillGrantCeiling(cfg *arenaconfig.Config, toolRegistry *tools.Registry) []string {
	names := map[string]bool{}
	if cfg.LoadedPack != nil {
		for name := range cfg.LoadedPack.Tools {
			names[name] = true
		}
	}
	if toolRegistry != nil {
		for name := range toolRegistry.GetTools() {
			names[name] = true
		}
	}

	ceiling := make([]string, 0, len(names))
	for name := range names {
		ceiling = append(ceiling, name)
	}
	sort.Strings(ceiling)
	return ceiling
}
