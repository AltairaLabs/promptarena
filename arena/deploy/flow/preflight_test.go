package flow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/promptarena/v2/deploy"
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

// TestCheckPreflight_ProjectDirUnresolvable covers the second ConfigErr exit,
// which is reached after the config parses. Preflight's whole job is to say
// why a deploy cannot proceed before anything is created, so an unresolvable
// project directory has to surface as a reported reason rather than a later
// failure part-way through an apply.
func TestCheckPreflight_ProjectDirUnresolvable(t *testing.T) {
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

	// A project dir that is a file, not a directory: opts.dir() resolves it
	// but AdapterInstalled cannot search it.
	notADir := filepath.Join(dir, "regular-file")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	pf := CheckPreflight(context.Background(), Options{ConfigPath: configPath, ProjectDir: notADir})

	if pf.Ready() {
		t.Fatal("Ready() must be false when the adapter cannot be located")
	}
	if pf.AdapterFound {
		t.Error("no adapter can be found under a non-directory")
	}
	// The install hint is the actionable part of a not-ready preflight.
	if pf.InstallCommand == "" {
		t.Error("a missing adapter must come with an install command")
	}
}

// probeStub is an adapterProbe that returns canned provider info.
type probeStub struct {
	info     *deploy.ProviderInfo
	err      error
	closed   bool
	connectE error
}

func (p *probeStub) GetProviderInfo(context.Context) (*deploy.ProviderInfo, error) {
	return p.info, p.err
}
func (p *probeStub) Close() error { p.closed = true; return nil }

// withProbe swaps the adapter probe for the duration of a test.
func withProbe(t *testing.T, stub *probeStub) {
	t.Helper()
	saved := probeAdapter
	probeAdapter = func(context.Context, string, string) (adapterProbe, error) {
		if stub.connectE != nil {
			return nil, stub.connectE
		}
		return stub, nil
	}
	t.Cleanup(func() { probeAdapter = saved })
}

func preflightConfig(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	// A real adapter file so AdapterInstalled finds one and the probe runs.
	adapterDir := filepath.Join(dir, ".promptarena", "adapters")
	if err := os.MkdirAll(adapterDir, 0o750); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(adapterDir, "promptarena-deploy-acme")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	configPath := writeArenaConfig(t, dir, `  providers: []
  provider_specs:
    p1:
      type: openai
      model: gpt-4
  defaults:
    concurrency: 1
  deploy:
    provider: acme
    config: {}
`)
	return dir, configPath
}

// TestCheckPreflight_ProbeReportsCapabilities covers the probe half of
// CheckPreflight, which was unreachable without installing an adapter.
// Capabilities decide what the CLI offers: miss the login capability and
// `deploy login` disappears from a provider that supports it, with no error
// to explain why.
func TestCheckPreflight_ProbeReportsCapabilities(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir, configPath := preflightConfig(t)

	stub := &probeStub{info: &deploy.ProviderInfo{
		Version:      "9.9.9",
		Capabilities: []string{"plan", deploy.LoginCapability},
	}}
	withProbe(t, stub)

	pf := CheckPreflight(context.Background(), Options{ConfigPath: configPath, ProjectDir: dir})

	if pf.ProbeErr != nil {
		t.Fatalf("unexpected ProbeErr: %v", pf.ProbeErr)
	}
	if pf.AdapterVersion != "9.9.9" {
		t.Errorf("AdapterVersion = %q", pf.AdapterVersion)
	}
	if !pf.SupportsLogin {
		t.Error("the login capability must set SupportsLogin")
	}
	if len(pf.Capabilities) != 2 {
		t.Errorf("Capabilities = %v", pf.Capabilities)
	}
	if !stub.closed {
		t.Error("the probe connection must be closed")
	}
}

// TestCheckPreflight_WithoutLoginCapability pins the negative: an adapter that
// does not advertise login must not have the CLI offering it.
func TestCheckPreflight_WithoutLoginCapability(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir, configPath := preflightConfig(t)

	withProbe(t, &probeStub{info: &deploy.ProviderInfo{Version: "1.0", Capabilities: []string{"plan"}}})

	pf := CheckPreflight(context.Background(), Options{ConfigPath: configPath, ProjectDir: dir})
	if pf.SupportsLogin {
		t.Error("SupportsLogin must be false when the adapter does not advertise it")
	}
}

// TestCheckPreflight_ProbeFailureIsReportedNotFatal pins that an adapter which
// is installed but will not answer produces a ProbeErr — a not-ready snapshot
// naming the reason, rather than a panic or a silently "ready" result.
func TestCheckPreflight_ProbeFailureIsReportedNotFatal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir, configPath := preflightConfig(t)

	withProbe(t, &probeStub{connectE: errors.New("adapter refused the handshake")})

	pf := CheckPreflight(context.Background(), Options{ConfigPath: configPath, ProjectDir: dir})
	if pf.ProbeErr == nil {
		t.Fatal("a failed connection must be reported as ProbeErr")
	}
	if pf.Ready() {
		t.Error("Ready() must be false when the probe failed")
	}
}

// TestCheckPreflight_GetProviderInfoFailure covers the second probe exit:
// connected, but the adapter cannot describe itself.
func TestCheckPreflight_GetProviderInfoFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir, configPath := preflightConfig(t)

	withProbe(t, &probeStub{err: errors.New("malformed provider info")})

	pf := CheckPreflight(context.Background(), Options{ConfigPath: configPath, ProjectDir: dir})
	if pf.ProbeErr == nil {
		t.Fatal("a failed GetProviderInfo must be reported as ProbeErr")
	}
	if pf.AdapterVersion != "" {
		t.Errorf("no version may be recorded from a failed probe, got %q", pf.AdapterVersion)
	}
}
