package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	_ "github.com/AltairaLabs/PromptKit/runtime/v2/evals/handlers" // register built-in eval handlers
)

var validateCmd = &cobra.Command{
	Use:   "validate [file]",
	Short: "Validate configuration files, and that an arena config builds a valid pack",
	Long: `Validates Arena configs, scenarios, providers, and other YAML files against their JSON schemas.

For an arena config it also runs the arena's consistency checks and then every
check packc runs: it builds the pack in memory and validates it as
'packc compile' and 'packc validate' would. A config that passes here compiles
with packc. Every problem is reported, not just the first; use --json for a
machine-readable report that names the file to edit for each.

Automatically detects file type based on the 'kind' field in the YAML file.
Can also explicitly specify the type with --type flag.

Examples:
  promptarena validate arena.yaml
  promptarena validate scenarios/test.yaml --type scenario
  promptarena validate providers/*.yaml
  promptarena validate arena.yaml --schema-only`,
	RunE: runValidate,
}

var (
	validateType       string
	validateVerbose    bool
	validateSchemaOnly bool
	validateJSONFlag   bool
)

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.Flags().StringVar(&validateType, "type", "auto", "Config type: auto, arena, scenario, provider, promptconfig, tool, persona")
	validateCmd.Flags().BoolVar(&validateVerbose, "verbose", false, "Show detailed validation errors")
	validateCmd.Flags().BoolVar(&validateSchemaOnly, "schema-only", false, "Only validate schema, skip business logic checks")
	validateCmd.Flags().BoolVar(&validateJSONFlag, "json", false, "Emit the validation result as JSON")
}

func runValidate(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("file path required")
	}

	filePath := args[0]
	if validateJSONFlag {
		return writeValidateJSON(cmd.OutOrStdout(), filePath, validateType, validateSchemaOnly)
	}

	report, err := collectValidation(filePath, validateType, validateSchemaOnly)
	if err != nil {
		return err
	}
	printValidateReport(report, validateVerbose)
	if !report.Valid {
		// The problems are printed above; the full list in the returned error
		// would print it twice.
		return fmt.Errorf("validation failed with %d error(s)", len(report.Errors))
	}

	fmt.Printf("\n✅ %s is valid\n", filepath.Base(filePath))
	return nil
}

func runValidationChecks(filePath, typeOption string, verbose, schemaOnly bool) error {
	report, err := collectValidation(filePath, typeOption, schemaOnly)
	if err != nil {
		return err
	}
	printValidateReport(report, verbose)
	return report.err()
}

// printValidateReport renders a report as text: schema errors with their
// suggestions, then every later finding with the file to edit.
func printValidateReport(r *validateJSONReport, verbose bool) {
	fmt.Printf("Validating %s as type '%s'...\n", filepath.Base(r.File), r.Type)
	if len(r.schemaErrors) > 0 {
		fmt.Printf("❌ Schema validation failed for %s:\n", r.File)
		displayErrors(r.schemaErrors, verbose)
		return
	}
	fmt.Printf("✅ Schema validation passed for %s\n", r.File)
	printStageFindings(r)
}

// printStageFindings renders everything after the schema stage.
func printStageFindings(r *validateJSONReport) {
	if !slices.Contains(r.Stages, stageLoad) {
		return
	}
	fmt.Println("\nRunning business logic validation...")
	if len(r.Errors) > 0 {
		fmt.Printf("\n❌ %d problem(s):\n", len(r.Errors))
		for _, f := range r.Errors {
			fmt.Printf("  - %s\n", f)
		}
	}
	if len(r.Warnings) > 0 {
		fmt.Printf("\n⚠️  Validation warnings (%d):\n", len(r.Warnings))
		for _, f := range r.Warnings {
			fmt.Printf("  - %s\n", f)
		}
	}
	for _, n := range r.Notes {
		fmt.Printf("\nℹ️  %s\n", n)
	}
	if len(r.Errors) == 0 && slices.Contains(r.Stages, stagePack) {
		fmt.Println("\n✅ Pack builds and validates (packc compile would succeed)")
	}
}

