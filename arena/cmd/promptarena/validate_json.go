package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/packc/compiler"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
)

// Stages `promptarena validate` reports, in the order they run. A stage runs
// only when the ones before it got far enough to feed it. After these come the
// pack stages from compiler.Check ("compile", "skills", "workflow",
// "compositions", "agents", "pack", "pack_schema") — the same checks
// `packc compile` and `packc validate` run, so validate is a superset of packc
// by construction rather than by keeping two lists in step.
const (
	stageSchema     = "schema"     // the file against its config type's JSON schema
	stageLoad       = "load"       // loading the arena config and every file it references
	stageConfig     = "config"     // arena cross-reference and consistency checks
	stageAssertions = "assertions" // scenario assertion types against the eval registry
	stagePack       = "pack"       // building and validating the pack, as packc does
)

// validateFinding is one error or warning. Field is the location within the
// file that was checked; Source is the file to edit when that is a different
// file (a prompt config, a scenario).
type validateFinding struct {
	Stage       string   `json:"stage"`
	Field       string   `json:"field"`
	Source      string   `json:"source,omitempty"`
	Description string   `json:"description"`
	Keyword     string   `json:"keyword,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

func (f validateFinding) String() string {
	s := "[" + f.Stage + "] "
	if field := displayField(f.Field); field != "" && field != rootField {
		s += field + ": "
	}
	s += f.Description
	if f.Source != "" {
		s += " (in " + f.Source + ")"
	}
	return s
}

// validateJSONReport is the result `validate` builds, emitted as-is by
// `validate --json` and rendered as text otherwise. It is written to be read
// by a coding agent fixing a config: every problem found, not just the first,
// each with the file to edit.
type validateJSONReport struct {
	File  string `json:"file"`
	Type  string `json:"type"`
	Valid bool   `json:"valid"`
	// Stages lists the stages that ran, so a reader can tell "passed" from
	// "never checked".
	Stages []string `json:"stages"`
	// Notes explains stages that were deliberately skipped.
	Notes    []string          `json:"notes,omitempty"`
	Errors   []validateFinding `json:"errors"`
	Warnings []validateFinding `json:"warnings"`

	schemaErrors []config.SchemaValidationError // kept for the richer text rendering
}

func buildValidateReport(file, configType string, result *config.SchemaValidationResult) validateJSONReport {
	report := validateJSONReport{
		File:     file,
		Type:     configType,
		Valid:    result.Valid,
		Stages:   []string{stageSchema},
		Errors:   []validateFinding{},
		Warnings: []validateFinding{},
	}
	report.schemaErrors = result.Errors
	for _, e := range result.Errors {
		report.Errors = append(report.Errors, validateFinding{
			Stage:       stageSchema,
			Field:       e.Field,
			Description: e.Description,
			Keyword:     e.Keyword,
			Suggestions: e.Suggestions,
		})
	}
	return report
}

// collectValidation runs every stage that applies to filePath and returns the
// report. The error is for a file that could not be checked at all (missing,
// unreadable, unknown type), not for a file that failed validation.
func collectValidation(filePath, typeOption string, schemaOnly bool) (*validateJSONReport, error) {
	data, configType, err := prepareValidationWithType(filePath, typeOption)
	if err != nil {
		return nil, err
	}
	result, err := validateWithSchema(data, config.ConfigType(configType))
	if err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}
	report := buildValidateReport(filePath, configType, result)
	if result.Valid && !schemaOnly && configType == string(config.ConfigTypeArena) {
		report.addArenaStages(filePath)
	}
	report.Valid = len(report.Errors) == 0
	return &report, nil
}

// addArenaStages runs the checks that need the whole arena config loaded: the
// arena's own consistency checks, then the pack checks packc runs.
func (r *validateJSONReport) addArenaStages(filePath string) {
	r.Stages = append(r.Stages, stageLoad)
	cfg, err := arenaconfig.LoadConfig(filePath)
	if err != nil {
		r.addError(validateFinding{Stage: stageLoad, Description: fmt.Sprintf("config loading failed: %v", err)})
		return
	}

	r.Stages = append(r.Stages, stageConfig)
	validator := arenaconfig.NewConfigValidatorWithPath(cfg, filePath)
	_ = validator.Validate() // findings are read below, one per problem
	for _, e := range validator.GetErrors() {
		r.addError(validateFinding{Stage: stageConfig, Description: e})
	}
	for _, w := range validator.GetWarnings() {
		r.addWarning(validateFinding{Stage: stageConfig, Description: w})
	}

	if len(cfg.LoadedScenarios) > 0 {
		r.Stages = append(r.Stages, stageAssertions)
		for _, e := range assertions.ValidateAssertionTypes(cfg.LoadedScenarios, evals.NewEvalTypeRegistry()) {
			r.addError(validateFinding{Stage: stageAssertions, Description: e})
		}
	}

	// A config with no prompts runs scenarios against a pack built elsewhere;
	// there is nothing for packc to build, so failing it here would be wrong.
	if len(cfg.LoadedPromptConfigs) == 0 {
		r.Notes = append(r.Notes,
			"pack checks skipped: the config declares no prompt_configs, so it does not build a pack")
		return
	}
	r.Stages = append(r.Stages, stagePack)
	check := compiler.Check(filePath)
	for _, p := range check.Errors {
		r.addError(findingFromProblem(p, filePath))
	}
	for _, p := range check.Warnings {
		r.addWarning(findingFromProblem(p, filePath))
	}
}

// findingFromProblem converts a packc problem, naming its source file only
// when it is not the file being validated.
func findingFromProblem(p compiler.Problem, filePath string) validateFinding {
	f := validateFinding{Stage: p.Stage, Field: p.Path, Description: p.Message}
	if p.Source != filePath {
		f.Source = p.Source
	}
	return f
}

// addError records an error unless the same message is already reported:
// the arena and pack stages both validate the workflow, for one.
func (r *validateJSONReport) addError(f validateFinding) {
	if !r.reported(f) {
		r.Errors = append(r.Errors, f)
	}
}

func (r *validateJSONReport) addWarning(f validateFinding) {
	if !r.reported(f) {
		r.Warnings = append(r.Warnings, f)
	}
}

func (r *validateJSONReport) reported(f validateFinding) bool {
	for _, list := range [][]validateFinding{r.Errors, r.Warnings} {
		for _, e := range list {
			if e.Field == f.Field && e.Description == f.Description {
				return true
			}
		}
	}
	return false
}

// err summarizes a failed report as an error that names every problem.
func (r *validateJSONReport) err() error {
	if r.Valid {
		return nil
	}
	lines := make([]string, len(r.Errors))
	for i, f := range r.Errors {
		lines[i] = "  - " + f.String()
	}
	return fmt.Errorf("validation failed with %d error(s):\n%s", len(r.Errors), strings.Join(lines, "\n"))
}

// writeValidateJSON runs validation and writes the JSON report. It returns a
// non-nil error when validation fails so the process exits non-zero, while still
// emitting the JSON report to w first.
func writeValidateJSON(w io.Writer, filePath, typeOption string, schemaOnly bool) error {
	report, err := collectValidation(filePath, typeOption, schemaOnly)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return err
	}
	if !report.Valid {
		return fmt.Errorf("validation failed with %d error(s)", len(report.Errors))
	}
	return nil
}
