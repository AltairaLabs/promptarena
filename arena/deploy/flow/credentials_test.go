package flow

import (
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
