package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/calibrate"
	"github.com/AltairaLabs/promptarena/v2/arena/engine"
)

var calibrateFlags struct {
	config   string
	check    string
	examples string
	out      string
	minKappa float64
	minAgree float64
}

var calibrateCmd = &cobra.Command{
	Use:   "calibrate",
	Short: "Score a check, such as an LLM judge, against examples a person has labeled",
	Long: `Runs one check, written as in a scenario's conversation_assertions, over
examples a person has labeled, and reports how far its verdicts agree with the
labels: agreement, how often it passes pass-labeled and fails fail-labeled
examples, and Cohen's kappa, with 95% intervals. Every disagreement is listed.

The check uses the arena config's judges and providers, as it would in a run.

The examples file is JSONL, one object per line:
  {"id": "...", "input": "user message", "content": "reply", "label": true}
label is true when the check should pass. "messages" can replace input and
content with a whole conversation.

Examples:
  promptarena calibrate --check checks/refusal-judge.yaml --examples data/refusals.jsonl
  promptarena calibrate --check judge.yaml --examples labels.jsonl --min-kappa 0.6`,
	RunE: runCalibrate,
}

func init() {
	rootCmd.AddCommand(calibrateCmd)
	f := calibrateCmd.Flags()
	f.StringVarP(&calibrateFlags.config, "config", "c", "config.arena.yaml",
		"Arena config file, or a directory holding config.arena.yaml, whose judges and providers the check uses")
	f.StringVar(&calibrateFlags.check, "check", "", "YAML file holding the check: type, params, message (required)")
	f.StringVar(&calibrateFlags.examples, "examples", "", "JSONL file of labeled examples (required)")
	f.StringVarP(&calibrateFlags.out, "out", "o", "out", "Directory for calibration.json and calibration.md")
	f.Float64Var(&calibrateFlags.minKappa, "min-kappa", 0, "Exit non-zero when Cohen's kappa is below this")
	f.Float64Var(&calibrateFlags.minAgree, "min-agreement", 0, "Exit non-zero when agreement (0-1) is below this")
	_ = calibrateCmd.MarkFlagRequired("check")
	_ = calibrateCmd.MarkFlagRequired("examples")
}

func runCalibrate(cmd *cobra.Command, _ []string) error {
	check, err := loadCheck(calibrateFlags.check)
	if err != nil {
		return err
	}
	if typeErr := asrt.ValidateCheckType(calibrateFlags.check, check.Type, evals.NewEvalTypeRegistry()); typeErr != nil {
		return typeErr
	}
	examples, err := calibrate.LoadExamples(calibrateFlags.examples)
	if err != nil {
		return err
	}
	configFile := calibrateFlags.config
	if info, statErr := os.Stat(configFile); statErr == nil && info.IsDir() {
		configFile = filepath.Join(configFile, "config.arena.yaml")
	}
	cfg, err := arenaconfig.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	eng, err := engine.NewEngineFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to build engine: %w", err)
	}
	defer func() { _ = eng.Close() }()

	// Ctrl-C stops scoring; the examples already scored are still reported.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	report := calibrate.Run(ctx, check, examples, eng.RunCheck)
	md := calibrate.RenderMarkdown(report)
	if err := writeCalibration(calibrateFlags.out, report, md); err != nil {
		return err
	}
	if _, err := io.WriteString(cmd.OutOrStdout(), md); err != nil {
		return err
	}
	return calibrationGate(report, cmd.Flags().Changed("min-kappa"), calibrateFlags.minKappa,
		cmd.Flags().Changed("min-agreement"), calibrateFlags.minAgree)
}

// loadCheck reads the check under calibration from a YAML file.
func loadCheck(path string) (asrt.AssertionConfig, error) {
	var check asrt.AssertionConfig
	data, err := os.ReadFile(path) //nolint:gosec // operator-provided file path
	if err != nil {
		return check, fmt.Errorf("read check: %w", err)
	}
	if err := yaml.Unmarshal(data, &check); err != nil {
		return check, fmt.Errorf("parse check %s: %w", path, err)
	}
	if check.Type == "" {
		return check, fmt.Errorf("check %s needs a type", path)
	}
	return check, nil
}

func writeCalibration(dir string, report *calibrate.Report, md string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:mnd // directory permission
		return fmt.Errorf("create output dir: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "calibration.json"), data, 0o600); err != nil { //nolint:mnd // perm
		return fmt.Errorf("write calibration.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calibration.md"), []byte(md), 0o600); err != nil { //nolint:mnd // perm
		return fmt.Errorf("write calibration.md: %w", err)
	}
	return nil
}

// calibrationGate fails when a requested minimum isn't met, or when nothing
// could be evaluated at all.
func calibrationGate(r *calibrate.Report, kappaSet bool, minKappa float64, agreeSet bool, minAgree float64) error {
	if r.Evaluated == 0 {
		return fmt.Errorf("no example could be evaluated (%d not evaluated)", r.Errored)
	}
	if kappaSet && r.Kappa < minKappa {
		return fmt.Errorf("kappa %.2f is below --min-kappa %.2f", r.Kappa, minKappa)
	}
	if agreeSet && r.Agreement < minAgree {
		return fmt.Errorf("agreement %.3f is below --min-agreement %.3f", r.Agreement, minAgree)
	}
	return nil
}
