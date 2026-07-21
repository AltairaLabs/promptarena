package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// flattenOver composites an alpha color against an opaque backdrop.
// Terminals have no alpha channel, so every Atlas rgba() token has to be
// resolved to a solid color at construction time.
func TestFlattenOverCompositesAlphaAgainstBackdrop(t *testing.T) {
	// Atlas --hairline: rgba(147,197,253,0.10) over --ink-canvas #0A1120.
	got := flattenOver(rgba{147, 197, 253, 0.10}, "#0A1120")

	if got != "#182336" {
		t.Errorf("flattenOver(hairline, ink-canvas) = %q, want %q", got, "#182336")
	}
}

func TestFlattenOverIsIdentityAtFullAlpha(t *testing.T) {
	got := flattenOver(rgba{227, 179, 65, 1.0}, "#0A1120")

	if got != "#E3B341" {
		t.Errorf("flattenOver at alpha 1.0 = %q, want %q", got, "#E3B341")
	}
}

func TestDarkResolvesSemanticAliasesToRawRamp(t *testing.T) {
	th := Dark()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"BgApp -> --ink-canvas", HexOf(th.BgApp), "#0A1120"},
		{"SurfaceCode -> --ink-void", HexOf(th.SurfaceCode), "#070C16"},
		{"TextBody -> --star-300", HexOf(th.TextBody), "#E6EDF8"},
		{"TextMuted -> --star-700", HexOf(th.TextMuted), "#8195B0"},
		{"AccentPrimary -> --gold-500", HexOf(th.AccentPrimary), "#E3B341"},
		{"AccentInter -> --starlight-500", HexOf(th.AccentInter), "#3B82F6"},
		{"StatusError -> --signal-red", HexOf(th.StatusError), "#EF4444"},
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestLightRepointsRampSoTextInverts(t *testing.T) {
	dark, light := Dark(), Light()

	// In dark the body text is near-white; in light it must be ink-navy.
	if HexOf(dark.TextBody) != "#E6EDF8" {
		t.Fatalf("dark TextBody = %q, want #E6EDF8", dark.TextBody)
	}
	if HexOf(light.TextBody) != "#1E2C44" {
		t.Errorf("light TextBody = %q, want #1E2C44", light.TextBody)
	}

	// Gold FILLS stay identical across themes; gold TEXT deepens.
	if light.AccentPrimary != dark.AccentPrimary {
		t.Errorf("AccentPrimary should be theme-invariant, got dark=%q light=%q",
			dark.AccentPrimary, light.AccentPrimary)
	}
	if light.GoldText == dark.GoldText {
		t.Errorf("GoldText should deepen on light, both are %q", light.GoldText)
	}
}

func TestLightSurfacesStayDistinctWithoutShadow(t *testing.T) {
	light := Light()

	// Atlas light sets --ink-surface and --ink-raised both to #FFFFFF; on the
	// web a shadow separates them. Terminals have no shadow, so a raised panel
	// would vanish against a card. The TUI deviates by giving raised surfaces
	// Atlas's own tile surface, keeping them visibly distinct.
	if light.Surface1 == light.Surface2 {
		t.Errorf("light Surface1 == Surface2 (%q); raised surfaces need a distinct value in the TUI", light.Surface1)
	}
}

func TestDarkSurfacesUnaffectedByLightDeviation(t *testing.T) {
	dark := Dark()

	// The light-only fix must not perturb dark, where the ramp already
	// separates surface (#0C1526) from raised (#111C2E).
	if HexOf(dark.Surface1) != "#0C1526" {
		t.Errorf("dark Surface1 = %q, want #0C1526", dark.Surface1)
	}
	if HexOf(dark.Surface2) != "#111C2E" {
		t.Errorf("dark Surface2 = %q, want #111C2E", dark.Surface2)
	}
}

func TestBordersAreFlattenedToOpaqueColours(t *testing.T) {
	th := Dark()

	// --border-default is rgba(147,197,253,0.10) over the app background.
	if HexOf(th.BorderDefault) != "#182336" {
		t.Errorf("dark BorderDefault = %q, want %q", th.BorderDefault, "#182336")
	}

	// Whatever the theme, a border must never carry an alpha suffix.
	if len(HexOf(Light().BorderDefault)) != 7 {
		t.Errorf("light BorderDefault = %q, want a 7-char #RRGGBB", Light().BorderDefault)
	}
}

func TestSurfacesDoNotCollapseOn256Colour(t *testing.T) {
	// The four stacked surfaces are all near-navy near-black; termenv's nearest
	// match maps every one to 256-index 232, which would make cards and panels
	// indistinguishable. They must be pinned (CompleteColor) to DISTINCT 256
	// indices. This guards the exact regression that motivated the pin layer.
	for _, th := range []Theme{Dark(), Light()} {
		surfaces := map[string]lipgloss.TerminalColor{
			"SurfaceCode": th.SurfaceCode,
			"BgApp":       th.BgApp,
			"Surface1":    th.Surface1,
			"Surface2":    th.Surface2,
		}
		seen := map[string]string{}
		for name, s := range surfaces {
			cc, ok := s.(lipgloss.CompleteColor)
			if !ok {
				t.Errorf("%s %s is not pinned (got %T); it will auto-degrade and collapse", th.Name, name, s)
				continue
			}
			if cc.ANSI256 == "" {
				t.Errorf("%s %s has no ANSI256 pin", th.Name, name)
			}
			if prev, dup := seen[cc.ANSI256]; dup {
				t.Errorf("%s %s and %s share ANSI256 %q — they collapse on 256-color terminals",
					th.Name, name, prev, cc.ANSI256)
			}
			seen[cc.ANSI256] = name
		}
	}
}

func TestCategoricalPaletteHasEightOrderedSlots(t *testing.T) {
	th := Dark()

	if len(th.Category) != 8 {
		t.Fatalf("len(Category) = %d, want 8", len(th.Category))
	}
	if HexOf(th.Category[0]) != "#60A5FA" {
		t.Errorf("Category[0] = %q, want #60A5FA", th.Category[0])
	}
	if HexOf(th.Category[7]) != "#9CA3AF" {
		t.Errorf("Category[7] (neutral/other) = %q, want #9CA3AF", th.Category[7])
	}
}
