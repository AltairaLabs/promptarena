package theme

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// This file is a hand-port of the Atlas L0 token layer
// (@altairalabs/atlas-tokens, packages/tokens/src/colors.css and
// theme-light.css). Values are transcribed verbatim from the CSS raw ramps;
// the semantic aliases are resolved in theme.go.
//
// Keep in sync by eye with the CSS. The parity tests in atlas_test.go pin the
// values that matter, so drift shows up as a test failure rather than a
// silent visual regression.

// rgba is an Atlas alpha token. The terminal has no alpha channel, so these
// only ever appear flattened against a known backdrop via flattenOver.
type rgba struct {
	R, G, B uint8
	A       float64
}

// fallback pins the ANSI256 and ANSI (16-colour) rendering of a token so
// lipgloss does NOT auto-degrade it. It exists only for the ink surfaces and
// borders: those are all near-navy near-black (or, in light, near-white), and
// termenv's nearest-colour match collapses four distinct surfaces onto a single
// 256-colour index (measured: #070C16/#0A1120/#0C1526/#111C2E all → 232), which
// would make cards and panels indistinguishable. Text and accents are distinct
// hues that auto-degrade cleanly, so they are left unpinned. Values are 256/16
// palette indices as strings, monotonic so surfaces stay layered.
type fallback struct {
	c256, c16 string
}

// palette is the raw Atlas ramp for a single theme. These are the CSS custom
// properties before semantic aliasing — --ink-*, --star-*, --gold-*, etc.
type palette struct {
	// Ink ramp — the night sky (or the paper, in light).
	inkVoid, inkCanvas, inkSurface, inkRaised, inkTile, inkHairline string

	// Starlight text ramp, brightest → faintest.
	star100, star200, star300, star400, star500 string
	star600, star700, star800, star900, star950 string
	starDim                                     string

	// Leads.
	gold500, gold300, gold200, goldInk       string
	starlight500, starlight300, starlight200 string

	// Nebula support.
	nebulaViolet, nebulaVioletDeep, ionCyan string
	pulsar500, pulsar300                    string
	amber500, signalRed, signalRed300       string

	// Alpha tints — flattened against a backdrop before use.
	hairline, hairlineStrong, hairlineFaint rgba

	// Categorical palette, 8 ordered slots (slot 8 = neutral/"other").
	category [8]string

	// 256/16 pins for the collapse-prone surface + border slots. Ordered dark
	// to light so surfaces stay visibly layered on limited terminals.
	fbVoid, fbCanvas, fbSurface, fbRaised, fbTile, fbHairline fallback
	fbBorderDefault, fbBorderStrong, fbBorderFaint            fallback
}

// darkPalette mirrors :root in colors.css.
var darkPalette = palette{
	inkVoid: "#070C16", inkCanvas: "#0A1120", inkSurface: "#0C1526",
	inkRaised: "#111C2E", inkTile: "#101D30", inkHairline: "#1E3149",

	star100: "#F4F8FF", star200: "#F2F6FC", star300: "#E6EDF8",
	star400: "#DCE6F5", star500: "#C7D5EA", star600: "#9FB1CC",
	star700: "#8195B0", star800: "#73849F", star900: "#5E708C",
	star950: "#4A5A74", starDim: "#2E3D54",

	gold500: "#E3B341", gold300: "#F0D79A", gold200: "#F4D27A", goldInk: "#06122A",
	starlight500: "#3B82F6", starlight300: "#93C5FD", starlight200: "#BFD6F2",

	nebulaViolet: "#C4B5FD", nebulaVioletDeep: "#8B5CF6", ionCyan: "#67E8F9",
	pulsar500: "#34D399", pulsar300: "#7DD3A8",
	amber500: "#F59E0B", signalRed: "#EF4444", signalRed300: "#FCA5A5",

	hairline:       rgba{147, 197, 253, 0.10},
	hairlineStrong: rgba{147, 197, 253, 0.18},
	hairlineFaint:  rgba{147, 197, 253, 0.06},

	category: [8]string{
		"#60A5FA", "#A78BFA", "#F472B6", "#FBBF24",
		"#34D399", "#22D3EE", "#F87171", "#9CA3AF",
	},

	// Dark: greyscale ramp 232→238, ascending so void < canvas < surface <
	// tile < raised < hairline stay distinct. ANSI: black shell, bright-black
	// for raised/hairline/borders so dividers remain visible on 16 colours.
	fbVoid:          fallback{"232", "0"},
	fbCanvas:        fallback{"233", "0"},
	fbSurface:       fallback{"234", "0"},
	fbTile:          fallback{"235", "8"},
	fbRaised:        fallback{"236", "8"},
	fbHairline:      fallback{"238", "8"},
	fbBorderDefault: fallback{"236", "8"},
	fbBorderStrong:  fallback{"238", "8"},
	fbBorderFaint:   fallback{"234", "8"},
}

