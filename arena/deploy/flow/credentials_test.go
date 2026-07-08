package flow

import (
	"os"
	"path/filepath"
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
