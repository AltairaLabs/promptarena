package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"

	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

const (
	outputFilePerm = 0o600
	outputDirPerm  = 0o755
)

var version = devVersion //nolint:gochecknoglobals // overridden via ldflags at build time

// devVersion is the version of a build that was not stamped via ldflags.
const devVersion = "dev"

// packcVersion is the version packc reports and stamps into compiled packs.
func packcVersion() string {
	return resolveVersion(version, debug.ReadBuildInfo)
}

// resolveVersion returns the ldflags-stamped version when there is one. A
// `go install …/packc@vX.Y.Z` build is not stamped, so it falls back to the
// module version recorded in the binary's build info, and only reports "dev"
// when that is unknown too ("(devel)" or empty).
func resolveVersion(stamped string, buildInfo func() (*debug.BuildInfo, bool)) string {
	if stamped != devVersion {
		return stamped
	}
	info, ok := buildInfo()
	if !ok || info == nil || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return stamped
	}
	return info.Main.Version
}

const warningFormat = "  - %s\n"

// writePackFile writes data to path, creating the parent directory if it
// does not already exist. Matches the behavior of `go build -o` so that
// `packc compile -o dist/foo.pack.json` works in fresh CI checkouts.
func writePackFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, outputDirPerm); err != nil {
			return fmt.Errorf("creating output directory %s: %w", dir, err)
		}
	}
	return os.WriteFile(path, data, outputFilePerm)
}

func printPackInfo(pack *prompt.Pack) {
	fmt.Printf("\n=== Pack Information ===\n")
	fmt.Printf("Pack: %s\n", pack.Name)
	fmt.Printf("ID: %s\n", pack.ID)
	fmt.Printf("Version: %s\n", pack.Version)
	if pack.Description != "" {
		fmt.Printf("Description: %s\n", pack.Description)
	}
}

func printTemplateEngine(pack *prompt.Pack) {
	if pack.TemplateEngine != nil {
		fmt.Printf("\nTemplate Engine: %s (%s)\n", pack.TemplateEngine.Version, pack.TemplateEngine.Syntax)
		if len(pack.TemplateEngine.Features) > 0 {
			fmt.Printf("Features: %v\n", pack.TemplateEngine.Features)
		}
	}
}

func printPrompts(pack *prompt.Pack) {
	fmt.Printf("\n=== Prompts (%d) ===\n", len(pack.Prompts))
	taskTypes := make([]string, 0, len(pack.Prompts))
	for taskType := range pack.Prompts {
		taskTypes = append(taskTypes, taskType)
	}
	sort.Strings(taskTypes)
	for _, taskType := range taskTypes {
		printPromptDetails(taskType, pack.Prompts[taskType])
	}
}

func printPromptDetails(taskType string, p *prompt.PackPrompt) {
	fmt.Printf("\n[%s]\n", taskType)
	fmt.Printf("  Name: %s\n", p.Name)
	if p.Description != "" {
		fmt.Printf("  Description: %s\n", p.Description)
	}
	fmt.Printf("  Version: %s\n", p.Version)

	printPromptVariables(p)
	printPromptTools(p)
	printPromptValidators(p)
	printPromptTestedModels(p)
}

func printPromptVariables(p *prompt.PackPrompt) {
	requiredVars := []string{}
	optionalVars := []string{}
	for _, v := range p.Variables {
		if v.Required {
			requiredVars = append(requiredVars, v.Name)
		} else {
			optionalVars = append(optionalVars, v.Name)
		}
	}
	fmt.Printf("  Variables: %d required, %d optional\n", len(requiredVars), len(optionalVars))
	if len(requiredVars) > 0 {
		fmt.Printf("    Required: %v\n", requiredVars)
	}
}

func printPromptTools(p *prompt.PackPrompt) {
	if len(p.Tools) > 0 {
		fmt.Printf("  Tools (%d): %v\n", len(p.Tools), p.Tools)
	}
}

