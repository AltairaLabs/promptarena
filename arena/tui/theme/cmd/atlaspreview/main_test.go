package main

import (
	"strings"
	"testing"

	"github.com/AltairaLabs/promptarena/arena/tui/theme"
)

// TestRenderProducesContent exercises the whole preview render path (header,
// swatches, panel, statuses) for both themes.
func TestRenderProducesContent(t *testing.T) {
	for _, th := range []theme.Theme{theme.Dark(), theme.Light()} {
		out := render(th)
		if strings.TrimSpace(out) == "" {
			t.Fatalf("render(%s) produced no output", th.Name)
		}
		for _, want := range []string{"atlas", "promptarena"} {
			if !strings.Contains(out, want) {
				t.Errorf("render(%s) missing %q", th.Name, want)
			}
		}
	}
}
