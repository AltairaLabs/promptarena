package theme

import "testing"

func TestActiveDefaultsToDark(t *testing.T) {
	// Without any SetActive call, the process theme is Atlas dark — matching
	// the TUI's historical hardcoded-dark behavior.
	if got := Active().Theme.Name; got != "dark" {
		t.Errorf("Active().Theme.Name = %q, want %q", got, "dark")
	}
}

func TestSetActiveSwapsTheProcessTheme(t *testing.T) {
	t.Cleanup(func() { SetActive(Dark()) })

	SetActive(Light())
	if got := Active().Theme.Name; got != "light" {
		t.Errorf("after SetActive(Light), Active().Theme.Name = %q, want %q", got, "light")
	}
}

func TestChooseHonoursExplicitOverride(t *testing.T) {
	cases := []struct {
		env      string
		darkTerm bool
		want     string
	}{
		{"light", true, "light"},    // explicit override beats a dark terminal
		{"dark", false, "dark"},     // explicit override beats a light terminal
		{"", true, "dark"},          // no override: follow the terminal
		{"", false, "light"},        // no override: follow the terminal
		{"garbage", false, "light"}, // unknown override is ignored → follow the terminal
		{"garbage", true, "dark"},   // unknown override is ignored → follow the terminal
	}

	for _, tc := range cases {
		got := Choose(tc.env, tc.darkTerm).Name
		if got != tc.want {
			t.Errorf("Choose(env=%q, darkTerm=%v) = %q, want %q",
				tc.env, tc.darkTerm, got, tc.want)
		}
	}
}
