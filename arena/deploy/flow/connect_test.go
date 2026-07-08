package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCommand(t *testing.T) {
	got := InstallCommand("omnia")
	if got != "promptarena deploy adapter install omnia" {
		t.Fatalf("InstallCommand = %q", got)
	}
}

func TestAdapterInstalled_Missing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, found := AdapterInstalled("nonexistent-xyz", t.TempDir())
	if found {
		t.Fatal("expected adapter not found")
	}
}

// TestAdapterInstalled_ProjectLocal covers the discovery-succeeds branch:
// Discover never executes the candidate, only checks it exists with the
// executable bit set, so a project-local placeholder binary is enough to
// prove the project-local search tier resolves and returns its path — no
// real adapter subprocess involved.
func TestAdapterInstalled_ProjectLocal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	adapterDir := filepath.Join(dir, ".promptarena", "adapters")
	if err := os.MkdirAll(adapterDir, 0o755); err != nil {
		t.Fatalf("mkdir adapter dir: %v", err)
	}
	binPath := filepath.Join(adapterDir, "promptarena-deploy-fakeprov")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\necho fake\n"), 0o755); err != nil {
		t.Fatalf("write placeholder adapter: %v", err)
	}

	path, found := AdapterInstalled("fakeprov", dir)
	if !found {
		t.Fatal("expected the project-local placeholder adapter to be discovered")
	}
	if path != binPath {
		t.Fatalf("path = %q, want %q", path, binPath)
	}
}

func TestLock_ContentionReturnsError(t *testing.T) {
	dir := t.TempDir()
	release, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := Lock(dir); err == nil {
		t.Fatal("expected lock contention error on second Lock")
	} else if !strings.Contains(err.Error(), "lock") {
		t.Fatalf("unexpected error: %v", err)
	}
}
