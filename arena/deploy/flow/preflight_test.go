package flow

import (
	"context"
	"testing"
)

func TestCheckPreflight_ConfigMissing(t *testing.T) {
	pf := CheckPreflight(context.Background(), Options{ProjectDir: t.TempDir(), ConfigPath: t.TempDir() + "/nope.yaml"})
	if pf.ConfigErr == nil {
		t.Fatal("expected ConfigErr for missing config")
	}
	if pf.Ready() {
		t.Fatal("Ready() must be false when config is missing")
	}
}

// TestCheckPreflight_AdapterMissing exercises the branch where config loads
// fine but the provider's adapter binary cannot be discovered: CheckPreflight
// should stop right after AdapterInstalled (no probe attempted) and report a
// not-ready, install-guided snapshot.
func TestCheckPreflight_AdapterMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()
	configPath := writeArenaConfig(t, dir, `  providers: []
  provider_specs:
    p1:
      type: openai
      model: gpt-4
  defaults:
    concurrency: 1
  deploy:
    provider: nonexistent-xyz
    config: {}
`)

	pf := CheckPreflight(context.Background(), Options{ConfigPath: configPath, ProjectDir: dir})

	if pf.ConfigErr != nil {
		t.Fatalf("unexpected ConfigErr: %v", pf.ConfigErr)
	}
	if pf.Provider != "nonexistent-xyz" {
		t.Fatalf("Provider = %q, want nonexistent-xyz", pf.Provider)
	}
	if pf.InstallCommand != InstallCommand("nonexistent-xyz") {
		t.Fatalf("InstallCommand = %q, want %q", pf.InstallCommand, InstallCommand("nonexistent-xyz"))
	}
	if pf.AdapterFound {
		t.Fatal("expected AdapterFound=false for a provider with no installed adapter")
	}
	if pf.ProbeErr != nil {
		t.Fatalf("ProbeErr should be nil — CheckPreflight must not attempt to probe a missing adapter, got %v", pf.ProbeErr)
	}
	if pf.AdapterVersion != "" || len(pf.Capabilities) != 0 {
		t.Fatalf("expected no version/capabilities to be populated, got version=%q caps=%v", pf.AdapterVersion, pf.Capabilities)
	}
	if pf.Ready() {
		t.Fatal("Ready() must be false when the adapter is missing")
	}
}

// TestConfigHasToken exercises the small JSON-sniffing helper directly: a
// merged config JSON with a non-empty api_token means Authenticated should be
// derivable as true; an empty or missing token, or malformed JSON, means false.
func TestConfigHasToken(t *testing.T) {
	cases := []struct {
		name string
		json string
		want bool
	}{
		{"has token", `{"api_token":"tok-123","region":"us"}`, true},
		{"empty token", `{"api_token":"","region":"us"}`, false},
		{"missing token", `{"region":"us"}`, false},
		{"malformed json", `not-json`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := configHasToken(tc.json); got != tc.want {
				t.Errorf("configHasToken(%q) = %v, want %v", tc.json, got, tc.want)
			}
		})
	}
}
