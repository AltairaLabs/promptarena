package compiler

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt/schema"
)

// Stages a Problem can come from, in the order Check runs them.
const (
	StagePackID       = "pack_id"      // the pack ID derived from (or given for) the config
	StageLoad         = "load"         // reading the arena config and the files it references
	StageCompile      = "compile"      // turning the config into a pack (parse errors, media, metadata)
	StageSkills       = "skills"       // skill sources and paths
	StageWorkflow     = "workflow"     // the workflow state machine
	StageCompositions = "compositions" // composition step graphs and their references
	StageAgents       = "agents"       // the agents block
	StagePack         = "pack"         // pack structure, as `packc validate` checks it
	StagePackSchema   = "pack_schema"  // the compiled pack against the PromptPack spec schema
)

// Problem is one finding from Check. It says which stage found it, where in the
// compiled pack it is (when known), which source file to edit, and what is wrong.
type Problem struct {
	Stage string `json:"stage"`
	// Path is the location in the compiled pack, e.g. "prompts.builder.tool_policy".
	Path string `json:"path,omitempty"`
	// Source is the file to edit: the prompt config a prompts.<task> path came
	// from, otherwise the arena config.
	Source  string `json:"source,omitempty"`
	Message string `json:"message"`
}

// String renders the problem on one line.
func (p Problem) String() string {
	s := "[" + p.Stage + "] "
	if p.Path != "" {
		s += p.Path + ": "
	}
	return s + p.Message
}

// CheckResult is everything Check found. Pack and JSON are set when the config
// compiled far enough to produce a pack, even if later stages found errors.
type CheckResult struct {
	Pack     *prompt.Pack
	JSON     []byte
	Errors   []Problem
	Warnings []Problem
}

// Err returns nil when there are no errors, otherwise one error listing them all.
func (r *CheckResult) Err() error {
	switch len(r.Errors) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("%s", r.Errors[0])
	}
	lines := make([]string, len(r.Errors))
	for i, p := range r.Errors {
		lines[i] = "  - " + p.String()
	}
	return fmt.Errorf("%d problems:\n%s", len(r.Errors), strings.Join(lines, "\n"))
}

// WarningMessages returns the warnings as one-line strings.
func (r *CheckResult) WarningMessages() []string {
	out := make([]string, len(r.Warnings))
	for i, w := range r.Warnings {
		out[i] = w.String()
	}
	return out
}

// Check compiles configFile exactly as `packc compile` does, then validates the
// result as `packc validate` does, without writing anything.
//
// It is the single definition of "this config builds a valid pack": Compile is
// Check plus the output, and `promptarena validate` reports Check's findings,
// so any check added here reaches both. Unlike a compile, it does not stop at
// the first failing stage once a pack exists: it reports every problem it can,
// so a caller fixing a config (often a coding agent) gets the whole list in one
// pass. Only a config that cannot be loaded or compiled at all stops early.
func Check(configFile string, opts ...Option) *CheckResult {
	options := applyOptions(opts)
	r := &CheckResult{}

	if err := resolvePackID(&options, configFile); err != nil {
		r.add(true, Problem{Stage: StagePackID, Source: configFile, Message: err.Error()})
		return r
	}

	cfg, err := arenaconfig.LoadConfig(configFile)
	if err != nil {
		r.add(true, Problem{Stage: StageLoad, Source: configFile, Message: fmt.Sprintf("loading arena config: %v", err)})
		return r
	}
	src := newSourceMap(cfg, configFile)

	pack, warnings, err := compilePack(cfg, configFile, options)
	if err != nil {
		r.add(true, Problem{Stage: StageCompile, Source: configFile, Message: err.Error()})
		return r
	}
	r.Pack = pack
	for _, w := range warnings {
		r.add(false, Problem{Stage: StageCompile, Source: configFile, Message: w})
	}

	r.checkPack(pack, filepath.Dir(configFile), configFile)

	data, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		r.add(true, Problem{Stage: StageCompile, Source: configFile, Message: fmt.Sprintf("marshaling pack: %v", err)})
		return r
	}
	r.JSON = data

	if !options.skipSchemaValidation {
		r.checkSchema(data, src)
	}
	return r
}

