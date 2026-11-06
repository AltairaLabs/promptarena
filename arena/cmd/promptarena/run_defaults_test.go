package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

// newRunDefaultsCmd builds a command with run's concurrency and out flags and
// their real defaults, so each test starts with nothing marked Changed.
func newRunDefaultsCmd() *cobra.Command {
	c := &cobra.Command{Use: "run"}
	c.Flags().IntP("concurrency", "j", 6, "")
	c.Flags().StringP("out", "o", "out", "")
	c.Flags().StringP("config", "c", "config.arena.yaml", "")
	return c
}

func TestApplyConfigRunDefaults_Concurrency(t *testing.T) {
	tests := []struct {
		name   string
		flag   string // "" leaves -j unset
		config int
		want   int
	}{
		{name: "neither set uses the flag default", want: 6},
		{name: "config value used when -j unset", config: 2, want: 2},
		{name: "explicit -j overrides config", flag: "3", config: 2, want: 3},
		{name: "explicit -j equal to the default still overrides", flag: "6", config: 2, want: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRunDefaultsCmd()
			if tt.flag != "" {
				require.NoError(t, cmd.Flags().Set("concurrency", tt.flag))
			}
			params := &RunParameters{}
			params.Concurrency, _ = cmd.Flags().GetInt("concurrency")
			params.OutDir, _ = cmd.Flags().GetString("out")
			cfg := &arenaconfig.Config{Defaults: arenaconfig.Defaults{Concurrency: tt.config}}

			applyConfigRunDefaults(cmd, cfg, params)
			assert.Equal(t, tt.want, params.Concurrency)
		})
	}
}

func TestApplyConfigRunDefaults_OutputDir(t *testing.T) {
	tests := []struct {
		name      string
		flag      string // "" leaves --out unset
		configDir string
		legacyDir string
		want      string
	}{
		{name: "neither set falls back to out", want: "out"},
		{name: "defaults.output.dir used when --out unset", configDir: "results", want: "results"},
		{name: "deprecated out_dir used when nothing newer is set", legacyDir: "legacy", want: "legacy"},
		{name: "explicit --out overrides config", flag: "cli-out", configDir: "results", want: "cli-out"},
		{name: "explicit --out out still overrides config", flag: "out", configDir: "results", want: "out"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRunDefaultsCmd()
			if tt.flag != "" {
				require.NoError(t, cmd.Flags().Set("out", tt.flag))
			}
			params := &RunParameters{}
			params.OutDir, _ = cmd.Flags().GetString("out")
			cfg := &arenaconfig.Config{Defaults: arenaconfig.Defaults{
				Output: arenaconfig.OutputConfig{Dir: tt.configDir},
				OutDir: tt.legacyDir,
			}}

			applyConfigRunDefaults(cmd, cfg, params)
			assert.Equal(t, tt.want, params.OutDir)
			// Written back so media storage and recordings follow it.
			assert.Equal(t, tt.want, cfg.OutputDir())
		})
	}
}

func TestResolveRunConfigPath(t *testing.T) {
	t.Run("no argument uses the -c default", func(t *testing.T) {
		got, err := resolveRunConfigPath(newRunDefaultsCmd(), nil)
		require.NoError(t, err)
		assert.Equal(t, "config.arena.yaml", got)
	})
	t.Run("one argument is the config path", func(t *testing.T) {
		got, err := resolveRunConfigPath(newRunDefaultsCmd(), []string{"arena.yaml"})
		require.NoError(t, err)
		assert.Equal(t, "arena.yaml", got)
	})
	t.Run("-c alone is used", func(t *testing.T) {
		cmd := newRunDefaultsCmd()
		require.NoError(t, cmd.Flags().Set("config", "x.arena.yaml"))
		got, err := resolveRunConfigPath(cmd, nil)
		require.NoError(t, err)
		assert.Equal(t, "x.arena.yaml", got)
	})
	t.Run("argument and -c together is an error", func(t *testing.T) {
		cmd := newRunDefaultsCmd()
		require.NoError(t, cmd.Flags().Set("config", "x.arena.yaml"))
		_, err := resolveRunConfigPath(cmd, []string{"arena.yaml"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config path given twice")
	})
}

func TestRunCmdRejectsMoreThanOneArgument(t *testing.T) {
	require.NoError(t, runCmd.Args(runCmd, []string{"a.yaml"}))
	assert.Error(t, runCmd.Args(runCmd, []string{"a.yaml", "b.yaml"}))
}
