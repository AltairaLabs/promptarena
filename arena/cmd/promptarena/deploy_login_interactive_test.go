package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveLoginProvider covers the thin CLI wrapper's provider resolution
// (flag vs. config fallback). The login flow itself (capability check, loopback
// server, browser hooks, profile merge) moved to arena/deploy/flow and is
// covered there — see arena/deploy/flow/login_test.go.
func TestResolveLoginProvider(t *testing.T) {
	orig := deployLoginProvider
	defer func() { deployLoginProvider = orig }()

	deployLoginProvider = "omnia"
	if p, err := resolveLoginProvider(); err != nil || p != "omnia" {
		t.Errorf("flag path: (%q, %v), want (omnia, nil)", p, err)
	}

	deployLoginProvider = ""
	origCfg := deployConfig
	deployConfig = filepath.Join(t.TempDir(), "nope.yaml")
	defer func() { deployConfig = origCfg }()
	if _, err := resolveLoginProvider(); err == nil {
		t.Error("expected error with no flag and no config")
	}

	// No flag, but the config has a deploy.provider → use it.
	cfgPath := filepath.Join(t.TempDir(), "arena.yaml")
	cfg := "apiVersion: promptkit.altairalabs.ai/v1alpha1\nkind: Arena\n" +
		"metadata:\n  name: t\nspec:\n  prompt_configs: []\n  providers: []\n" +
		"  defaults:\n    temperature: 0.7\n    max_tokens: 100\n" +
		"  deploy:\n    provider: omnia\n    config:\n      api_endpoint: https://x\n      workspace: demo\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	deployConfig = cfgPath
	if p, err := resolveLoginProvider(); err != nil || p != "omnia" {
		t.Errorf("config path: (%q, %v), want (omnia, nil)", p, err)
	}
}
