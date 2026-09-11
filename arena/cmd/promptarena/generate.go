package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
	"github.com/AltairaLabs/promptarena/arena/generate"
	"github.com/AltairaLabs/promptarena/arena/generate/sources"
)

var generateRegistry = generate.NewRegistry()

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate Arena scenarios from recorded sessions",
	Long: `Generate scenario YAML files from recorded sessions.

Every user turn in a session becomes a scenario turn, with its attachments.
Recorded evals become assertions when their expected range is known: a
recorded verdict, the pack's declared threshold (with --config), or an
--expect you pass. A measurement with no known range is reported, not
asserted. Each decision is printed.

  promptarena generate --from-recordings "out/*.json" --filter-passed=false --output scenarios/
  promptarena generate --from-recordings "out/*.json" --expect faithfulness>=0.8 --config config.arena.yaml
  promptarena generate --source omnia --config arena.yaml --filter-passed=false --output scenarios/prod

--source names an installed deploy adapter with the sessions capability; the
deploy: section of --config supplies the endpoint, workspace and token.`,
	RunE: runGenerate,
}

func init() {
	rootCmd.AddCommand(generateCmd)
	registerGenerateFlags(generateCmd.Flags())
}

// registerGenerateFlags is shared with the tests so they exercise the real
// flag set.
func registerGenerateFlags(f *pflag.FlagSet) {
	f.String("source", "", "Installed deploy adapter to pull sessions from (e.g., omnia); "+
		"needs its sessions capability and a deploy: section in --config") // NOSONAR
	f.String("workspace", "", "Override the deploy profile's workspace when using --source") // NOSONAR
	f.String("from-recordings", "", "Glob of local recordings: arena run output (out/*.json), "+
		"PromptKit session recordings (*.recording.json, *.jsonl) or transcripts") // NOSONAR
	f.String("filter-eval-type", "", "Filter sessions by assertion failure type") // NOSONAR
	f.Bool("filter-passed", false, "Filter by pass/fail status")                  // NOSONAR
	f.StringArray("expect", nil, "Expected range for a measured eval, e.g. faithfulness>=0.8; "+
		"selects sessions outside it and becomes the assertion's bound (repeatable)") // NOSONAR
	f.String("config", "", "Arena config whose pack supplies eval params and declared thresholds") // NOSONAR
	f.String("task-type", "", "task_type to set on generated scenarios (default: conversation)")   // NOSONAR
	f.String("pack", "", "Deprecated alias for --task-type")                                       // NOSONAR
	_ = f.MarkDeprecated("pack", "it only ever set task_type; use --task-type")
	f.String("output", ".", "Output directory for generated scenario files") // NOSONAR
	f.Bool("dedup", true, "Deduplicate sessions by failure pattern")         // NOSONAR
}

// resolveSource returns the session source the flags name and a closer that
// releases it (a no-op for sources with nothing to release).
//
// --from-recordings reads local files. --source <name> is first looked up in
// the in-process registry (a Go caller may have registered one), then resolved
// as an installed deploy adapter with the sessions capability, using the
// deploy: section of --config for endpoint, workspace and token.
func resolveSource(ctx context.Context, cmd *cobra.Command) (generate.SessionSourceAdapter, func() error, error) {
	noop := func() error { return nil }
	sourceName, _ := cmd.Flags().GetString("source")
	fromRecordings, _ := cmd.Flags().GetString("from-recordings")

	switch {
	case fromRecordings != "":
		return sources.NewRecordingsAdapter(fromRecordings), noop, nil
	case sourceName != "":
		if adapter, err := generateRegistry.Get(sourceName); err == nil {
			return adapter, noop, nil
		}
		configPath, _ := cmd.Flags().GetString("config")
		workspace, _ := cmd.Flags().GetString("workspace")
		src, err := flow.OpenSessionSource(ctx, sourceName, flow.Options{ConfigPath: configPath}, workspace)
		if err != nil {
			return nil, nil, err
		}
		return src, src.Close, nil
	default:
		return nil, nil, fmt.Errorf("specify either --source or --from-recordings")
	}
}

// buildRequest turns the flags into a generate.Request for the resolved source.
func buildRequest(cmd *cobra.Command, source generate.SessionSourceAdapter) (generate.Request, error) {
	req := generate.Request{Source: source}

	if cmd.Flags().Changed("filter-passed") {
		v, _ := cmd.Flags().GetBool("filter-passed")
		req.List.FilterPassed = &v
	}
	req.List.FilterEvalType, _ = cmd.Flags().GetString("filter-eval-type")

	raw, _ := cmd.Flags().GetStringArray("expect")
	for _, s := range raw {
		e, pErr := generate.ParseExpectation(s)
		if pErr != nil {
			return generate.Request{}, pErr
		}
		req.List.Expectations = append(req.List.Expectations, e)
	}

	req.Convert.TaskType, _ = cmd.Flags().GetString("task-type")
	if req.Convert.TaskType == "" {
		req.Convert.TaskType, _ = cmd.Flags().GetString("pack") // deprecated alias
	}
	req.Dedup, _ = cmd.Flags().GetBool("dedup")

	if configPath, _ := cmd.Flags().GetString("config"); configPath != "" {
		cfg, cfgErr := arenaconfig.LoadConfig(configPath)
		if cfgErr != nil {
			return generate.Request{}, fmt.Errorf("loading %s: %w", configPath, cfgErr)
		}
		if cfg.LoadedPack != nil {
			req.Pack = &cfg.LoadedPack.Pack
		}
	}
	return req, nil
}
