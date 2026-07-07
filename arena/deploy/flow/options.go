// Package flow holds the deploy orchestration shared by the CLI and the TUI.
package flow

import "os"

// DefaultEnv is the environment used when none is specified.
const DefaultEnv = "default"

// ConfigureDocsURL is surfaced when deploy config is missing or invalid.
const ConfigureDocsURL = "https://promptkit.altairalabs.ai/arena/how-to/deploy/configure/"

// Options carries what CLI flags or TUI selections resolve to.
type Options struct {
	ConfigPath string // path to arena config; "" → "arena.yaml"
	Env        string // deploy environment; "" → DefaultEnv
	PackFile   string // pre-compiled *.pack.json; "" → compile from config
	ProjectDir string // project root; "" → os.Getwd()
}

func (o Options) config() string {
	if o.ConfigPath != "" {
		return o.ConfigPath
	}
	return "arena.yaml"
}

func (o Options) dir() (string, error) {
	if o.ProjectDir != "" {
		return o.ProjectDir, nil
	}
	return os.Getwd()
}
