package main

import (
	"bytes"

	"strings"
	"testing"

	"github.com/muesli/termenv"
)

// TestRun_SelectsThemesByFlag covers the tool's entire behavior: which themes
// it renders for a given flag combination. atlaspreview exists so a human can
// eyeball both palettes side by side, so rendering the wrong one — or only
// one when both were asked for — defeats the point without erroring.
func TestRun_SelectsThemesByFlag(t *testing.T) {
	renderOf := func(light, dark bool) string {
		var buf bytes.Buffer
		run(&buf, light, dark)
		return buf.String()
	}

	both := renderOf(false, false)
	lightOnly := renderOf(true, false)
	darkOnly := renderOf(false, true)

	for name, out := range map[string]string{
		"both": both, "light": lightOnly, "dark": darkOnly,
	} {
		if strings.TrimSpace(out) == "" {
			t.Fatalf("%s rendered nothing", name)
		}
	}

	// The default renders two previews; each flag renders one. Comparing
	// lengths is what distinguishes them without pinning the palette itself,
	// which is the thing this tool exists to let a human change.
	if len(both) <= len(lightOnly) {
		t.Errorf("the default must render both themes: both=%d light=%d",
			len(both), len(lightOnly))
	}
	if len(both) <= len(darkOnly) {
		t.Errorf("the default must render both themes: both=%d dark=%d",
			len(both), len(darkOnly))
	}

	// -light and -dark must not produce the same output, or the flag does
	// nothing and the tool silently previews one theme twice.
	if lightOnly == darkOnly {
		t.Error("-light and -dark rendered identically; the flag is not selecting a theme")
	}

	// -light wins when both are passed, matching the switch order.
	if got := renderOf(true, true); got != lightOnly {
		t.Error("-light must take precedence when both flags are given")
	}
}

// TestProfileFor_P256SelectsTheDegradedProfile pins the -p256 flag. It exists
// so a reviewer can see how the palette degrades at 256 colors before
// shipping it; if the flag silently kept truecolor the preview would look
// perfect and the degradation would ship unseen.
func TestProfileFor_P256SelectsTheDegradedProfile(t *testing.T) {
	if got := profileFor(false); got != termenv.TrueColor {
		t.Errorf("default profile = %v, want TrueColor so the full ramp shows", got)
	}
	if got := profileFor(true); got != termenv.ANSI256 {
		t.Errorf("-p256 profile = %v, want ANSI256", got)
	}
}