func (r *CheckResult) add(isErr bool, p Problem) {
	if isErr {
		r.Errors = append(r.Errors, p)
	} else {
		r.Warnings = append(r.Warnings, p)
	}
}

func (r *CheckResult) addAll(isErr bool, stage, source string, msgs []string) {
	for _, m := range msgs {
		r.add(isErr, Problem{Stage: stage, Source: source, Message: m})
	}
}

// checkPack runs every structural check `packc validate` runs on a pack file,
// plus the composition check `packc compile` adds.
func (r *CheckResult) checkPack(pack *prompt.Pack, configDir, configFile string) {
	skillErrs, skillWarnings := runSkillValidation(pack, configDir)
	r.addAll(true, StageSkills, configFile, skillErrs)
	r.addAll(false, StageSkills, configFile, skillWarnings)

	wf := pack.ValidateWorkflow()
	r.addAll(true, StageWorkflow, configFile, wf.Errors)
	r.addAll(false, StageWorkflow, configFile, wf.Warnings)

	if len(pack.Compositions) > 0 {
		c := pack.ValidateCompositions()
		r.addAll(true, StageCompositions, configFile, c.Errors)
		r.addAll(false, StageCompositions, configFile, c.Warnings)
	}

	var agentErrs, agentWarnings []string
	if pack.Agents != nil {
		agentErrs, agentWarnings = pack.ValidateAgents()
		r.addAll(true, StageAgents, configFile, agentErrs)
		r.addAll(false, StageAgents, configFile, agentWarnings)
	}

	// pack.Validate repeats the workflow and agent findings reported above;
	// what is left is the pack's own structure, which `packc validate` treats
	// as blocking.
	reported := map[string]bool{}
	for _, list := range [][]string{wf.Errors, wf.Warnings, agentErrs, agentWarnings} {
		for _, m := range list {
			reported[m] = true
		}
	}
	for _, m := range pack.Validate() {
		if !reported[m] {
			r.add(true, Problem{Stage: StagePack, Source: configFile, Message: m})
		}
	}
}

// checkSchema validates the compiled pack against the PromptPack spec schema,
// one Problem per violation.
func (r *CheckResult) checkSchema(data []byte, src sourceMap) {
	loader, err := schema.GetSchemaLoader(schema.ExtractSchemaURL(data))
	if err != nil {
		msg := fmt.Sprintf("schema validation could not be performed: %v", err)
		r.add(true, Problem{Stage: StagePackSchema, Message: msg})
		return
	}
	result, err := schema.ValidateJSONAgainstLoader(data, loader)
	if err != nil {
		r.add(true, Problem{Stage: StagePackSchema, Message: err.Error()})
		return
	}
	for _, e := range result.Errors {
		path := strings.TrimPrefix(e.Field, "(root).")
		if path == "(root)" {
			path = ""
		}
		msg := e.Description
		if e.Value != nil {
			msg = fmt.Sprintf("%s (value: %v)", msg, e.Value)
		}
		r.add(true, Problem{Stage: StagePackSchema, Path: path, Source: src.forPath(path), Message: msg})
	}
}

// sourceMap points a compiled-pack path back at the file it was authored in.
type sourceMap struct {
	arenaConfig string
	prompts     map[string]string // task type → prompt config file
}

func newSourceMap(cfg *arenaconfig.Config, configFile string) sourceMap {
	m := sourceMap{arenaConfig: configFile, prompts: map[string]string{}}
	dir := filepath.Dir(configFile)
	for _, pc := range cfg.LoadedPromptConfigs {
		if pc == nil || pc.FilePath == "" {
			continue
		}
		taskType := pc.TaskType
		if c, ok := pc.Config.(*prompt.Config); ok && c != nil && c.Spec.TaskType != "" {
			taskType = c.Spec.TaskType
		}
		path := pc.FilePath
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		m.prompts[taskType] = path
	}
	return m
}

// forPath returns the prompt config file for a prompts.<task>… path, and the
// arena config for anything else.
func (m sourceMap) forPath(path string) string {
	if rest, ok := strings.CutPrefix(path, "prompts."); ok {
		task, _, _ := strings.Cut(rest, ".")
		if f, ok := m.prompts[task]; ok {
			return f
		}
	}
	return m.arenaConfig
}
