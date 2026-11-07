package deploy

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLocker_LockAndUnlock(t *testing.T) {
	dir := t.TempDir()
	l := NewEnvLocker(dir, "default")

	if err := l.Lock(); err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}

	// Lock file should exist.
	if _, err := os.Stat(l.lockPath()); err != nil {
		t.Fatalf("lock file does not exist after Lock(): %v", err)
	}

	if err := l.Unlock(); err != nil {
		t.Fatalf("Unlock() failed: %v", err)
	}

	// The lock file is deliberately left in place: unlinking a flock'd file
	// lets a second process lock a fresh inode while the first still runs.
	if _, err := os.Stat(l.lockPath()); err != nil {
		t.Fatalf("lock file should survive Unlock(): %v", err)
	}

	// And a leftover file never blocks the next locker.
	l2 := NewEnvLocker(dir, "default")
	if err := l2.Lock(); err != nil {
		t.Fatalf("a leftover lock file must not block a new lock: %v", err)
	}
	_ = l2.Unlock()
}

// TestLocker_ContentionNamesHolderAndEnv pins the contention message: it
// names the environment and the holding pid, and does not tell the user to
// delete the lock file.
func TestLocker_ContentionNamesHolderAndEnv(t *testing.T) {
	dir := t.TempDir()
	l1 := NewEnvLocker(dir, "production")
	if err := l1.Lock(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l1.Unlock() }()

	err := NewEnvLocker(dir, "production").Lock()
	if err == nil {
		t.Fatal("expected contention")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"production"`) {
		t.Errorf("message should name the environment: %q", msg)
	}
	if !strings.Contains(msg, "pid "+strconv.Itoa(os.Getpid())) {
		t.Errorf("message should name the holder pid: %q", msg)
	}
	if strings.Contains(msg, "remove the lock file") {
		t.Errorf("message must not advise deleting the lock file: %q", msg)
	}
}

// TestLocker_EnvironmentsAreIndependent pins per-environment locking.
func TestLocker_EnvironmentsAreIndependent(t *testing.T) {
	dir := t.TempDir()
	prod := NewEnvLocker(dir, "production")
	if err := prod.Lock(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = prod.Unlock() }()
	staging := NewEnvLocker(dir, "staging")
	if err := staging.Lock(); err != nil {
		t.Fatalf("staging must lock while production is held: %v", err)
	}
	_ = staging.Unlock()
}

// TestLocker_RejectsInvalidEnvironment pins that an env name cannot become
// a path outside .promptarena/envs.
func TestLocker_RejectsInvalidEnvironment(t *testing.T) {
	for _, env := range []string{"", "../x", "a/b", ".hidden"} {
		if err := NewEnvLocker(t.TempDir(), env).Lock(); err == nil {
			t.Errorf("env %q should be rejected", env)
		}
	}
}

func TestLocker_DoubleLock(t *testing.T) {
	dir := t.TempDir()
	l := NewEnvLocker(dir, "default")

	if err := l.Lock(); err != nil {
		t.Fatalf("first Lock() failed: %v", err)
	}
	defer func() { _ = l.Unlock() }()

	err := l.Lock()
	if err == nil {
		t.Fatal("second Lock() should have returned an error")
	}
	if !strings.Contains(err.Error(), "already holds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocker_ConcurrentLock(t *testing.T) {
	dir := t.TempDir()

	l1 := NewEnvLocker(dir, "default")
	l2 := NewEnvLocker(dir, "default")

	if err := l1.Lock(); err != nil {
		t.Fatalf("l1.Lock() failed: %v", err)
	}
	defer func() { _ = l1.Unlock() }()

	err := l2.Lock()
	if err == nil {
		_ = l2.Unlock()
		t.Fatal("l2.Lock() should have returned an error while l1 holds the lock")
	}
	if !strings.Contains(err.Error(), "held by another process") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocker_UnlockWithoutLock(t *testing.T) {
	dir := t.TempDir()
	l := NewEnvLocker(dir, "default")

	if err := l.Unlock(); err != nil {
		t.Fatalf("Unlock() without Lock() should be a no-op, got: %v", err)
	}
}

func TestLocker_LockCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".promptarena")

	// Ensure the state directory does not exist yet.
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf(".promptarena dir should not exist yet")
	}

	l := NewEnvLocker(dir, "default")
	if err := l.Lock(); err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}
	defer func() { _ = l.Unlock() }()

	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf(".promptarena dir was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal(".promptarena should be a directory")
	}
}
