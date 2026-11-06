package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigOutputDir(t *testing.T) {
	assert.Equal(t, DefaultOutputDir, (&Config{}).OutputDir())
	assert.Equal(t, "legacy", (&Config{Defaults: Defaults{OutDir: "legacy"}}).OutputDir())
	assert.Equal(t, "new", (&Config{Defaults: Defaults{
		OutDir: "legacy", Output: OutputConfig{Dir: "new"},
	}}).OutputDir())
}

func TestConfigResolveOutputDir(t *testing.T) {
	cfg := &Config{Defaults: Defaults{Output: OutputConfig{Dir: "results"}}}
	assert.Equal(t, "results", cfg.ResolveOutputDir("out", false), "an unset flag defers to the config")

	cfg = &Config{Defaults: Defaults{Output: OutputConfig{Dir: "results"}}}
	assert.Equal(t, "cli", cfg.ResolveOutputDir("cli", true), "a set flag wins")
	assert.Equal(t, "cli", cfg.OutputDir(), "the winner is what media and recordings read")

	// The legacy formats must still resolve: recording the flag must not
	// switch GetOutputConfig onto the (empty) new-style block.
	cfg = &Config{Defaults: Defaults{OutputFormats: []string{"junit"}}}
	cfg.ResolveOutputDir("cli", true)
	assert.Equal(t, []string{"junit"}, cfg.Defaults.GetOutputConfig().Formats)

	assert.Equal(t, DefaultOutputDir, (&Config{}).ResolveOutputDir("", false))
}
