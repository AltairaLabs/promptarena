package console

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/AltairaLabs/promptarena/arena/tui/theme"
)

// Banner renders the product heading for the non-TUI run: a gold sparkle (the
// PromptArena mark) followed by the bright wordmark. Gold appears once, on the
// mark — matching the TUI's restraint.
func Banner(title string) string {
	mark := lipgloss.NewStyle().Foreground(theme.Colors().AccentPrimary).Render("✦")
	return mark + " " + theme.Active().Heading.Render(title)
}

// Field renders a "label  value" line: muted mono-ish label, body value. Used
// for the run configuration summary.
func Field(label, value string) string {
	return theme.Active().Muted.Render(label+":") + " " + theme.Active().Body.Render(value)
}

// Note renders a secondary, muted line (e.g. "Starting execution…").
func Note(s string) string {
	return theme.Active().Muted.Render(s)
}

// Emphasis renders a value that should stand out without being a status color
// (starlight) — e.g. a count.
func Emphasis(s string) string {
	return theme.Active().Info.Render(s)
}

// Countf is a convenience for an emphasized count embedded in a muted sentence,
// e.g. Countf("Generated", 12, "run combinations").
func Countf(prefix string, n int, suffix string) string {
	return theme.Active().Muted.Render(prefix+" ") +
		Emphasis(fmt.Sprintf("%d", n)) +
		theme.Active().Muted.Render(" "+suffix)
}
