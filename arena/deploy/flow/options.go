// Package flow holds the deploy orchestration shared by the CLI and the TUI.
package flow

import "os"

// DefaultEnv is the environment used when none is specified.
const DefaultEnv = "default"

// ConfigureDocsURL is surfaced when deploy config is missing or invalid.
const ConfigureDocsURL = "https://promptarena.altairalabs.ai/arena/how-to/deploy/configure/"

// DefaultConfigFile is the arena config deploy reads when none is given — the
// same default as `promptarena run` and the file `promptarena init` writes.
const DefaultConfigFile = "config.arena.yaml"

// LegacyConfigFile was deploy's default before it matched `run`. It is still
// used when it exists and DefaultConfigFile does not, so projects set up for
// the old default keep working.
const LegacyConfigFile = "arena.yaml"

// Options carries what CLI flags or TUI selections resolve to.
type Options struct {
	ConfigPath string // path to arena config; "" → ResolveConfigPath("")
	Env        string // deploy environment; "" → DefaultEnv
	PackFile   string // pre-compiled *.pack.json; "" → compile from config
	ProjectDir string // project root; "" → os.Getwd()
}

// ResolveConfigPath returns path when set. Otherwise it returns
// DefaultConfigFile, or LegacyConfigFile when only that one exists.
func ResolveConfigPath(path string) string {
	if path != "" {
		return path
	}
	if _, err := os.Stat(DefaultConfigFile); err != nil {
		if _, legacyErr := os.Stat(LegacyConfigFile); legacyErr == nil {
			return LegacyConfigFile
		}
	}
	return DefaultConfigFile
}

// Config returns the arena config path these options resolve to.
func (o Options) Config() string { return ResolveConfigPath(o.ConfigPath) }

func (o Options) config() string { return o.Config() }

func (o Options) dir() (string, error) {
	if o.ProjectDir != "" {
		return o.ProjectDir, nil
	}
	return os.Getwd()
}
