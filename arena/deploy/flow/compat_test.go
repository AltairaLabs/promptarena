package flow

import "testing"

// TestLock_LocksTheDefaultEnvironment pins the v2.4.0 one-argument Lock onto
// the environment deploy commands use without --env.
func TestLock_LocksTheDefaultEnvironment(t *testing.T) {
	dir := t.TempDir()
	release, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := LockEnv(dir, DefaultEnv); err == nil {
		t.Fatal("Lock must hold the default environment's lock")
	}
}