// lightPalette mirrors [data-theme="light"] in theme-light.css. Note that gold
// FILLS stay identical while gold TEXT deepens — light gold is illegible on
// paper.
var lightPalette = palette{
	// TUI deviation: Atlas light sets --ink-raised to #FFFFFF, distinguished
	// from --ink-surface only by a drop-shadow. Terminals have no shadow, so
	// raised surfaces borrow Atlas's own --ink-tile (#EAF0F8) to stay visible
	// against a white card. Dark is untouched — its ramp already separates the
	// two.
	inkVoid: "#E9EEF6", inkCanvas: "#F3F6FB", inkSurface: "#FFFFFF",
	inkRaised: "#EAF0F8", inkTile: "#EAF0F8", inkHairline: "#D3DCE8",

	star100: "#0A1424", star200: "#101C2F", star300: "#1E2C44",
	star400: "#263751", star500: "#3A4C68", star600: "#4E6081",
	star700: "#5E7091", star800: "#74849F", star900: "#7E8DA6",
	star950: "#93A0B6", starDim: "#C4CEDC",

	gold500: "#E3B341", gold300: "#96690C", gold200: "#C7982E", goldInk: "#06122A",
	starlight500: "#2563EB", starlight300: "#2E6BE0", starlight200: "#3E5E8C",

	nebulaViolet: "#7C3AED", nebulaVioletDeep: "#6D28D9", ionCyan: "#0E7490",
	pulsar500: "#10B981", pulsar300: "#0F7A4E",
	amber500: "#B4740A", signalRed: "#EF4444", signalRed300: "#DC2626",

	hairline:       rgba{16, 27, 46, 0.10},
	hairlineStrong: rgba{16, 27, 46, 0.16},
	hairlineFaint:  rgba{16, 27, 46, 0.06},

	category: [8]string{
		"#3B82F6", "#8B5CF6", "#EC4899", "#F59E0B",
		"#10B981", "#06B6D4", "#EF4444", "#6B7280",
	},

	// Light: pure white (231) card sits above a near-white canvas (255); the
	// deepest panel and raised chips are faint greys. ANSI: white shell, plain
	// black borders so hairlines read on 16-colour paper terminals.
	fbVoid:          fallback{"254", "7"},
	fbCanvas:        fallback{"255", "15"},
	fbSurface:       fallback{"231", "15"},
	fbTile:          fallback{"254", "7"},
	fbRaised:        fallback{"253", "7"},
	fbHairline:      fallback{"252", "7"},
	fbBorderDefault: fallback{"251", "8"},
	fbBorderStrong:  fallback{"249", "8"},
	fbBorderFaint:   fallback{"253", "7"},
}

// flattenOver composites an alpha colour against an opaque backdrop, returning
// an opaque #RRGGBB. Atlas expresses every border and tint as rgba(); the
// terminal cannot blend, so we resolve them once against the surface they sit
// on.
func flattenOver(fg rgba, backdrop string) string {
	br, bg, bb := parseHex(backdrop)

	blend := func(f, b uint8) uint8 {
		return uint8(math.Round(float64(f)*fg.A + float64(b)*(1-fg.A)))
	}
	return fmt.Sprintf("#%02X%02X%02X",
		blend(fg.R, br), blend(fg.G, bg), blend(fg.B, bb))
}

// parseHex splits a #RRGGBB string into components. Input is always a literal
// from the ports above, so a malformed value is a programming error and yields
// black rather than an error return.
func parseHex(s string) (r, g, b uint8) {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil {
		return 0, 0, 0
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v)
}
