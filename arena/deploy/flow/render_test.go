package flow

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

func TestActionSymbol(t *testing.T) {
	cases := map[deploy.Action]string{
		deploy.ActionCreate:   "+",
		deploy.ActionUpdate:   "~",
		deploy.ActionDelete:   "-",
		deploy.ActionNoChange: " ",
		deploy.ActionDrift:    "!",
	}
	for action, want := range cases {
		if got := ActionSymbol(action); got != want {
			t.Errorf("ActionSymbol(%q) = %q, want %q", action, got, want)
		}
	}
}

func TestStatusSymbol(t *testing.T) {
	cases := map[string]string{"created": "+", "updated": "~", "deleted": "-", "failed": "!", "other": " "}
	for status, want := range cases {
		if got := StatusSymbol(status); got != want {
			t.Errorf("StatusSymbol(%q) = %q, want %q", status, got, want)
		}
	}
}
