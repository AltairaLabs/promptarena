package flow

import (
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
