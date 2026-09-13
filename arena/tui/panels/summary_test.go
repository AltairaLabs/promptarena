package panels

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/tui/viewmodels"
)

// TestSummaryPanel_ViewRendersTheRunTotals covers SummaryPanel.View, which was
// 0%. The panel is the last thing a run prints, so the numbers it shows are
// what someone reports as the result — a panel that renders empty looks like a
// run that produced nothing rather than a rendering bug.
func TestSummaryPanel_ViewRendersTheRunTotals(t *testing.T) {
	p := NewSummaryPanel()
	p.Update(100)

	vm := viewmodels.NewSummaryViewModel(&viewmodels.SummaryData{
		TotalRuns:     7,
		CompletedRuns: 5,
		FailedRuns:    2,
		ScenarioCount: 3,
	})

	out := p.View(vm, false)
	require.NotEmpty(t, out, "the summary panel must render something")

	// The counts are the payload; assert on them rather than on layout, which
	// is lipgloss's business and changes with width.
	for _, want := range []string{"7", "5", "2"} {
		assert.Contains(t, out, want, "run totals missing from:\n%s", out)
	}
}

// TestSummaryPanel_ViewHonorsWidth pins that Update's width actually reaches
// the view. Without it the panel renders at zero width and collapses to
// unreadable single-character lines.
func TestSummaryPanel_ViewHonorsWidth(t *testing.T) {
	vm := viewmodels.NewSummaryViewModel(&viewmodels.SummaryData{TotalRuns: 1})

	narrow := NewSummaryPanel()
	narrow.Update(40)

	wide := NewSummaryPanel()
	wide.Update(200)

	narrowOut, wideOut := narrow.View(vm, false), wide.View(vm, false)
	require.NotEmpty(t, narrowOut)
	require.NotEmpty(t, wideOut)

	widest := func(s string) int {
		var maxLen int
		for _, line := range strings.Split(s, "\n") {
			if len([]rune(line)) > maxLen {
				maxLen = len([]rune(line))
			}
		}
		return maxLen
	}
	assert.Greater(t, widest(wideOut), widest(narrowOut),
		"a wider panel must actually render wider")
}

// TestSummaryPanel_ViewSurvivesAnEmptyRun covers the zero case: a run that
// executed nothing still has to render, because the panel is also what an
// aborted run leaves on screen.
func TestSummaryPanel_ViewSurvivesAnEmptyRun(t *testing.T) {
	p := NewSummaryPanel()
	p.Update(80)
	vm := viewmodels.NewSummaryViewModel(&viewmodels.SummaryData{})

	assert.NotPanics(t, func() {
		_ = p.View(vm, false)
	})
}
