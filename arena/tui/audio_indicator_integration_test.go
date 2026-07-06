package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	arenaaudio "github.com/AltairaLabs/promptarena/arena/audio"
)

// TestRunsPane_ShowsAudioGlyph_WithRealMonitor drives the full model render
// with a REAL arenaaudio.Monitor (not a fake), a real router attached for a
// running run, and asserts the runs pane shows the active-audio glyph. This
// guards the live wiring path RunPage.attachAudioMonitor uses:
// Monitor.AttachRouter -> Monitor.HasAudio/ActiveRunID -> convertToRunInfos ->
// panels runs table.
func TestRunsPane_ShowsAudioGlyph_WithRealMonitor(t *testing.T) {
	mon, err := arenaaudio.NewMonitor(arenaaudio.MonitorConfig{
		Rate:            arenaaudio.Rate24k,
		EnableLocalSink: false, // metering-only: no host audio device needed in tests
	})
	require.NoError(t, err)

	router := arenaaudio.NewAudioRouter(arenaaudio.Rate24k)
	defer router.Close()
	mon.AttachRouter("run-x", router) // first attach auto-activates -> active source

	m := newGoldenModel()
	m.SetAudioMonitor(mon)
	m.activeRuns = []RunInfo{{
		RunID:     "run-x",
		Status:    StatusRunning,
		Scenario:  "duplex-scenario",
		Provider:  "mock-duplex",
		StartTime: time.Now(),
	}}

	out := renderGolden(m, 160, 50)
	assert.Contains(t, out, "Audio", "runs pane should have the Audio column header")
	assert.Contains(t, out, "LIVE", "active audio run should render the LIVE marker")
}
