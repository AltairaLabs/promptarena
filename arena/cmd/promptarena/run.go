package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	arenaaudio "github.com/AltairaLabs/promptarena/arena/audio"
	"github.com/AltairaLabs/promptarena/arena/engine"
	"github.com/AltairaLabs/promptarena/arena/results"
	jsonrepo "github.com/AltairaLabs/promptarena/arena/results/json"
	"github.com/AltairaLabs/promptarena/arena/results/junit"
	"github.com/AltairaLabs/promptarena/arena/results/markdown"
	"github.com/AltairaLabs/promptarena/arena/statestore"
)

// Flag name constants to avoid duplication
const (
	flagMaxTokens = "max-tokens"

	formatMarkdown = "markdown"
	// formatHTML was a bespoke standalone report renderer. The web UI
	// supersedes it and markdown carries the same content for CI, so the
	// format is still accepted but normalised to markdown — see
	// normalizeOutputFormats.
	formatHTML = "html"
)

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// createResultRepository creates a composite repository based on output formats
func createResultRepository(params *RunParameters, configFile string) (results.ResultRepository, error) {
	composite := results.NewCompositeRepository()

	for _, format := range params.OutputFormats {
		switch format {
		case "json":
			jsonRepo := jsonrepo.NewJSONResultRepository(params.OutDir)
			composite.AddRepository(jsonRepo)
		case "junit":
			junitRepo := junit.NewJUnitResultRepository(params.JUnitFile)
			composite.AddRepository(junitRepo)
		// formatHTML is normalised away in processHTMLSettings; accept it here
		// too so any caller building params directly still gets a report.
		case formatMarkdown, formatHTML:
			// Get markdown configuration from arena defaults
			var markdownConfig *markdown.MarkdownConfig
			if configFile != "" {
				// Load config to get markdown defaults
				cfg, err := arenaconfig.LoadConfig(configFile)
				if err == nil && cfg != nil {
					markdownConfig = markdown.CreateMarkdownConfigFromDefaults(&cfg.Defaults)
				}
			}
			if markdownConfig == nil {
				markdownConfig = markdown.CreateMarkdownConfigFromDefaults(nil)
			}

			// Create markdown repository with configuration and custom output file
			markdownRepo := markdown.NewMarkdownResultRepositoryWithConfig(filepath.Dir(params.MarkdownFile), markdownConfig)
			// Override the output file path
			markdownRepo.SetOutputFile(params.MarkdownFile)
			composite.AddRepository(markdownRepo)
		default:
			return nil, fmt.Errorf("unsupported output format: %s", format)
		}
	}

	return composite, nil
}

