package arenaconfig

import (
	"path/filepath"
)

// ResolveConfigDir resolves the base directory for config files with proper defaulting behavior:
// 1. If cfg.Defaults.ConfigDir is explicitly set, use it
// 2. Otherwise, if configFilePath is provided, use its directory
// 3. Otherwise, default to current working directory (".")
//
// This ensures all config file types (prompts, providers, scenarios, tools) are resolved
// relative to the same base directory.
func ResolveConfigDir(cfg *Config, configFilePath string) string {
	// Use explicit config_dir if specified
	if cfg.Defaults.ConfigDir != "" {
		return cfg.Defaults.ConfigDir
	}

	// Use directory containing the main config file
	if configFilePath != "" {
		dir := filepath.Dir(configFilePath)
		return dir
	}

	// Default to current working directory
	return "."
}

// absDir returns the directory holding path, made absolute when the working
// directory allows, so a fragment error names a file a reader can find.
func absDir(path string) string {
	dir := filepath.Dir(path)
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// DefaultOutputDir is where results, media and recordings go when neither the
// --out flag nor defaults.output.dir names a directory.
const DefaultOutputDir = "out"

// OutputDir returns where results, media and recordings go, in the precedence
// every command shares: the --out override recorded by ResolveOutputDir, then
// defaults.output.dir, then the deprecated defaults.out_dir, then
// DefaultOutputDir. A relative path is relative to the working directory.
func (c *Config) OutputDir() string {
	switch {
	case c.outputDirOverride != "":
		return c.outputDirOverride
	case c.Defaults.Output.Dir != "":
		return c.Defaults.Output.Dir
	case c.Defaults.OutDir != "":
		return c.Defaults.OutDir
	default:
		return DefaultOutputDir
	}
}

// ResolveOutputDir records an explicitly set --out flag as the output
// directory and returns the result of OutputDir. Recording it on the config
// is what makes the engine's media storage and session recordings, which read
// the config, land in the same directory as the results.
func (c *Config) ResolveOutputDir(flagValue string, flagSet bool) string {
	if flagSet && flagValue != "" {
		c.outputDirOverride = flagValue
	}
	return c.OutputDir()
}
