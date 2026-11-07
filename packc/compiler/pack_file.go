package compiler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AltairaLabs/promptarena/v2/arena/configschema"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/persistence/memory"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// ValidatePackFile validates a compiled pack file, as `packc validate` does.
//
// The file must parse as JSON before anything else is checked, so a syntax
// error is reported as one, not as schema errors against whatever prefix
// happened to parse. It is then checked against the PromptPack schema and by
// the same structural checks Check runs on a freshly compiled pack, so a pack
// that `packc compile` accepts is never rejected here for a finding compile
// treats as a warning.
func ValidatePackFile(packFile string) *CheckResult {
	r := &CheckResult{}

	data, err := os.ReadFile(packFile) //nolint:gosec // the caller names the pack file to validate
	if err != nil {
		r.add(true, Problem{Stage: StageLoad, Source: packFile, Message: fmt.Sprintf("reading pack file: %v", err)})
		return r
	}

	var pack prompt.Pack
	if err := json.Unmarshal(data, &pack); err != nil {
		r.add(true, Problem{Stage: StageLoad, Source: packFile, Message: fmt.Sprintf("invalid JSON: %v", err)})
		return r
	}
	r.Pack = &pack
	r.JSON = data

	r.checkSchema(data, sourceMap{arenaConfig: packFile})
	r.checkPack(&pack, filepath.Dir(packFile), packFile)
	return r
}

// CompilePromptFile compiles a single prompt config file into a pack, as
// `packc compile-prompt` does, and validates the result the way Check
// validates a compiled arena config: the prompt config against its schema
// first, then the pack's structure and the PromptPack schema. Nothing is
// written.
func CompilePromptFile(promptFile string, opts ...Option) *CheckResult {
	options := applyOptions(opts)
	r := &CheckResult{}

	if options.packVersion != "" {
		if err := validatePackVersion(options.packVersion); err != nil {
			r.add(true, Problem{Stage: StagePackVersion, Source: promptFile, Message: err.Error()})
			return r
		}
	}

	promptConfig, err := loadPromptFile(promptFile)
	if err != nil {
		r.add(true, Problem{Stage: StageLoad, Source: promptFile, Message: err.Error()})
		return r
	}

	pack, warnings, err := compileSinglePrompt(promptConfig, filepath.Dir(promptFile), options)
	if err != nil {
		r.add(true, Problem{Stage: StageCompile, Source: promptFile, Message: err.Error()})
		return r
	}
	r.Pack = pack
	r.addAll(false, StageCompile, promptFile, warnings)

	r.checkPack(pack, filepath.Dir(promptFile), promptFile)

	data, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		r.add(true, Problem{Stage: StageCompile, Source: promptFile, Message: fmt.Sprintf("marshaling pack: %v", err)})
		return r
	}
	r.JSON = data

	if !options.skipSchemaValidation {
		r.checkSchema(data, sourceMap{arenaConfig: promptFile})
	}
	return r
}

// loadPromptFile reads a prompt config and checks it against the PromptConfig
// schema before parsing it, exactly as the arena config loader does for each
// prompt it references.
func loadPromptFile(promptFile string) (*prompt.Config, error) {
	data, err := os.ReadFile(promptFile) //nolint:gosec // the caller names the prompt file to compile
	if err != nil {
		return nil, fmt.Errorf("reading prompt file: %w", err)
	}
	if err = configschema.Check(config.ConfigTypePromptConfig, data); err != nil {
		return nil, fmt.Errorf("prompt config failed schema validation: %w", err)
	}
	promptConfig, err := prompt.ParseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("parsing prompt config: %w", err)
	}
	return promptConfig, nil
}

// compileSinglePrompt builds the one-prompt pack. PromptKit's single-config
// compile leaves $schema unset, which the registry path sets, so it is filled
// in here; --pack-version, when given, replaces the prompt's spec.version.
func compileSinglePrompt(
	promptConfig *prompt.Config, promptDir string, options compileOptions,
) (*prompt.Pack, []string, error) {
	var warnings []string
	for _, w := range validateMediaReferences(promptConfig, promptDir) {
		warnings = append(warnings, fmt.Sprintf("media(%s): %s", promptConfig.Spec.TaskType, w))
	}

	memRepo := memory.NewPromptRepository()
	if err := memRepo.SavePrompt(promptConfig); err != nil {
		return nil, nil, fmt.Errorf("registering prompt: %w", err)
	}
	pc := prompt.NewPackCompiler(prompt.NewRegistryWithRepository(memRepo))
	pack, err := pc.Compile(promptConfig.Spec.TaskType, options.compilerVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("compilation failed: %w", err)
	}
	if pack.Schema == nil {
		pack.Schema = packspec.Ptr(prompt.PromptPackSchemaURL)
	}
	if options.packVersion != "" {
		pack.Version = options.packVersion
	}
	return pack, warnings, nil
}