// createResultSummary creates a summary using the results builder
func createResultSummary(runResults []engine.RunResult, successCount, errorCount int, configFile string) *results.ResultSummary {
	builder := results.NewSummaryBuilder(configFile)
	return builder.BuildSummary(runResults)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run conversation simulations",
	Long: `Run multi-turn conversation simulations across multiple LLM providers, 
voice profiles, and system prompts.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSimulations(cmd)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	// NOSONAR
	// Configuration file
	runCmd.Flags().StringP("config", "c", "config.arena.yaml", "Configuration file path") // NOSONAR

	// Override flags
	runCmd.Flags().StringSlice("region", []string{}, "Regions to run (e.g., us,uk,au)")
	runCmd.Flags().StringSlice("provider", []string{}, "Providers to use")
	runCmd.Flags().StringSlice("scenario", []string{}, "Scenarios to run")
	runCmd.Flags().StringSlice("eval", []string{}, "Evaluations to run")

	// Execution settings
	runCmd.Flags().IntP("concurrency", "j", 6, "Number of concurrent workers")
	runCmd.Flags().StringP("out", "o", "out", "Output directory")
	runCmd.Flags().Bool("ci", false, "CI mode (disable TUI, simple logs)")
	runCmd.Flags().Bool("simple", false, "Simple mode (alias for --ci)")
	runCmd.Flags().Bool("html", false, "Deprecated: the HTML report was removed; produces markdown instead")
	runCmd.Flags().StringSlice("format", []string{}, "Output formats (json, junit, markdown) - defaults from config")
	runCmd.Flags().StringSlice("formats", []string{}, "Output formats (json, junit, markdown) - alias for --format")
	runCmd.Flags().String("junit-file", "", "JUnit XML output file (default: out/junit.xml)")
	runCmd.Flags().String("html-file", "", "Deprecated: the HTML report was removed; use --markdown-file")
	// Keep both accepted so existing scripts do not fail outright, but say so.
	_ = runCmd.Flags().MarkDeprecated("html", "the HTML report was removed; markdown is produced instead")
	_ = runCmd.Flags().MarkDeprecated("html-file", "the HTML report was removed; use --markdown-file")
	runCmd.Flags().String("markdown-file", "", "Markdown report output file (default: out/results.md)")
	runCmd.Flags().Float32("temperature", 0.6, "Override temperature")
	runCmd.Flags().Int(flagMaxTokens, 0, "Override max tokens for all scenarios")
	runCmd.Flags().IntP("seed", "s", 42, "Random seed")
	runCmd.Flags().BoolP("verbose", "v", false, "Enable verbose debug logging for API calls")

	// Mock provider settings
	runCmd.Flags().Bool("mock-provider", false, "Replace all providers with MockProvider (for CI/testing)")
	runCmd.Flags().String("mock-config", "", "Path to mock provider configuration file (YAML)")

	// Provider substitution: swap one configured provider's spec for another (repeatable)
	runCmd.Flags().StringArray("override-provider", nil,
		"Substitute a configured provider with another: from=to (repeatable)")

	// Pack eval settings
	runCmd.Flags().Bool("skip-pack-evals", false, "Disable pack eval execution")
	runCmd.Flags().StringSlice("eval-types", []string{}, "Filter to specific eval types (e.g., contains,regex)")

	// Self-play settings
	runCmd.Flags().Bool("selfplay", false, "Enable self-play mode")
	runCmd.Flags().StringSlice("roles", []string{}, "Self-play role configurations to use")

	// Audio monitoring settings (real-time duplex audio)
	runCmd.Flags().String("audio-monitor", string(arenaaudio.ModeAuto), "Audio monitoring: auto, on, off")
	runCmd.Flags().Int("audio-rate", arenaaudio.Rate24k, "Audio canonical sample rate: 16000, 24000, or 48000")

	// Bind flags to viper
	_ = viper.BindPFlag("concurrency", runCmd.Flags().Lookup("concurrency"))
	_ = viper.BindPFlag("out_dir", runCmd.Flags().Lookup("out"))
	_ = viper.BindPFlag("ci_mode", runCmd.Flags().Lookup("ci"))
	_ = viper.BindPFlag("html_report", runCmd.Flags().Lookup("html"))
	_ = viper.BindPFlag("temperature", runCmd.Flags().Lookup("temperature"))
	_ = viper.BindPFlag(flagMaxTokens, runCmd.Flags().Lookup(flagMaxTokens))
	_ = viper.BindPFlag("seed", runCmd.Flags().Lookup("seed"))
	_ = viper.BindPFlag("selfplay", runCmd.Flags().Lookup("selfplay"))
	_ = viper.BindPFlag("roles", runCmd.Flags().Lookup("roles"))

	// Register dynamic completions (must be after flags are defined)
	RegisterRunCompletions()
}

// NOTE: runSimulations, executeRuns, executeWithTUI, and executeSimple are defined in run_interactive.go
// These functions are excluded from coverage testing as they involve interactive terminal operations.

// RunParameters holds all the parameters for running simulations
type RunParameters struct {
	Regions       []string
	Providers     []string
	Scenarios     []string
	Evals         []string // Evaluation configurations to run
	Concurrency   int
	OutDir        string
	CIMode        bool
	SimpleMode    bool // Alias for CIMode
	Verbose       bool
	OutputFormats []string // New: output formats (json, junit, markdown)
	JUnitFile     string   // New: JUnit XML output file
	MarkdownFile  string   // New: Markdown output file
	MockProvider  bool     // Enable mock provider mode
	MockConfig    string   // Path to mock provider configuration
	// ProviderOverrides holds raw "from=to" pairs from --override-provider,
	// substituting one configured provider's spec for another at invocation time.
	ProviderOverrides []string
	SkipPackEvals     bool     // Disable pack eval execution
	EvalTypes         []string // Filter to specific eval types
	ConfigFile        string   // Configuration file name for TUI display
	TotalRuns         int      // Total number of runs for TUI progress

	// Audio monitor settings (real-time duplex audio)
	AudioMonitorMode string // "auto" | "on" | "off"
	AudioMonitorRate int    // 16000, 24000, or 48000
}

// loadConfiguration loads the configuration file and sets up viper
// extractRunParameters extracts all run parameters from command flags
func extractRunParameters(cmd *cobra.Command, cfg *arenaconfig.Config) (*RunParameters, error) {
	params := &RunParameters{}

	// Extract basic flags
	if err := extractBasicFlags(cmd, params); err != nil {
		return nil, err
	}

	// Extract mock provider flags
	if err := extractMockFlags(cmd, params); err != nil {
		return nil, err
	}

	// Extract provider override flags
	if err := extractOverrideFlags(cmd, params); err != nil {
		return nil, err
	}

	// Extract audio monitor flags
	if err := extractAudioMonitorFlags(cmd, params); err != nil {
		return nil, err
	}

	// Extract output format flags
	if err := extractOutputFormatFlags(cmd, cfg, params); err != nil {
		return nil, err
	}

	// Process HTML report settings (maintain backward compatibility)
	if err := processHTMLSettings(cmd, cfg, params); err != nil {
		return nil, err
	}

	// Apply configuration overrides
	applyConfigurationOverrides(cmd, cfg, params)

	return params, nil
}

// extractBasicFlags extracts basic command flags
func extractBasicFlags(cmd *cobra.Command, params *RunParameters) error {
	var err error
	if params.Regions, err = cmd.Flags().GetStringSlice("region"); err != nil {
		return fmt.Errorf("failed to get region flag: %w", err)
	}
	if params.Providers, err = cmd.Flags().GetStringSlice("provider"); err != nil {
		return fmt.Errorf("failed to get provider flag: %w", err)
	}
	if params.Scenarios, err = cmd.Flags().GetStringSlice("scenario"); err != nil {
		return fmt.Errorf("failed to get scenario flag: %w", err)
	}
	if params.Evals, err = cmd.Flags().GetStringSlice("eval"); err != nil {
		return fmt.Errorf("failed to get eval flag: %w", err)
	}
	if params.Concurrency, err = cmd.Flags().GetInt("concurrency"); err != nil {
		return fmt.Errorf("failed to get concurrency flag: %w", err)
	}
	if params.OutDir, err = cmd.Flags().GetString("out"); err != nil {
		return fmt.Errorf("failed to get out flag: %w", err)
	}
	if params.CIMode, err = cmd.Flags().GetBool("ci"); err != nil {
		return fmt.Errorf("failed to get ci flag: %w", err)
	}
	if params.SimpleMode, err = cmd.Flags().GetBool("simple"); err != nil {
		return fmt.Errorf("failed to get simple flag: %w", err)
	}
	// If either --ci or --simple is set, disable TUI
	if params.SimpleMode {
		params.CIMode = true
	}
	if params.Verbose, err = cmd.Flags().GetBool("verbose"); err != nil {
		return fmt.Errorf("failed to get verbose flag: %w", err)
	}
	if params.SkipPackEvals, err = cmd.Flags().GetBool("skip-pack-evals"); err != nil {
		return fmt.Errorf("failed to get skip-pack-evals flag: %w", err)
	}
	if params.EvalTypes, err = cmd.Flags().GetStringSlice("eval-types"); err != nil {
		return fmt.Errorf("failed to get eval-types flag: %w", err)
	}
	return nil
}

// extractAudioMonitorFlags extracts the audio monitoring flags.
func extractAudioMonitorFlags(cmd *cobra.Command, params *RunParameters) error {
	mode, err := cmd.Flags().GetString("audio-monitor")
	if err != nil {
		return fmt.Errorf("failed to get audio-monitor flag: %w", err)
	}
	rate, err := cmd.Flags().GetInt("audio-rate")
	if err != nil {
		return fmt.Errorf("failed to get audio-rate flag: %w", err)
	}
	params.AudioMonitorMode = mode
	params.AudioMonitorRate = rate
	return nil
}

// extractMockFlags extracts mock provider configuration flags
func extractMockFlags(cmd *cobra.Command, params *RunParameters) error {
	var err error
	if params.MockProvider, err = cmd.Flags().GetBool("mock-provider"); err != nil {
		return fmt.Errorf("failed to get mock-provider flag: %w", err)
	}
	if params.MockConfig, err = cmd.Flags().GetString("mock-config"); err != nil {
		return fmt.Errorf("failed to get mock-config flag: %w", err)
	}
	return nil
}

// extractOverrideFlags extracts the repeatable --override-provider pairs.
// Pairs are validated against the loaded providers later, in applyProviderOverrides.
func extractOverrideFlags(cmd *cobra.Command, params *RunParameters) error {
	overrides, err := cmd.Flags().GetStringArray("override-provider")
	if err != nil {
		return fmt.Errorf("failed to get override-provider flag: %w", err)
	}
	params.ProviderOverrides = overrides
	return nil
}

// extractOutputFormatFlags extracts output format flags and applies config defaults
func extractOutputFormatFlags(cmd *cobra.Command, cfg *arenaconfig.Config, params *RunParameters) error {
	var err error

	// Check if --formats (plural) flag was used
	if cmd.Flags().Changed("formats") {
		if params.OutputFormats, err = cmd.Flags().GetStringSlice("formats"); err != nil {
			return fmt.Errorf("failed to get formats flag: %w", err)
		}
	} else {
		// Use --format (singular) flag
		if params.OutputFormats, err = cmd.Flags().GetStringSlice("format"); err != nil {
			return fmt.Errorf("failed to get format flag: %w", err)
		}
		// If format flag wasn't changed, use config defaults, otherwise fallback to json
		if !cmd.Flags().Changed("format") {
			outputConfig := cfg.Defaults.GetOutputConfig()
			if len(outputConfig.Formats) > 0 {
				params.OutputFormats = outputConfig.Formats
			} else {
				params.OutputFormats = []string{"json"} // Default fallback
			}
		}
	}

	if params.JUnitFile, err = cmd.Flags().GetString("junit-file"); err != nil {
		return fmt.Errorf("failed to get junit-file flag: %w", err)
	}
	if params.MarkdownFile, err = cmd.Flags().GetString("markdown-file"); err != nil {
		return fmt.Errorf("failed to get markdown-file flag: %w", err)
	}
	return nil
}

// processHTMLSettings folds the retired HTML report settings into the live
// formats and resolves the default output file paths.
func processHTMLSettings(cmd *cobra.Command, cfg *arenaconfig.Config, params *RunParameters) error {
	// Handle HTML flag and config settings
	if err := processHTMLFlags(cmd, cfg, params); err != nil {
		return err
	}

	params.OutputFormats = normalizeOutputFormats(params.OutputFormats)

	// Set default output file paths
	setDefaultFilePaths(cfg, params)

	return nil
}

// processHTMLFlags maps the retired HTML report onto markdown. The --html flag
// and the html_report config key both used to request a standalone HTML report;
// they now request markdown instead, so existing configs and CI scripts keep
// producing a human-readable report rather than erroring or silently emitting
// nothing.
func processHTMLFlags(cmd *cobra.Command, cfg *arenaconfig.Config, params *RunParameters) error {
	if cmd.Flags().Changed("html") {
		wantHTML, err := cmd.Flags().GetBool("html")
		if err != nil {
			return fmt.Errorf("failed to get html flag: %w", err)
		}
		if wantHTML {
			params.OutputFormats = append(params.OutputFormats, formatMarkdown)
		}
		return nil
	}
	if cfg.Defaults.HTMLReport != "" {
		params.OutputFormats = append(params.OutputFormats, formatMarkdown)
	}
	return nil
}

// normalizeOutputFormats rewrites the retired "html" format to "markdown" and
// drops any duplicates, so downstream code only ever sees live formats.
func normalizeOutputFormats(formats []string) []string {
	seen := make(map[string]bool, len(formats))
	out := make([]string, 0, len(formats))
	for _, f := range formats {
		if f == formatHTML {
			f = formatMarkdown
		}
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// setDefaultFilePaths sets default file paths for output files if not specified
func setDefaultFilePaths(cfg *arenaconfig.Config, params *RunParameters) {
	// Set default JUnit file path
	if params.JUnitFile == "" {
		params.JUnitFile = filepath.Join(params.OutDir, "junit.xml")
	}

	// Set default Markdown file path if markdown generation is enabled
	if params.MarkdownFile == "" && contains(params.OutputFormats, formatMarkdown) {
		if cfg.Defaults.Output.Markdown != nil && cfg.Defaults.Output.Markdown.File != "" {
			params.MarkdownFile = config.ResolveOutputPath(params.OutDir, cfg.Defaults.Output.Markdown.File)
		} else {
			params.MarkdownFile = filepath.Join(params.OutDir, "results.md")
		}
	}
}

// applyConfigurationOverrides applies command line overrides to configuration
func applyConfigurationOverrides(cmd *cobra.Command, cfg *arenaconfig.Config, params *RunParameters) {
	// Apply verbose override to config
	if params.Verbose {
		cfg.Defaults.Verbose = true
	}

	// Apply max_tokens override if provided
	if cmd.Flags().Changed(flagMaxTokens) {
		maxTokens, _ := cmd.Flags().GetInt(flagMaxTokens)
		cfg.Defaults.MaxTokens = maxTokens
	}
}

// displayRunInfo displays run information when not in CI mode
// convertRunResults retrieves and converts run results from the statestore
// convertToEngineRunResult converts statestore.RunResult to engine.RunResult
func convertToEngineRunResult(sr *statestore.RunResult) engine.RunResult {
	result := engine.RunResult{
		RunID:        sr.RunID,
		PromptPack:   sr.PromptPack,
		Region:       sr.Region,
		ScenarioID:   sr.ScenarioID,
		ProviderID:   sr.ProviderID,
		Labels:       sr.Labels,
		Params:       sr.Params,
		Messages:     sr.Messages,
		Commit:       sr.Commit,
		Cost:         sr.Cost,
		ToolStats:    sr.ToolStats,
		Violations:   sr.Violations,
		StartTime:    sr.StartTime,
		EndTime:      sr.EndTime,
		Duration:     sr.Duration,
		Error:        sr.Error,
		SelfPlay:     sr.SelfPlay,
		PersonaID:    sr.PersonaID,
		UserFeedback: sr.UserFeedback,
		SessionTags:  sr.SessionTags,
		ConversationAssertions: engine.AssertionsSummary{
			Failed:  countFailed(sr.ConversationAssertions.Results),
			Passed:  sr.ConversationAssertions.Passed,
			Results: sr.ConversationAssertions.Results,
			Total:   sr.ConversationAssertions.Total,
		},
		EvalResults: sr.EvalResults,
	}

	// Convert A2AAgents
	result.A2AAgents = convertA2AAgentsToEngine(sr.A2AAgents)

	// Convert AssistantRole from interface{} to *engine.SelfPlayRoleInfo
	if sr.AssistantRole != nil {
		if roleMap, ok := sr.AssistantRole.(map[string]interface{}); ok {
			result.AssistantRole = &engine.SelfPlayRoleInfo{
				Provider: getStringFromMap(roleMap, "Provider"),
				Model:    getStringFromMap(roleMap, "Model"),
				Region:   getStringFromMap(roleMap, "Region"),
			}
		}
	}

	// Convert UserRole from interface{} to *engine.SelfPlayRoleInfo
	if sr.UserRole != nil {
		if roleMap, ok := sr.UserRole.(map[string]interface{}); ok {
			result.UserRole = &engine.SelfPlayRoleInfo{
				Provider: getStringFromMap(roleMap, "Provider"),
				Model:    getStringFromMap(roleMap, "Model"),
				Region:   getStringFromMap(roleMap, "Region"),
			}
		}
	}

	return result
}

// convertA2AAgentsToEngine converts statestore A2A agent info to engine types.
func convertA2AAgentsToEngine(agents []statestore.A2AAgentInfo) []engine.A2AAgentInfo {
	if len(agents) == 0 {
		return nil
	}
	result := make([]engine.A2AAgentInfo, len(agents))
	for i, agent := range agents {
		info := engine.A2AAgentInfo{
			Name:        agent.Name,
			Description: agent.Description,
		}
		if len(agent.Skills) > 0 {
			info.Skills = make([]engine.A2ASkillInfo, len(agent.Skills))
			for j, skill := range agent.Skills {
				info.Skills[j] = engine.A2ASkillInfo{
					ID:          skill.ID,
					Name:        skill.Name,
					Description: skill.Description,
					Tags:        skill.Tags,
				}
			}
		}
		result[i] = info
	}
	return result
}

// countFailed returns the number of failed conversation assertions
func countFailed(convResults []statestore.ConversationValidationResult) int {
	failed := 0
	for i := range convResults {
		if !convResults[i].Passed {
			failed++
		}
	}
	return failed
}

// getStringFromMap safely extracts a string value from a map
func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// countResultsByStatus counts successful and error results
func countResultsByStatus(results []engine.RunResult) (successCount, errorCount int) {
	for i := range results {
		if results[i].Error != "" {
			errorCount++
		} else {
			successCount++
		}
	}

	return successCount, errorCount
}

// countFailedAssertions counts the total number of failed assertions across all results.
// Also counts runs where Passed is false even if Failed is 0, which indicates assertions
// were defined but never executed (a configuration or wiring error).
func countFailedAssertions(results []engine.RunResult) int {
	failedCount := 0
	for i := range results {
		failedCount += results[i].ConversationAssertions.Failed
		if !results[i].ConversationAssertions.Passed && results[i].ConversationAssertions.Failed == 0 {
			// Assertions were defined but never executed — count as 1 failure
			failedCount++
		}
	}
	return failedCount
}