func prepareValidationWithType(filePath, typeOption string) ([]byte, string, error) {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, "", fmt.Errorf("file not found: %s", filePath)
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file: %w", err)
	}

	// Determine config type
	configType := typeOption
	if configType == "auto" {
		detectedType, err := config.DetectConfigType(data)
		if err != nil {
			return nil, "", fmt.Errorf("could not auto-detect config type: %w\nUse --type to specify explicitly", err)
		}
		configType = string(detectedType)
	}

	return data, configType, nil
}

func performSchemaValidationWithVerbose(data []byte, configType string, filePath string, verbose bool) error {
	fmt.Printf("Validating %s as type '%s'...\n", filepath.Base(filePath), configType)

	result, err := validateWithSchema(data, config.ConfigType(configType))
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	if !result.Valid {
		fmt.Printf("❌ Schema validation failed for %s:\n", filePath)
		displayErrors(result.Errors, verbose)
		return fmt.Errorf("schema validation failed with %d error(s)", len(result.Errors))
	}

	fmt.Printf("✅ Schema validation passed for %s\n", filePath)
	return nil
}

// performBusinessLogicValidation runs the checks that need the arena config
// loaded — its consistency checks, assertion types and the packc checks — and
// prints them. Schema validation is the caller's.
func performBusinessLogicValidation(filePath string) error {
	r := &validateJSONReport{File: filePath, Errors: []validateFinding{}, Warnings: []validateFinding{}}
	r.addArenaStages(filePath)
	r.Valid = len(r.Errors) == 0
	printStageFindings(r)
	return r.err()
}

func validateWithSchema(data []byte, configType config.ConfigType) (*config.SchemaValidationResult, error) {
	return config.ValidateWithSchema(data, configType)
}

func displayErrors(errors []config.SchemaValidationError, verbose bool) {
	maxErrors := 5
	if verbose {
		maxErrors = len(errors)
	}

	displayed := 0
	for i, e := range errors {
		if i >= maxErrors {
			break
		}
		displayError(e)
		displayed++
	}

	if !verbose && len(errors) > maxErrors {
		remaining := len(errors) - displayed
		fmt.Printf("\n  ... and %d more error(s) (use --verbose to see all)\n", remaining)
	}
}

// rootField is how a finding about the whole document names its field.
const rootField = "root"

// displayField trims the schema validator's "(root)." prefix from a field path.
func displayField(field string) string {
	field = strings.TrimPrefix(field, "(root).")
	if field == "(root)" {
		return rootField
	}
	return field
}

func displayError(err config.SchemaValidationError) {
	field := displayField(err.Field)

	switch {
	case err.Keyword == "additional_property_not_allowed" && len(err.ValidValues) > 0:
		offending := extractAdditionalPropertyName(err.Description)
		if offending == "" {
			displayDefault(field, &err)
			return
		}
		header := fmt.Sprintf("  - %s: unknown property '%s'", field, offending)
		if len(err.Suggestions) > 0 {
			header += fmt.Sprintf(". Did you mean '%s'?", err.Suggestions[0])
		}
		fmt.Println(header)
		fmt.Printf("      Valid keys: %s\n", strings.Join(err.ValidValues, ", "))

	case len(err.Suggestions) > 0:
		displayDefault(field, &err)
		fmt.Printf("      Did you mean: %s\n", strings.Join(err.Suggestions, ", "))

	default:
		displayDefault(field, &err)
	}
}

func displayDefault(field string, err *config.SchemaValidationError) {
	if err.Value != nil {
		fmt.Printf("  - %s: %s (value: %v)\n", field, err.Description, err.Value)
	} else {
		fmt.Printf("  - %s: %s\n", field, err.Description)
	}
}

// extractAdditionalPropertyName pulls X out of "Additional property X is not allowed".
func extractAdditionalPropertyName(desc string) string {
	const prefix = "Additional property "
	const suffix = " is not allowed"
	if !strings.HasPrefix(desc, prefix) || !strings.HasSuffix(desc, suffix) {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(desc, prefix), suffix)
}