func printPromptValidators(p *prompt.PackPrompt) {
	if len(p.Validators) > 0 {
		fmt.Printf("  Validators (%d):\n", len(p.Validators))
		for _, v := range p.Validators {
			// Validator.Enabled is *bool again in PromptKit 1.8.0. The schema
			// defaults it to true, so an unset flag reads as enabled — the same
			// default the prompt registry applies when it loads a config.
			enabled := "disabled"
			if packspec.Deref(v.Enabled, true) {
				enabled = "enabled"
			}
			fmt.Printf("    - %s (%s)\n", v.Type, enabled)
		}
	}
}

// successRateAsPercent scales the spec's 0-1 success rate for display.
const successRateAsPercent = 100

func printPromptTestedModels(p *prompt.PackPrompt) {
	if len(p.TestedModels) > 0 {
		fmt.Printf("  Tested Models (%d):\n", len(p.TestedModels))
		for _, tm := range p.TestedModels {
			fmt.Printf("    - %s/%s: %.1f%% success, avg %.0f tokens\n",
				tm.Provider, tm.Model,
				packspec.Deref(tm.SuccessRate, 0)*successRateAsPercent,
				packspec.Deref(tm.AvgTokens, 0))
		}
	}
}

func printFragments(pack *prompt.Pack) {
	if len(pack.Fragments) > 0 {
		fmt.Printf("\nShared Fragments (%d): %v\n", len(pack.Fragments), getFragmentNames(pack.Fragments))
	}
}

func printWorkflow(pack *prompt.Pack) {
	if pack.Workflow == nil {
		return
	}
	fmt.Printf("\n=== Workflow (v%d) ===\n", pack.Workflow.Version)
	fmt.Printf("Entry: %s\n", pack.Workflow.Entry)
	fmt.Printf("States (%d):\n", len(pack.Workflow.States))
	stateNames := make([]string, 0, len(pack.Workflow.States))
	for name := range pack.Workflow.States {
		stateNames = append(stateNames, name)
	}
	sort.Strings(stateNames)
	for _, name := range stateNames {
		printWorkflowState(name, pack.Workflow.States[name])
	}
}

func printWorkflowState(name string, state *prompt.WorkflowState) {
	fmt.Printf("  [%s] prompt_task=%s\n", name, state.PromptTask)
	if state.Description != "" {
		fmt.Printf("    Description: %s\n", state.Description)
	}
	if len(state.OnEvent) > 0 {
		events := make([]string, 0, len(state.OnEvent))
		for event := range state.OnEvent {
			events = append(events, event)
		}
		sort.Strings(events)
		for _, event := range events {
			fmt.Printf("    on %s → %s\n", event, state.OnEvent[event])
		}
	}
	if state.Persistence != "" {
		fmt.Printf("    Persistence: %s\n", state.Persistence)
	}
	if state.Orchestration != nil && *state.Orchestration != "" {
		fmt.Printf("    Orchestration: %s\n", *state.Orchestration)
	}
}

func printAgents(pack *prompt.Pack) {
	if pack.Agents == nil {
		return
	}
	fmt.Printf("\n=== Agents ===\n")
	fmt.Printf("Entry: %s\n", pack.Agents.Entry)
	fmt.Printf("Members (%d):\n", len(pack.Agents.Members))
	memberNames := make([]string, 0, len(pack.Agents.Members))
	for name := range pack.Agents.Members {
		memberNames = append(memberNames, name)
	}
	sort.Strings(memberNames)
	for _, name := range memberNames {
		agent := pack.Agents.Members[name]
		fmt.Printf("  [%s]\n", name)
		if agent.Description != "" {
			fmt.Printf("    Description: %s\n", agent.Description)
		}
		if len(agent.Tags) > 0 {
			fmt.Printf("    Tags: %v\n", agent.Tags)
		}
		if len(agent.InputModes) > 0 {
			fmt.Printf("    Input modes: %v\n", agent.InputModes)
		}
		if len(agent.OutputModes) > 0 {
			fmt.Printf("    Output modes: %v\n", agent.OutputModes)
		}
	}
}

