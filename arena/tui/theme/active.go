// Package theme is the Atlas design-system token layer for the TUI: a
// hand-port of the Atlas L0 color tokens (dark + light) with a process-wide
// active theme, prebuilt lipgloss styles, and helpers. Render code reaches for
// Colors() / Active() rather than raw hex values.
package theme

import (
	"strings"
	"sync"

	"github.com/muesli/termenv"
)

// The active theme is the single Styles set selected for this terminal session.
// A TUI is not a web app: the theme is chosen once at startup (from the
// terminal background, or an explicit override) and never toggled per-render.
// So rather than thread a Styles value through every view constructor, render
// code reads the process-wide Active() — set once by the launcher before the
// bubbletea program runs.
//
// Set-once-before-Run is the contract. The guard exists only so that a
// background goroutine reading the theme during startup cannot race the
// launcher's write; it is not a license to swap themes mid-session.
var (
	activeMu sync.RWMutex
	active   = NewStyles(Dark())
)

// Active returns the Styles for the current session theme.
func Active() Styles {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return active
}

// Colors returns the resolved Theme (semantic colors) for the current
// session. Use it when a call site builds its own lipgloss.Style and only
// needs a color — e.g. Foreground(theme.Colors().TextBody) — so the site
// keeps its own bold/width/align modifiers.
func Colors() Theme {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return active.Theme
}

// SetActive selects the session theme. Call once at startup (or from tests).
func SetActive(t Theme) {
	activeMu.Lock()
	defer activeMu.Unlock()
	active = NewStyles(t)
}

// Choose resolves the session theme from an explicit override and the detected
// terminal background. An override of "light"/"dark" wins; anything else
// (including "") falls back to the terminal, which defaults to dark when it
// cannot be read — matching the TUI's historical hardcoded-dark behavior.
func Choose(override string, darkTerminal bool) Theme {
	switch strings.ToLower(strings.TrimSpace(override)) {
	case "light":
		return Light()
	case "dark":
		return Dark()
	}
	if darkTerminal {
		return Dark()
	}
	return Light()
}

// Detect resolves and installs the session theme from an explicit override
// (e.g. an ARENA_THEME env value) and the real terminal background. Call it
// once at startup, before the bubbletea program runs.
func Detect(override string) {
	SetActive(Choose(override, termenv.HasDarkBackground()))
}
