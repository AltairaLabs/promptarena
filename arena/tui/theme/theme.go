package theme

import "github.com/charmbracelet/lipgloss"

// Theme is the resolved Atlas semantic layer for one mode. It is a value, not
// a set of package globals: construct one with Dark() or Light() and thread it
// through render code, so a second theme (or a test fixture) never requires
// mutating global state.
//
// Field names mirror the Atlas semantic aliases in colors.css. Reach for these
// rather than the raw ramp — the raw ramp is an implementation detail of the
// port and is deliberately unexported.
type Theme struct {
	// Name identifies the mode ("dark" / "light").
	Name string

	// Surfaces — --bg-app, --surface-*.
	BgApp       lipgloss.TerminalColor
	Surface1    lipgloss.TerminalColor
	Surface2    lipgloss.TerminalColor
	SurfaceCode lipgloss.TerminalColor
	SurfaceCard lipgloss.TerminalColor

	// Borders — Atlas alpha hairlines, flattened against BgApp.
	BorderDefault lipgloss.TerminalColor
	BorderStrong  lipgloss.TerminalColor
	BorderFaint   lipgloss.TerminalColor

	// Text — --text-*.
	TextHeading lipgloss.TerminalColor
	TextBody    lipgloss.TerminalColor
	TextMuted   lipgloss.TerminalColor
	TextFaint   lipgloss.TerminalColor
	TextLink    lipgloss.TerminalColor
	TextOnGold  lipgloss.TerminalColor

	// Accents. AccentPrimary is gold — "the one thing that matters most",
	// used once per view, never as filler.
	AccentPrimary lipgloss.TerminalColor
	AccentInter   lipgloss.TerminalColor
	AccentNode    lipgloss.TerminalColor

	// GoldText is gold used as TEXT rather than as a fill; it deepens on
	// light where --gold-500 would be illegible.
	GoldText lipgloss.TerminalColor

	// Status — --status-*.
	StatusHealthy     lipgloss.TerminalColor
	StatusHealthyText lipgloss.TerminalColor
	StatusPending     lipgloss.TerminalColor
	StatusError       lipgloss.TerminalColor
	StatusErrorText   lipgloss.TerminalColor

	// Node kinds — --node-* (constellation graph).
	NodePrompt lipgloss.TerminalColor
	NodeAgent  lipgloss.TerminalColor
	NodeTool   lipgloss.TerminalColor
	NodeBranch lipgloss.TerminalColor
	NodeOutput lipgloss.TerminalColor

	// Category is the 8-slot ordered categorical palette. Consumers map
	// their own domain categories onto slots; slot 8 reads as neutral.
	Category [8]lipgloss.TerminalColor
}

// HexOf returns a token's truecolor #RRGGBB value, whether it auto-degrades
// (a plain color) or is pinned across profiles (a CompleteColor). Useful for
// probes and tests; render code should pass the token itself to lipgloss.
func HexOf(tc lipgloss.TerminalColor) string {
	switch v := tc.(type) {
	case lipgloss.Color:
		return string(v)
	case lipgloss.CompleteColor:
		return v.TrueColor
	default:
		return ""
	}
}

// Dark returns the Atlas dark theme — "night sky, starlight, gold". This is
// the Atlas default.
func Dark() Theme { return resolve("dark", darkPalette) }

// Light returns the Atlas light theme — "the printed star chart".
func Light() Theme { return resolve("light", lightPalette) }

// resolve applies the semantic alias layer from colors.css to a raw ramp.
func resolve(name string, p palette) Theme {
	// c leaves a token to lipgloss's automatic degradation — correct for the
	// distinct-hue accents, status, and text ramps.
	c := func(s string) lipgloss.TerminalColor { return lipgloss.Color(s) }

	// pin fixes a token's rendering across all three profiles so it does NOT
	// auto-degrade. Used only for the near-navy surfaces and borders, which
	// would otherwise collapse onto one 256-color index (see fallback).
	pin := func(hex string, f fallback) lipgloss.TerminalColor {
		return lipgloss.CompleteColor{TrueColor: hex, ANSI256: f.c256, ANSI: f.c16}
	}

	// Alpha hairlines composite against the app background. Atlas layers them
	// over varying surfaces in CSS, but the terminal cannot blend per-cell, so
	// BgApp is the single reference backdrop. The flattened truecolor is pinned
	// to a grey index for limited terminals.
	border := func(a rgba, f fallback) lipgloss.TerminalColor {
		return pin(flattenOver(a, p.inkCanvas), f)
	}

	t := Theme{
		Name: name,

		BgApp:       pin(p.inkCanvas, p.fbCanvas),
		Surface1:    pin(p.inkSurface, p.fbSurface),
		Surface2:    pin(p.inkRaised, p.fbRaised),
		SurfaceCode: pin(p.inkVoid, p.fbVoid),
		SurfaceCard: pin(p.inkSurface, p.fbSurface),

		BorderDefault: border(p.hairline, p.fbBorderDefault),
		BorderStrong:  border(p.hairlineStrong, p.fbBorderStrong),
		BorderFaint:   border(p.hairlineFaint, p.fbBorderFaint),

		TextHeading: c(p.star200),
		TextBody:    c(p.star300),
		TextMuted:   c(p.star700),
		TextFaint:   c(p.star900),
		TextLink:    c(p.starlight300),
		TextOnGold:  c(p.goldInk),

		AccentPrimary: c(p.gold500),
		AccentInter:   c(p.starlight500),
		AccentNode:    c(p.starlight300),
		GoldText:      c(p.gold300),

		StatusHealthy:     c(p.pulsar500),
		StatusHealthyText: c(p.pulsar300),
		StatusPending:     c(p.amber500),
		StatusError:       c(p.signalRed),
		StatusErrorText:   c(p.signalRed300),

		NodePrompt: c(p.nebulaViolet),
		NodeAgent:  c(p.starlight300),
		NodeTool:   c(p.ionCyan),
		NodeBranch: c(p.gold500),
		NodeOutput: c(p.gold500),
	}

	for i, hex := range p.category {
		t.Category[i] = c(hex)
	}
	return t
}
