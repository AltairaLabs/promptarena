package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Atlas spacing rhythm, translated to cells. The CSS scale is in px on a 4px
// grid; a terminal cell is roughly 8px wide and 17px tall, so horizontal
// padding halves and vertical padding collapses hard.
//
//	--space-4 (12px) → 2 cells horizontally
//	--space-6 (16px) → 1 row vertically
const (
	PadX = 2
	PadY = 1
)

// Styles is the prebuilt lipgloss style set for one Theme. Building styles
// once per theme (rather than calling lipgloss.NewStyle() at render time, as
// the legacy TUI does) keeps colour decisions in this package.
type Styles struct {
	Theme Theme

	// Text.
	Heading lipgloss.Style
	Body    lipgloss.Style
	Muted   lipgloss.Style
	Faint   lipgloss.Style
	Link    lipgloss.Style

	// Label is the un-tracked mono label; pair it with Eyebrow for the
	// tracked, uppercase Atlas panel label.
	Label lipgloss.Style

	// Accent renders gold as text. Gold is "the one thing that matters
	// most" — one use per view, never filler.
	Accent lipgloss.Style

	// Info is the neutral interactive accent (starlight) — links, hints, the
	// non-primary "this is interactive" colour.
	Info lipgloss.Style

	// Value is bright emphasised data (heading star, bold).
	Value lipgloss.Style

	// Card is the Atlas panel: a hairline defines it, and there is no
	// drop-shadow.
	Card lipgloss.Style

	// CardFocused is a Card promoted to the strong hairline.
	CardFocused lipgloss.Style

	// Header is the top app bar — a bottom hairline, no side borders.
	Header lipgloss.Style

	// Status text styles.
	Healthy lipgloss.Style
	Pending lipgloss.Style
	Error   lipgloss.Style
}

// NewStyles builds the style set for a theme.
func NewStyles(t Theme) Styles {
	base := lipgloss.NewStyle()

	return Styles{
		Theme: t,

		Heading: base.Foreground(t.TextHeading).Bold(true),
		Body:    base.Foreground(t.TextBody),
		Muted:   base.Foreground(t.TextMuted),
		Faint:   base.Foreground(t.TextFaint),
		Link:    base.Foreground(t.TextLink).Underline(true),
		Label:   base.Foreground(t.TextFaint),
		Accent:  base.Foreground(t.AccentPrimary).Bold(true),
		Info:    base.Foreground(t.AccentInter),
		Value:   base.Foreground(t.TextHeading).Bold(true),

		Card: base.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.BorderDefault).
			Padding(PadY, PadX),

		CardFocused: base.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.BorderStrong).
			Padding(PadY, PadX),

		Header: base.
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(t.BorderDefault).
			Padding(0, PadX),

		Healthy: base.Foreground(t.StatusHealthyText),
		Pending: base.Foreground(t.StatusPending),
		Error:   base.Foreground(t.StatusErrorText),
	}
}

// Eyebrow renders text in the Atlas panel-label style: uppercase with
// letter-spacing. CSS expresses the spacing as --tracking-eyebrow (0.14em);
// the terminal has no sub-cell spacing, so tracking becomes a space between
// characters and a double space between words.
func Eyebrow(s string) string {
	var b strings.Builder

	for i, word := range strings.Fields(strings.ToUpper(s)) {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(strings.Join(strings.Split(word, ""), " "))
	}
	return b.String()
}
