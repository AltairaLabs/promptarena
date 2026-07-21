// Package console renders Atlas-themed output for the non-TUI ("simple") run
// path: styled run info and a live progress status bar. Colours come from the
// theme package's active theme and degrade automatically when stdout is not a
// terminal, so piped/CI output stays clean.
package console

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"

	"github.com/AltairaLabs/promptarena/arena/tui/theme"
)

// barWidth is the number of cells in the progress bar.
const barWidth = 24

// StatusBar tracks run completion and renders a single Atlas-styled progress
// line. It is safe for concurrent Advance calls (runs complete on worker
// goroutines).
type StatusBar struct {
	total int
	tty   bool

	mu     sync.Mutex
	done   int
	failed int

	// outMu serialises writes to the destination so concurrent Live calls from
	// worker goroutines do not interleave. Kept separate from mu so Live can
	// hold it while Render independently takes mu (mu is not reentrant).
	outMu sync.Mutex
}

// NewStatusBar creates a status bar for total runs. tty reports whether the
// destination is an interactive terminal; when false the bar never rewrites in
// place (Live is a no-op) so CI logs are not spammed with carriage returns.
func NewStatusBar(total int, tty bool) *StatusBar {
	return &StatusBar{total: total, tty: tty}
}

// Advance records one completed run, counting a failure when err is non-nil.
func (s *StatusBar) Advance(err error) {
	s.mu.Lock()
	s.done++
	if err != nil {
		s.failed++
	}
	s.mu.Unlock()
}

// Render returns the styled status line. Exported for testing; callers use
// Live / Finish to actually paint it.
func (s *StatusBar) Render() string {
	s.mu.Lock()
	done, failed, total := s.done, s.failed, s.total
	s.mu.Unlock()

	st := theme.Active()

	filled := 0
	if total > 0 {
		filled = done * barWidth / total
	}
	if filled > barWidth {
		filled = barWidth
	}

	bar := lipgloss.NewStyle().Foreground(theme.Colors().AccentInter).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(theme.Colors().BorderStrong).Render(strings.Repeat("░", barWidth-filled))

	label := "Running"
	labelStyle := st.Info
	if done >= total {
		label = "Done"
		labelStyle = st.Healthy
	}

	line := labelStyle.Render(label) + "  " +
		lipgloss.NewStyle().Foreground(theme.Colors().TextFaint).Render("[") + bar +
		lipgloss.NewStyle().Foreground(theme.Colors().TextFaint).Render("]") + "  " +
		st.Body.Render(fmt.Sprintf("%d/%d", done, total))

	if failed > 0 {
		line += st.Faint.Render("  ·  ") + st.Error.Render(fmt.Sprintf("%d failed", failed))
	}
	return line
}

// Live repaints the bar in place on a terminal (leading carriage return, no
// newline). On a non-terminal destination it does nothing, so progress does not
// clutter piped/CI output.
func (s *StatusBar) Live(w io.Writer) {
	if !s.tty {
		return
	}
	line := s.Render()
	s.outMu.Lock()
	defer s.outMu.Unlock()
	fmt.Fprint(w, "\r\033[K"+line)
}

// Finish writes the final bar followed by a newline. On a terminal it clears
// the live line first; on a non-terminal it just prints the summary line once.
func (s *StatusBar) Finish(w io.Writer) {
	line := s.Render()
	s.outMu.Lock()
	defer s.outMu.Unlock()
	if s.tty {
		fmt.Fprint(w, "\r\033[K")
	}
	fmt.Fprintln(w, line)
}
