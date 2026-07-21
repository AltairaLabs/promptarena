package console

import (
	"strings"
	"testing"
)

func TestBannerHasMarkAndTitle(t *testing.T) {
	got := plain(Banner("PromptArena"))
	if !strings.Contains(got, "✦") {
		t.Errorf("Banner() = %q, want the sparkle mark", got)
	}
	if !strings.Contains(got, "PromptArena") {
		t.Errorf("Banner() = %q, want the title", got)
	}
}

func TestFieldRendersLabelAndValue(t *testing.T) {
	got := plain(Field("Config", "config.arena.yaml"))
	if !strings.Contains(got, "Config") || !strings.Contains(got, "config.arena.yaml") {
		t.Errorf("Field() = %q, want both label and value", got)
	}
	if !strings.Contains(got, ":") {
		t.Errorf("Field() = %q, want a colon after the label", got)
	}
}

func TestNoteRendersText(t *testing.T) {
	got := plain(Note("Starting execution…"))
	if !strings.Contains(got, "Starting execution…") {
		t.Errorf("Note() = %q, want the text", got)
	}
}

func TestEmphasisRendersText(t *testing.T) {
	got := plain(Emphasis("18"))
	if !strings.Contains(got, "18") {
		t.Errorf("Emphasis() = %q, want the value", got)
	}
}

func TestCountfComposesSentence(t *testing.T) {
	got := plain(Countf("Generated", 12, "run combinations"))
	for _, want := range []string{"Generated", "12", "run combinations"} {
		if !strings.Contains(got, want) {
			t.Errorf("Countf() = %q, want it to contain %q", got, want)
		}
	}
}
