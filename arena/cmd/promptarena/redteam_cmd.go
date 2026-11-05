package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/promptarena/v2/arena/redteam"
)

var redteamCmd = &cobra.Command{
	Use:   "redteam",
	Short: "Generate red-team scenarios from objective sets",
	Long: `Red-team tooling. generate crosses a set of objectives with attack
strategies and converters, and writes the result into your project as
ordinary scenario YAML, so you can review it, commit it and keep any attack
that gets through as a regression test.

Examples:
  promptarena redteam datasets
  promptarena redteam generate --objectives objectives.jsonl --strategies scripted,crescendo --judge policy-judge
  promptarena redteam generate --dataset jbb --limit 20 --converters none,base64,best_of_n`,
}

// generateUse is the name of the generate subcommands.
const generateUse = "generate"

// redteamGenerateFlags are the generate command's flag values.
type redteamGenerateFlags struct {
	objectives  string
	dataset     string
	datasetFile string
	cacheDir    string
	refresh     bool
	limit       int
	opts        redteam.Options
}

func newRedteamGenerateCmd() *cobra.Command {
	f := &redteamGenerateFlags{}
	cmd := &cobra.Command{
		Use:   generateUse,
		Short: "Write red-team scenarios for a set of objectives",
		Long: `Write red-team scenarios for a set of objectives into a project.

The objectives come from a local --objectives file (.jsonl or .csv), or from
a public --dataset fetched on demand into a cache directory; Arena ships none
of their text. Each strategy gets one scenario per converter, and every
scenario runs once per objective from a data file, so the scenarios stay few
and readable however many objectives there are.

Writes:
  data/<name>.jsonl                   the objectives
  data/<name>-benign.jsonl            benign look-alikes, when the source has them
  scenarios/<name>-<strategy>[-<converter>].scenario.yaml
  scenarios/<name>-baseline.scenario.yaml       no-attack baseline (--baseline)
  scenarios/<name>-over-refusal.scenario.yaml   benign requests the agent should answer
  personas/<persona>.persona.yaml     built-in personas the scenarios use, unless present

Existing scenario and data files are left alone unless --force is given;
existing persona files are always kept. The arena config entries to add are
printed at the end.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRedteamGenerate(cmd, f)
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.objectives, "objectives", "", "Objectives file (.jsonl or .csv): an objective column, "+
		"and optionally id, prompt, category, taxonomy, success_pattern and benign")
	fl.StringVar(&f.dataset, "dataset", "", "Public objective set to fetch (see `promptarena redteam datasets`)")
	fl.StringVar(&f.datasetFile, "dataset-file", "", "Local copy of the --dataset's behaviors file, read instead of "+
		"fetching (no benign split)")
	fl.StringVar(&f.cacheDir, "cache-dir", "", "Where fetched datasets are cached (default: the user cache "+
		"directory's promptarena/datasets)")
	fl.BoolVar(&f.refresh, "refresh", false, "Fetch the dataset again even when it is cached")
	fl.IntVar(&f.limit, "limit", 0, "Use only the first N objectives (and the first N benign requests)")
	fl.StringVar(&f.opts.Name, "name", "", "Prefix for scenario IDs and file names (default: the dataset or file name)")
	fl.StringVar(&f.opts.Dir, "dir", ".", "Project directory to write into")
	fl.StringSliceVar(&f.opts.Strategies, "strategies", []string{redteam.StrategyScripted},
		"Strategies: scripted, or a built-in adversarial persona (crescendo, refine, multi-technique, ...)")
	fl.StringSliceVar(&f.opts.Converters, "converters", []string{redteam.ConverterNone},
		"Converters, one scenario each: none, base64, rot13, leetspeak, reverse, homoglyph, unicode_tags, "+
			"zero_width, best_of_n")
	fl.StringVar(&f.opts.Judge, "judge", "", "Configured judge that decides whether an attack got through")
	fl.StringVar(&f.opts.TaskType, "task-type", redteam.DefaultTaskType, "task_type of the generated scenarios")
	fl.StringVar(&f.opts.Role, "role", redteam.DefaultRole, "Self-play role the personas speak as")
	fl.IntVar(&f.opts.Turns, "turns", redteam.DefaultTurns, "Exchanges a persona strategy gets")
	fl.IntVar(&f.opts.Trials, "trials", 1, "Trials per objective; above 1, persona scenarios stop at the "+
		"first that gets through")
	fl.Float64Var(&f.opts.MaxCostUSD, "max-cost-usd", 0, "Cost cap for one objective's trials (with --trials)")
	fl.BoolVar(&f.opts.Baseline, "baseline", true, "Add a no-attack baseline scenario")
	fl.StringSliceVar(&f.opts.Taxonomy, "taxonomy", nil, "Taxonomy entries for every objective, "+
		"e.g. owasp-llm:LLM01,atlas:AML.T0054")
	fl.BoolVar(&f.opts.Force, "force", false, "Replace existing scenario and data files")
	return cmd
}

var redteamDatasetsJSON bool

var redteamDatasetsCmd = &cobra.Command{
	Use:   "datasets",
	Short: "List the public objective sets generate can fetch",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return writeRedteamDatasets(cmd.OutOrStdout(), redteamDatasetsJSON)
	},
}

func init() {
	rootCmd.AddCommand(redteamCmd)
	redteamCmd.AddCommand(newRedteamGenerateCmd(), redteamDatasetsCmd)
	redteamDatasetsCmd.Flags().BoolVar(&redteamDatasetsJSON, "json", false, "Output JSON")
}

func writeRedteamDatasets(w io.Writer, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(redteam.Datasets)
	}
	tw := tabwriter.NewWriter(w, tabMinWidth, tabWidth, tabPadding, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tLICENSE\tTITLE\tTAKES"); err != nil {
		return err
	}
	for _, d := range redteam.Datasets {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", d.Name, d.License, d.Title, d.Note); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func runRedteamGenerate(cmd *cobra.Command, f *redteamGenerateFlags) error {
	w := cmd.OutOrStdout()
	objs, src, name, err := loadRedteamObjectives(cmd, f)
	if err != nil {
		return err
	}
	objs.Harmful = limitObjectives(objs.Harmful, f.limit)
	objs.Benign = limitObjectives(objs.Benign, f.limit)
	opts := f.opts
	opts.Source = src
	if opts.Name == "" {
		opts.Name = name
	}
	res, err := redteam.Generate(opts, objs)
	if err != nil {
		return err
	}
	return printRedteamResult(w, opts.Dir, len(objs.Harmful), len(objs.Benign), res)
}

// loadRedteamObjectives reads the objectives the flags name, and returns
// them with their source and a default name.
func loadRedteamObjectives(cmd *cobra.Command, f *redteamGenerateFlags) (redteam.Objectives, redteam.Source,
	string, error) {
	switch {
	case f.objectives != "" && f.dataset != "":
		return redteam.Objectives{}, redteam.Source{}, "", errors.New("give --objectives or --dataset, not both")
	case f.objectives != "":
		objs, err := redteam.ReadObjectivesFile(f.objectives)
		base := filepath.Base(f.objectives)
		src := redteam.Source{Title: base, URLs: []string{base}}
		return objs, src, redteam.Slugify(strings.TrimSuffix(base, filepath.Ext(base))), err
	case f.dataset != "":
		d, err := redteam.LookupDataset(f.dataset)
		if err != nil {
			return redteam.Objectives{}, redteam.Source{}, "", err
		}
		cacheDir := f.cacheDir
		if cacheDir == "" {
			if cacheDir, err = redteam.DefaultCacheDir(); err != nil {
				return redteam.Objectives{}, redteam.Source{}, "", err
			}
		}
		fetcher := redteam.Fetcher{CacheDir: cacheDir, Refresh: f.refresh}
		objs, skipped, src, err := redteam.LoadDataset(cmd.Context(), d, fetcher, f.datasetFile)
		if err == nil && skipped > 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skipped %d of the %s rows: %s.\n", skipped, d.Name, d.Note)
		}
		return objs, src, d.Name, err
	default:
		return redteam.Objectives{}, redteam.Source{}, "", errors.New("give --objectives <file> or --dataset <name>")
	}
}

func limitObjectives(objs []redteam.Objective, limit int) []redteam.Objective {
	if limit > 0 && len(objs) > limit {
		return objs[:limit]
	}
	return objs
}

func printRedteamResult(w io.Writer, dir string, harmful, benign int, res *redteam.Result) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%d objectives, %d benign requests.\n\n", harmful, benign)
	for _, p := range res.Written {
		fmt.Fprintf(&b, "Wrote %s\n", filepath.ToSlash(filepath.Join(dir, p)))
	}
	for _, p := range res.KeptPersonas {
		fmt.Fprintf(&b, "Kept  %s (already present)\n", filepath.ToSlash(filepath.Join(dir, p)))
	}
	fmt.Fprintf(&b, "\n%d scenarios, %d runs per provider.\n\n"+
		"Add to your arena config (paths are relative to %s):\n\n", len(res.Scenarios), res.Runs, dir)
	for _, line := range strings.Split(strings.TrimSuffix(res.ConfigSnippet(), "\n"), "\n") {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
