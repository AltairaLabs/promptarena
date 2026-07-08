package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredential_StoreAndLookup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := filepath.Join(t.TempDir(), "arena.yaml")

	if _, ok := LookupCredential("omnia", cfg); ok {
		t.Fatal("expected no credential before store")
	}
	if err := StoreCredential("omnia", cfg, Credential{Token: "tok-123", Endpoint: "https://e"}); err != nil {
		t.Fatal(err)
	}
	tok, ok := LookupCredential("omnia", cfg)
	if !ok || tok != "tok-123" {
		t.Fatalf("LookupCredential = %q,%v; want tok-123,true", tok, ok)
	}
	// Different provider/config → miss.
	if _, ok := LookupCredential("agentcore", cfg); ok {
		t.Fatal("expected miss for different provider")
	}
}

// TestCredential_KeyDeterminism verifies the on-disk credential key scheme
// (provider + "|" + filepath.Abs(configPath)) is the stable compatibility
// contract with the CLI: the same (provider, configPath) resolves to the
// same stored credential regardless of which equivalent form of configPath
// is passed (relative vs. its absolute form), while a different provider or
// a different config path never collides with it.
func TestCredential_KeyDeterminism(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	abs := filepath.Join(dir, "arena.yaml")

	rel, err := filepath.Rel(mustGetwd(t), abs)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}

	if err := StoreCredential("omnia", abs, Credential{Token: "tok-abs"}); err != nil {
		t.Fatalf("StoreCredential(abs): %v", err)
	}

	// The same path, passed relative instead of absolute, must resolve to the
	// same stored credential (both normalize to the same filepath.Abs key).
	tok, ok := LookupCredential("omnia", rel)
	if !ok || tok != "tok-abs" {
		t.Fatalf("LookupCredential(rel) = %q,%v; want tok-abs,true (relative and absolute forms must map to the same key)", tok, ok)
	}

	// Storing via the relative form must overwrite the same entry, not create
	// a second one.
	if err := StoreCredential("omnia", rel, Credential{Token: "tok-rel"}); err != nil {
		t.Fatalf("StoreCredential(rel): %v", err)
	}
	if tok, _ := LookupCredential("omnia", abs); tok != "tok-rel" {
		t.Fatalf("LookupCredential(abs) after StoreCredential(rel) = %q; want tok-rel (relative store must overwrite the same key)", tok)
	}

	// A different provider for the same config path must not collide.
	if _, ok := LookupCredential("agentcore", abs); ok {
		t.Fatal("expected miss for a different provider at the same config path")
	}

	// A different config path for the same provider must not collide.
	otherCfg := filepath.Join(dir, "other.yaml")
	if _, ok := LookupCredential("omnia", otherCfg); ok {
		t.Fatal("expected miss for a different config path")
	}
}

// TestLookupCredential_EmptyTokenTreatedAsMiss covers the branch where a
// credential entry exists for the key but its Token is empty — LookupCredential
// must treat that the same as no credential at all (ok=false), not return the
// blank string as if it were valid.
func TestLookupCredential_EmptyTokenTreatedAsMiss(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := filepath.Join(t.TempDir(), "arena.yaml")

	if err := StoreCredential("omnia", cfg, Credential{Token: ""}); err != nil {
		t.Fatalf("StoreCredential: %v", err)
	}
	if tok, ok := LookupCredential("omnia", cfg); ok || tok != "" {
		t.Fatalf("LookupCredential = %q,%v; want empty,false for a stored empty token", tok, ok)
	}
}

// TestLookupCredential_PropagatesLoadError covers the branch where the
// credentials file is corrupt: LookupCredential must fail closed (ok=false),
// not panic or return a stale/garbage token.
func TestLookupCredential_PropagatesLoadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("seed corrupt credentials file: %v", err)
	}

	if tok, ok := LookupCredential("omnia", "arena.yaml"); ok || tok != "" {
		t.Fatalf("LookupCredential = %q,%v; want empty,false when the credentials file is corrupt", tok, ok)
	}
}

// TestCredentialsPath_NoHome covers the os.UserHomeDir failure branch: with
// $HOME unset, CredentialsPath (and everything built on it) must surface a
// clear error instead of resolving to a bogus path.
func TestCredentialsPath_NoHome(t *testing.T) {
	t.Setenv("HOME", "")

	if _, err := CredentialsPath(); err == nil || !strings.Contains(err.Error(), "cannot determine home directory") {
		t.Fatalf("CredentialsPath error = %v, want it to mention the missing home directory", err)
	}
	if _, err := LoadCredentials(); err == nil {
		t.Fatal("LoadCredentials should propagate the CredentialsPath error")
	}
	if err := StoreCredential("omnia", "arena.yaml", Credential{Token: "tok"}); err == nil {
		t.Fatal("StoreCredential should propagate the CredentialsPath error")
	}
}

// TestStoreCredential_MkdirFails covers the branch where the credentials
// directory cannot be created: HOME exists but is read-only, so
// os.MkdirAll(HOME/.promptarena, ...) fails with permission denied while
// LoadCredentials still succeeds (the file simply doesn't exist yet — ENOENT,
// not a permission error, since only *reading* HOME's directory entries is
// needed to observe that).
func TestStoreCredential_MkdirFails(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "home")
	if err := os.Mkdir(home, 0o500); err != nil {
		t.Fatalf("mkdir read-only home: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0o700) }) // let TempDir cleanup remove it
	t.Setenv("HOME", home)

	// Confirm the sandbox actually enforces the permission (not running as
	// root, not on a filesystem that ignores mode bits); skip otherwise.
	if err := os.Mkdir(filepath.Join(home, "probe"), 0o700); err == nil {
		t.Skip("read-only permission not enforced in this environment (e.g. running as root); skipping")
	}

	err := StoreCredential("omnia", "arena.yaml", Credential{Token: "tok"})
	if err == nil {
		t.Fatal("expected error when the credentials directory cannot be created")
	}
	if !strings.Contains(err.Error(), "failed to create credentials directory") {
		t.Fatalf("error = %q, want it to mention the mkdir failure", err.Error())
	}
}

// TestStoreCredential_PropagatesLoadError covers the branch where an existing
// (corrupt) credentials file makes the initial LoadCredentials fail; the
// error should propagate rather than silently overwriting the file.
func TestStoreCredential_PropagatesLoadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("seed corrupt credentials file: %v", err)
	}

	if err := StoreCredential("omnia", "arena.yaml", Credential{Token: "tok"}); err == nil {
		t.Fatal("expected StoreCredential to propagate the LoadCredentials parse error")
	}
}

// mustGetwd returns the current working directory, failing the test if it
// cannot be determined (needed to build a relative-path form of an absolute
// path for TestCredential_KeyDeterminism).
func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	return wd
}
