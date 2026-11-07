package compiler

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// rePackVersion is the PromptPack schema's pattern for the pack version:
// Semantic Versioning 2.0.0 with an optional "v" prefix.
var rePackVersion = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
	`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// validatePackVersion rejects a version the PromptPack schema would reject, so a
// bad --pack-version fails before compiling rather than at schema validation.
func validatePackVersion(v string) error {
	if !rePackVersion.MatchString(v) {
		return fmt.Errorf("invalid pack version %q: must be a semantic version such as 1.2.0 or v1.2.0", v)
	}
	return nil
}

// declaredEngine is one prompt config's spec.template_engine, kept with the
// prompt it was declared on.
type declaredEngine struct {
	prompt string
	engine *prompt.TemplateEngineInfo
}

// declaredTemplateEngines returns the template engines the prompt configs
// declare, sorted by prompt so the choice never depends on map order. It must
// run before compilation: the registry fills in a default engine on the shared
// config when it loads a prompt that declares none.
func declaredTemplateEngines(cfg *arenaconfig.Config) []declaredEngine {
	var out []declaredEngine
	for name, pd := range cfg.LoadedPromptConfigs {
		pc, ok := pd.Config.(*prompt.Config)
		if !ok || pc == nil || pc.Spec.TemplateEngine == nil {
			continue
		}
		e := *pc.Spec.TemplateEngine
		out = append(out, declaredEngine{prompt: name, engine: &e})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].prompt < out[j].prompt })
	return out
}

// resolveTemplateEngine sets the pack's single template_engine from the
// engines the prompts declare. PromptKit takes whichever prompt it reaches
// first, in map order, so prompts that disagree would compile to a different
// pack on each run. A pack has one engine, so disagreement is an error.
func resolveTemplateEngine(declared []declaredEngine, pack *prompt.Pack) error {
	if len(declared) == 0 {
		return nil
	}
	first := declared[0]
	var differing []string
	for _, d := range declared[1:] {
		if !reflect.DeepEqual(d.engine, first.engine) {
			differing = append(differing, fmt.Sprintf("%q (%s %s)", d.prompt, d.engine.Version, d.engine.Syntax))
		}
	}
	if len(differing) > 0 {
		return fmt.Errorf("prompts declare different template engines: %q uses %s %s, but %s; "+
			"a pack has one template_engine, so every prompt that sets one must set the same",
			first.prompt, first.engine.Version, first.engine.Syntax, strings.Join(differing, ", "))
	}
	pack.TemplateEngine = first.engine
	return nil
}

// undeclaredPromptTools reports each tool a prompt names that the pack's tools
// object does not define. The PromptPack schema says a prompt's tools "must be
// defined in the pack-level 'tools' object". These are warnings, not errors:
// Arena also offers tools the pack does not carry (MCP, workflow, memory,
// skills), and a prompt may list those.
func undeclaredPromptTools(pack *prompt.Pack) []string {
	tasks := make([]string, 0, len(pack.Prompts))
	for task := range pack.Prompts {
		tasks = append(tasks, task)
	}
	sort.Strings(tasks)

	var out []string
	for _, task := range tasks {
		p := pack.Prompts[task]
		if p == nil {
			continue
		}
		for _, tool := range p.Tools {
			if _, ok := pack.Tools[tool]; !ok {
				out = append(out, fmt.Sprintf(
					"prompt %q: tool %q is not defined in the pack's tools", task, tool))
			}
		}
	}
	return out
}
