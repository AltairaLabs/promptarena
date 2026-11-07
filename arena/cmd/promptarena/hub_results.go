package main

import (
	"context"
	"path/filepath"

	"github.com/AltairaLabs/promptarena/v2/arena/engine"
)

// saveHubRunResults writes the results of a run started from the hub's Run
// page with the same writers `promptarena run` uses. The hub takes no run
// flags, so the output formats, directory and file names are the ones `run`
// resolves from the config when given none.
//
// `run` resolves a relative output path against the working directory. The
// hub can switch to a config in another directory, and its View page reads
// the out/ directory next to the loaded config, so relative paths are
// resolved against the config's directory instead. For a hub opened on the
// config in the working directory (the usual case) the two are the same.
func saveHubRunResults(eng *engine.Engine, configFile string, runIDs []string) error {
	params, err := extractRunParameters(runCmd, eng.GetConfig())
	if err != nil {
		return err
	}
	base := filepath.Dir(configFile)
	for _, p := range []*string{&params.OutDir, &params.JUnitFile, &params.MarkdownFile} {
		if *p != "" && !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
	}
	results, err := convertRunResults(context.Background(), eng, runIDs)
	if err != nil {
		return err
	}
	return processResults(results, params, configFile)
}
