package deploy

import "testing"

// TestNewLocker_LocksTheDefaultEnvironment pins the v2.4.0 one-argument
// constructor onto the environment deploy commands use without --env.
func TestNewLocker_LocksTheDefaultEnvironment(t *testing.T) {
	dir := t.TempDir()
	l := NewLocker(dir)
	if err := l.Lock(); err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}
	defer func() { _ = l.Unlock() }()

	if err := NewEnvLocker(dir, defaultEnvironment).Lock(); err == nil {
		t.Fatal("NewLocker must hold the default environment's lock")
	}
}

// TestNewStateStore_ScopesToTheDefaultEnvironment pins the v2.4.0
// one-argument constructor onto the default environment's state.
func TestNewStateStore_ScopesToTheDefaultEnvironment(t *testing.T) {
	dir := t.TempDir()
	s := NewStateStore(dir)
	if got := s.Environment(); got != defaultEnvironment {
		t.Fatalf("Environment() = %q, want %q", got, defaultEnvironment)
	}
	if err := s.Save(&State{Provider: "omnia", Environment: defaultEnvironment}); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}
	st, err := NewEnvStateStore(dir, defaultEnvironment).Load()
	if err != nil || st == nil || st.Provider != "omnia" {
		t.Fatalf("default environment state = %+v, %v; want the saved state", st, err)
	}
}
