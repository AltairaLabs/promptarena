// Command atlaspreview renders the ported Atlas token layer so it can be
// eyeballed against the design system. It is a development aid for the TUI
// restyle, not part of the shipped arena binary.
//
//	go run ./arena/tui/theme/cmd/atlaspreview          # both themes
//	go run ./arena/tui/theme/cmd/atlaspreview -light   # light only
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/tui/theme"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Preview layout dimensions (cells).
const (
	previewPad    = 2  // page horizontal padding
	previewLabelW = 10 // row-label column width
	previewCardW  = 34 // demo card width
)

func main() {
	light := flag.Bool("light", false, "render the light theme only")
	dark := flag.Bool("dark", false, "render the dark theme only")
	p256 := flag.Bool("p256", false, "force the ANSI256 profile to preview the pinned degradation")
	flag.Parse()

	// Default to truecolor so the full Atlas ramp shows. -p256 forces the
	// 256-color profile instead, so the pinned surface/border fallbacks can be
	// eyeballed (unpinned surfaces would collapse to one index here).
	lipgloss.SetColorProfile(profileFor(*p256))

	run(os.Stdout, *light, *dark)
}

// profileFor picks the color profile to render under.
//
// Truecolor by default so the full Atlas ramp shows; -p256 forces ANSI256 to
// preview the pinned degradation, which is the whole reason the flag exists —
// unpinned surfaces collapse to a single index at 256 colors, and that is
// what a reviewer needs to see before shipping a palette change.
func profileFor(p256 bool) termenv.Profile {
	if p256 {
		return termenv.ANSI256
	}
	return termenv.TrueColor
}

// run writes the requested theme previews to out.
//
// Split from main so the selection is testable: which themes render for a
// given flag combination is the tool's entire behavior, and main itself is
// unreachable from a test. Writing to an io.Writer rather than calling
// fmt.Println keeps the output assertable.
//
// Neither flag renders both, dark first — the default the tool is usually run
// with, and the order the two are meant to be compared in.
func run(out io.Writer, light, dark bool) {
	switch {
	case light:
		_, _ = fmt.Fprintln(out, render(theme.Light()))
	case dark:
		_, _ = fmt.Fprintln(out, render(theme.Dark()))
	default:
		_, _ = fmt.Fprintln(out, render(theme.Dark()))
		_, _ = fmt.Fprintln(out, render(theme.Light()))
	}
}

func render(t theme.Theme) string {
	s := theme.NewStyles(t)
	page := lipgloss.NewStyle().Background(t.BgApp).Padding(1, previewPad)

	body := lipgloss.JoinVertical(lipgloss.Left,
		header(t, s),
		"",
		swatches(t),
		"",
		lipgloss.JoinHorizontal(lipgloss.Top, panel(s), "  ", statuses(s)),
	)
	return page.Render(body)
}

func header(t theme.Theme, s theme.Styles) string {
	title := s.Accent.Render("atlas") + s.Muted.Render(" ❯ ") +
		s.Heading.Render("promptarena")
	mode := s.Label.Render(theme.Eyebrow(t.Name))

	gap := max(58-lipgloss.Width(title)-lipgloss.Width(mode), 1)
	return s.Header.Render(title + strings.Repeat(" ", gap) + mode)
}

// swatches shows the ramps that matter most in a terminal: the ink surfaces
// (which must stay distinguishable) and the accent/status leads.
func swatches(t theme.Theme) string {
	chip := func(c lipgloss.TerminalColor) string {
		return lipgloss.NewStyle().Background(c).Render("      ")
	}
	row := func(label string, cs ...lipgloss.TerminalColor) string {
		var b strings.Builder
		b.WriteString(lipgloss.NewStyle().Foreground(t.TextFaint).Width(previewLabelW).Render(label))
		for _, c := range cs {
			b.WriteString(chip(c))
		}
		return b.String()
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		row("ink", t.SurfaceCode, t.BgApp, t.Surface1, t.Surface2, t.BorderDefault, t.BorderStrong),
		row("text", t.TextHeading, t.TextBody, t.TextMuted, t.TextFaint),
		row("lead", t.AccentPrimary, t.GoldText, t.AccentInter, t.AccentNode),
		row("status", t.StatusHealthy, t.StatusPending, t.StatusError),
		row("category", t.Category[0], t.Category[1], t.Category[2], t.Category[3],
			t.Category[4], t.Category[5], t.Category[6], t.Category[7]),
	)
}

// panel demonstrates the Atlas card convention: a hairline defines the panel,
// an uppercase tracked eyebrow labels it, and mono data sits beneath.
func panel(s theme.Styles) string {
	rows := []struct{ k, v string }{
		{"model", "claude-opus-4-8"},
		{"turns", "18"},
		{"latency", "1.24s"},
	}

	var lines []string
	lines = append(lines, s.Label.Render(theme.Eyebrow("Run summary")), "")
	for _, r := range rows {
		lines = append(lines,
			s.Muted.Width(previewLabelW).Render(r.k)+s.Body.Render(r.v))
	}
	return s.Card.Width(previewCardW).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func statuses(s theme.Styles) string {
	line := func(st lipgloss.Style, glyph, label, detail string) string {
		return st.Render(glyph+" "+label) + "  " + s.Faint.Render(detail)
	}

	lines := []string{
		s.Label.Render(theme.Eyebrow("Agents")), "",
		line(s.Healthy, "●", "researcher", "running"),
		line(s.Pending, "●", "critic", "pending"),
		line(s.Error, "●", "summarizer", "failed"),
	}
	return s.CardFocused.Width(previewCardW).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}
