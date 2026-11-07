package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStateStore_EnvironmentsAreIsolated is the regression for #407 item 4:
// state saved for production must be invisible to staging, so destroying
// staging can never send production's adapter state to the adapter.
func TestStateStore_EnvironmentsAreIsolated(t *testing.T) {
	dir := t.TempDir()
	prod := NewStateStore(dir, "production")
	staging := NewStateStore(dir, "staging")

	if err := prod.Save(&State{Environment: "production", State: "prod-state"}); err != nil {
		t.Fatal(err)
	}
	got, err := staging.Load()
	if err != nil {
		t.Fatalf("staging Load: %v", err)
	}
	if got != nil {
		t.Fatalf("staging must not see production's state, got %+v", got)
	}
	if err := staging.Save(&State{Environment: "staging", State: "staging-state"}); err != nil {
		t.Fatal(err)
	}
	if err := staging.Delete(); err != nil {
		t.Fatal(err)
	}
	back, err := prod.Load()
	if err != nil || back == nil || back.State != "prod-state" {
		t.Fatalf("production state must survive staging's delete: %+v, %v", back, err)
	}
	if prod.Environment() != "production" {
		t.Errorf("Environment() = %q", prod.Environment())
	}

	// Plans are per environment too.
	if err := prod.SavePlan(NewSavedPlan("p", "production", "c", &PlanResponse{}, &PlanRequest{})); err != nil {
		t.Fatal(err)
	}
	if plan, err := staging.LoadPlan(); err != nil || plan != nil {
		t.Fatalf("staging must not see production's plan: %+v, %v", plan, err)
	}
}

func writeLegacyState(t *testing.T, dir, env string) string {
	t.Helper()
	legacyDir := filepath.Join(dir, ".promptarena")
	if err := os.MkdirAll(legacyDir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(legacyDir, "deploy.state")
	data := `{"version":1,"provider":"p","environment":"` + env + `","state":"legacy"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "deploy.plan"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestStateStore_AdoptsMatchingLegacyState pins the migration path: a
// pre-scoping .promptarena/deploy.state recorded for this environment is
// moved into the environment's directory.
func TestStateStore_AdoptsMatchingLegacyState(t *testing.T) {
	dir := t.TempDir()
	legacy := writeLegacyState(t, dir, "production")

	st, err := NewStateStore(dir, "production").Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st == nil || st.State != "legacy" {
		t.Fatalf("legacy state not adopted: %+v", st)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("the legacy file should have been moved")
	}
	if _, err := os.Stat(filepath.Join(dir, ".promptarena", "deploy.plan")); !os.IsNotExist(err) {
		t.Error("the legacy plan should have been dropped")
	}
	if _, err := os.Stat(filepath.Join(dir, ".promptarena", "envs", "production", "deploy.state")); err != nil {
		t.Errorf("state not at its per-environment path: %v", err)
	}
}

// TestStateStore_RefusesMismatchedLegacyState pins the safety half: a legacy
// file recorded for another environment is neither used nor ignored.
func TestStateStore_RefusesMismatchedLegacyState(t *testing.T) {
	dir := t.TempDir()
	legacy := writeLegacyState(t, dir, "production")

	st, err := NewStateStore(dir, "staging").Load()
	if err == nil {
		t.Fatalf("expected a refusal, got state %+v", st)
	}
	var mismatch *LegacyStateMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("error = %T %v, want *LegacyStateMismatchError", err, err)
	}
	if mismatch.Environment != "production" || mismatch.Requested != "staging" {
		t.Errorf("mismatch = %+v", mismatch)
	}
	if !strings.Contains(err.Error(), "--env production") {
		t.Errorf("message should say how to migrate: %q", err)
	}
	if _, statErr := os.Stat(legacy); statErr != nil {
		t.Error("a mismatched legacy file must be left alone")
	}
}

func TestStateStore_RejectsInvalidEnvironment(t *testing.T) {
	s := NewStateStore(t.TempDir(), "../escape")
	if _, err := s.Load(); err == nil {
		t.Error("Load should reject the env name")
	}
	if err := s.Save(&State{}); err == nil {
		t.Error("Save should reject the env name")
	}
	if _, err := s.LoadPlan(); err == nil {
		t.Error("LoadPlan should reject the env name")
	}
	if err := s.Delete(); err == nil {
		t.Error("Delete should reject the env name")
	}
}

func TestStateStore_LoadCorruptState(t *testing.T) {
	dir := t.TempDir()
	s := NewStateStore(dir, "default")
	if err := os.MkdirAll(s.envDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.statePath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Error("expected a parse error")
	}
}

// TestPackIdentityChecksum_IgnoresCompileTimestamp is the regression for
// #407 item 3: recompiling an unchanged pack must keep its checksum, so a
// saved plan stays fresh, while a real change must not.
func TestPackIdentityChecksum_IgnoresCompileTimestamp(t *testing.T) {
	a := []byte(`{"id":"x","version":"1.2.0","compilation":{"compiled_with":"packc-1","created_at":"2026-01-01T00:00:00Z"}}`)
	b := []byte(`{"version":"1.2.0","id":"x","compilation":{"created_at":"2026-01-01T00:00:09Z","compiled_with":"packc-1"}}`)
	c := []byte(`{"id":"y","version":"1.2.0","compilation":{"compiled_with":"packc-1","created_at":"2026-01-01T00:00:00Z"}}`)
	if PackIdentityChecksum(a) != PackIdentityChecksum(b) {
		t.Error("only created_at (and key order) differ; checksums must match")
	}
	if PackIdentityChecksum(a) == PackIdentityChecksum(c) {
		t.Error("a content change must change the checksum")
	}
	notJSON := []byte("not json")
	if PackIdentityChecksum(notJSON) != ComputePackChecksum(notJSON) {
		t.Error("non-JSON data falls back to the raw checksum")
	}
}

func TestPackVersion(t *testing.T) {
	if got := PackVersion([]byte(`{"version":"v1.0.0"}`)); got != "v1.0.0" {
		t.Errorf("PackVersion = %q", got)
	}
	if got := PackVersion([]byte(`nope`)); got != "" {
		t.Errorf("PackVersion(non-JSON) = %q", got)
	}
}
