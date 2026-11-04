package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/calibrate"
)

func TestLoadCheck(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "check.yaml")
	require.NoError(t, os.WriteFile(p, []byte("type: content_includes\nparams:\n  patterns: [refund]\nmessage: m\n"), 0o600))
	check, err := loadCheck(p)
	require.NoError(t, err)
	assert.Equal(t, "content_includes", check.Type)
	assert.Equal(t, "m", check.Message)

	require.NoError(t, os.WriteFile(p, []byte("params: {}\n"), 0o600))
	_, err = loadCheck(p)
	assert.ErrorContains(t, err, "needs a type")
	_, err = loadCheck(filepath.Join(dir, "missing.yaml"))
	assert.ErrorContains(t, err, "read check")
}

func TestCalibrationGate(t *testing.T) {
	r := &calibrate.Report{Evaluated: 10, Kappa: 0.5, Agreement: 0.8}
	assert.NoError(t, calibrationGate(r, false, 0, false, 0))
	assert.NoError(t, calibrationGate(r, true, 0.5, true, 0.8))
	assert.ErrorContains(t, calibrationGate(r, true, 0.6, false, 0), "kappa 0.50 is below --min-kappa 0.60")
	assert.ErrorContains(t, calibrationGate(r, false, 0, true, 0.9), "agreement 0.800 is below --min-agreement 0.900")
	assert.ErrorContains(t, calibrationGate(&calibrate.Report{Errored: 3}, false, 0, false, 0),
		"no example could be evaluated (3 not evaluated)")
}

func TestWriteCalibration(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	require.NoError(t, writeCalibration(dir, &calibrate.Report{Examples: 1}, "# md"))
	for _, name := range []string{"calibration.json", "calibration.md"} {
		_, err := os.Stat(filepath.Join(dir, name))
		assert.NoError(t, err, name)
	}
}

// runCalibrateWith runs the calibrate command against the adaptive-persona
// example with the given flag overrides.
func runCalibrateWith(t *testing.T, overrides map[string]string) (string, error) {
	t.Helper()
	example := filepath.Join("..", "..", "..", "examples", "adaptive-persona")
	flags := map[string]string{
		"config":   example,
		"check":    filepath.Join(example, "checks", "refusal.yaml"),
		"examples": filepath.Join(example, "data", "refusal-labels.jsonl"),
		"out":      t.TempDir(),
	}
	for k, v := range overrides {
		flags[k] = v
	}
	saved := calibrateFlags
	t.Cleanup(func() {
		calibrateFlags = saved
		for _, name := range []string{"min-kappa", "min-agreement"} {
			if f := calibrateCmd.Flags().Lookup(name); f != nil {
				f.Changed = false
			}
		}
	})
	for k, v := range flags {
		require.NoError(t, calibrateCmd.Flags().Set(k, v))
	}
	var out strings.Builder
	calibrateCmd.SetOut(&out)
	calibrateCmd.SetContext(context.Background())
	err := runCalibrate(calibrateCmd, nil)
	return out.String(), err
}

func TestRunCalibrate_Example(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	out, err := runCalibrateWith(t, nil)
	require.NoError(t, err)
	assert.Contains(t, out, "| Agreement | 7 / 10 (70.0%) |")

	_, err = runCalibrateWith(t, map[string]string{"min-kappa": "0.9"})
	assert.ErrorContains(t, err, "below --min-kappa 0.90")
}

func TestRunCalibrate_Errors(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()
	typo := filepath.Join(dir, "typo.yaml")
	require.NoError(t, os.WriteFile(typo, []byte("type: refusl\n"), 0o600))
	_, err := runCalibrateWith(t, map[string]string{"check": typo})
	assert.ErrorContains(t, err, `did you mean "refusal"?`)

	_, err = runCalibrateWith(t, map[string]string{"examples": filepath.Join(dir, "missing.jsonl")})
	assert.ErrorContains(t, err, "read examples")

	_, err = runCalibrateWith(t, map[string]string{"config": filepath.Join(dir, "missing.arena.yaml")})
	assert.ErrorContains(t, err, "failed to load config")

	_, err = runCalibrateWith(t, map[string]string{"check": filepath.Join(dir, "none.yaml")})
	assert.ErrorContains(t, err, "read check")
}