func printTools(pack *prompt.Pack) {
	if len(pack.Tools) == 0 {
		return
	}
	names := make([]string, 0, len(pack.Tools))
	for name := range pack.Tools {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Printf("\n=== Tools (%d) ===\n", len(names))
	for _, name := range names {
		if t := pack.Tools[name]; t != nil && t.Description != "" {
			fmt.Printf("  %s: %s\n", name, t.Description)
		} else {
			fmt.Printf("  %s\n", name)
		}
	}
}

func printEvals(pack *prompt.Pack) {
	if len(pack.Evals) == 0 {
		return
	}
	fmt.Printf("\n=== Evals (%d) ===\n", len(pack.Evals))
	for _, e := range pack.Evals {
		if e == nil {
			continue
		}
		fmt.Printf("  %s (type: %s, trigger: %s)\n", e.ID, e.Type, e.Trigger)
	}
}

func printSkills(pack *prompt.Pack) {
	if len(pack.Skills) == 0 {
		return
	}
	fmt.Printf("\n=== Skills (%d) ===\n", len(pack.Skills))
	for _, src := range pack.Skills {
		if src == nil {
			continue
		}
		if dir := prompt.SkillPath(src); dir != "" {
			fmt.Printf("  path: %s\n", dir)
		} else {
			fmt.Printf("  inline: %s\n", src.Name)
		}
	}
}

func printCompositions(pack *prompt.Pack) {
	if len(pack.Compositions) == 0 {
		return
	}
	names := make([]string, 0, len(pack.Compositions))
	for name := range pack.Compositions {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Printf("\n=== Compositions (%d) ===\n", len(names))
	for _, name := range names {
		fmt.Printf("  %s\n", name)
	}
}

func printMetadata(pack *prompt.Pack) {
	if pack.Metadata != nil {
		fmt.Printf("\nPack Metadata:\n")
		if pack.Metadata.Domain != "" {
			fmt.Printf("  Domain: %s\n", pack.Metadata.Domain)
		}
		if pack.Metadata.Language != "" {
			fmt.Printf("  Language: %s\n", pack.Metadata.Language)
		}
		if len(pack.Metadata.Tags) > 0 {
			fmt.Printf("  Tags: %v\n", pack.Metadata.Tags)
		}
	}
}

func printCompilationInfo(pack *prompt.Pack) {
	if pack.Compilation != nil {
		fmt.Printf("\nCompilation: %s, %s, Schema: %s\n",
			pack.Compilation.CompiledWith,
			pack.Compilation.CreatedAt,
			pack.Compilation.Schema)
	}
}

func getFragmentNames(fragments map[string]string) []string {
	names := make([]string, 0, len(fragments))
	for name := range fragments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// printPackSummary prints a summary of the compiled pack.
func printPackSummary(pack *prompt.Pack, outputFile string) {
	fmt.Printf("✓ Pack compiled successfully: %s\n", outputFile)
	prompts := pack.ListPrompts()
	sort.Strings(prompts)
	fmt.Printf("  Contains %d prompts: %v\n", len(pack.Prompts), prompts)
	if len(pack.Tools) > 0 {
		toolNames := make([]string, 0, len(pack.Tools))
		for name := range pack.Tools {
			toolNames = append(toolNames, name)
		}
		sort.Strings(toolNames)
		fmt.Printf("  Contains %d tools: %v\n", len(pack.Tools), toolNames)
	}
	if len(pack.Evals) > 0 {
		fmt.Printf("  Pack-level evals: %d\n", len(pack.Evals))
	}
	if pack.Workflow != nil {
		fmt.Printf("  Workflow: %d states (entry: %s)\n", len(pack.Workflow.States), pack.Workflow.Entry)
	}
	if pack.Agents != nil {
		fmt.Printf("  Agents: %d members (entry: %s)\n", len(pack.Agents.Members), pack.Agents.Entry)
	}
}
