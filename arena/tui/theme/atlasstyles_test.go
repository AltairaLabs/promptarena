package theme

import (
	"strings"
	"testing"
)

func TestStylesDeriveColoursFromTheirTheme(t *testing.T) {
	s := NewStyles(Dark())

	if got := HexOf(s.Body.GetForeground()); got != "#E6EDF8" {
		t.Errorf("Body foreground = %v, want --star-300 #E6EDF8", got)
	}
	if got := HexOf(s.Muted.GetForeground()); got != "#8195B0" {
		t.Errorf("Muted foreground = %v, want --star-700 #8195B0", got)
	}
}

func TestInfoAndValueStylesMapToTheirTokens(t *testing.T) {
	s := NewStyles(Dark())

	// Info is the neutral interactive accent (starlight); Value is bright
	// emphasized data (heading star, bold). These back the legacy InfoStyle /
	// ValueStyle during the migration.
	if got := HexOf(s.Info.GetForeground()); got != "#3B82F6" {
		t.Errorf("Info foreground = %v, want --starlight-500 #3B82F6", got)
	}
	if got := HexOf(s.Value.GetForeground()); got != "#F2F6FC" {
		t.Errorf("Value foreground = %v, want --star-200 #F2F6FC", got)
	}
	if !s.Value.GetBold() {
		t.Error("Value should be bold — it emphasizes data")
	}
}

func TestCardUsesFlattenedHairlineBorder(t *testing.T) {
	s := NewStyles(Dark())

	if got := HexOf(s.Card.GetBorderTopForeground()); got != "#182336" {
		t.Errorf("Card border color = %v, want flattened hairline #182336", got)
	}
	if !s.Card.GetBorderTop() {
		t.Error("Card should have a border — the hairline defines the panel in Atlas")
	}
}

// Atlas labels the corner of every panel with an "eyebrow": uppercase mono at
// 0.14em tracking. Letter-spacing has no direct terminal equivalent, so the
// port renders tracking as interleaved spaces.
func TestEyebrowUppercasesAndTracksText(t *testing.T) {
	got := Eyebrow("Active runs")

	if !strings.Contains(got, "A C T I V E") {
		t.Errorf("Eyebrow(%q) = %q, want uppercase letter-spaced text", "Active runs", got)
	}
}

func TestEyebrowPreservesWordBoundaries(t *testing.T) {
	got := Eyebrow("Active runs")

	// Two words must remain visually distinct, not merge into one run.
	if !strings.Contains(got, "  R U N S") {
		t.Errorf("Eyebrow(%q) = %q, want a wider gap between words", "Active runs", got)
	}
}

func TestStylesAreThemeSwappable(t *testing.T) {
	dark, light := NewStyles(Dark()), NewStyles(Light())

	if dark.Body.GetForeground() == light.Body.GetForeground() {
		t.Error("Body should differ between themes; styles are not reading their theme")
	}
}
