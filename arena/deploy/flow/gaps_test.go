package flow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/deploy"
)

// Regression tests for the deploy gaps in #407 / #417 / #429.

const noPromptsDeploySpec = `  providers: []
  provider_specs:
    p1:
      type: openai
      model: gpt-4
  defaults:
    concurrency: 1
  deploy:
    provider: nonexistent-xyz
    config: {}
`

// TestOpenWithoutPack_DoesNotCompile is #407 item 5: status never sends the
// pack, so a config that cannot compile must still get as far as the adapter.
func TestOpenWithoutPack_DoesNotCompile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()
	configPath := writeArenaConfig(t, dir, noPromptsDeploySpec)
	opts := Options{ConfigPath: configPath, ProjectDir: dir}

	// Open compiles, and this config has nothing to compile.
	if _, err := Open(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "compile pack") {
		t.Fatalf("Open err = %v, want a compile failure (precondition)", err)
	}

	_, err := OpenWithoutPack(context.Background(), opts)
	if err == nil {
		t.Fatal("expected the missing adapter to fail")
	}
	if strings.Contains(err.Error(), "compile pack") {
		t.Fatalf("OpenWithoutPack must not compile the pack, got %q", err)
	}
	if !strings.Contains(err.Error(), "adapter not found") {
		t.Fatalf("err = %q, want it to reach Connect", err)
	}
}

func TestOpen_RejectsUnsafeEnvironmentName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()
	configPath := writeArenaConfig(t, dir, noPromptsDeploySpec)
	_, err := OpenWithoutPack(context.Background(), Options{ConfigPath: configPath, ProjectDir: dir, Env: "../prod"})
	if err == nil || !strings.Contains(err.Error(), "invalid deploy environment name") {
		t.Fatalf("err = %v, want an env-name rejection", err)
	}
}

// TestLogin_BootstrapsConfigWithoutDeploySection is #407 item 7: a fresh
// config with no deploy section used to fail with "no deploy: section"
// before login could create one. It must now get as far as the adapter.
func TestLogin_BootstrapsConfigWithoutDeploySection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()
	configPath := writeArenaConfig(t, dir, `  providers: []
  provider_specs:
    p1:
      type: openai
      model: gpt-4
  defaults:
    concurrency: 1
`)
	err := Login(context.Background(), "nonexistent-xyz", Options{ConfigPath: configPath, ProjectDir: dir}, LoginHooks{})
	if err == nil {
		t.Fatal("expected the missing adapter to fail")
	}
	if strings.Contains(err.Error(), "no deploy:") {
		t.Fatalf("login must not require a deploy section, got %q", err)
	}
	if !strings.Contains(err.Error(), "adapter not found") {
		t.Fatalf("err = %q, want it to reach Connect", err)
	}
}

// TestLogin_ChecksConfigBeforeStartingAdapter pins the ordering half of
// item 7: a missing config is reported before any adapter is looked up.
func TestLogin_ChecksConfigBeforeStartingAdapter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	err := Login(context.Background(), "nonexistent-xyz",
		Options{ConfigPath: filepath.Join(dir, "absent.yaml"), ProjectDir: dir}, LoginHooks{})
	if err == nil || !strings.Contains(err.Error(), "config file not found") {
		t.Fatalf("err = %v, want config-not-found", err)
	}
}

func TestLoginConfigJSON_UsesExistingDeploySection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()
	configPath := writeArenaConfig(t, dir, `  providers: []
  provider_specs:
    p1:
      type: openai
      model: gpt-4
  defaults:
    concurrency: 1
  deploy:
    provider: omnia
    config:
      workspace: demo
`)
	got, err := loginConfigJSON("omnia", Options{ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"workspace":"demo"`) {
		t.Errorf("config JSON = %s, want the existing deploy config", got)
	}
}

func TestLoginConfigJSON_PropagatesLoadError(t *testing.T) {
	dir := t.TempDir()
	configPath := writeArenaConfig(t, dir, "  : : not yaml\n")
	if _, err := loginConfigJSON("omnia", Options{ConfigPath: configPath}); err == nil {
		t.Fatal("expected a load error")
	}
}

// TestMergeProfileIntoConfigFile_StatErrorIsReported covers a stat failure
// other than "does not exist" (a path through a regular file).
func TestMergeProfileIntoConfigFile_StatErrorIsReported(t *testing.T) {
	dir := t.TempDir()
	file := writeArenaConfig(t, dir, "  providers: []\n")
	err := MergeProfileIntoConfigFile(filepath.Join(file, "nested.yaml"), map[string]interface{}{}, "omnia")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestEnvironments(t *testing.T) {
	if got := Environments(nil); !reflect.DeepEqual(got, []string{DefaultEnv}) {
		t.Errorf("Environments(nil) = %v", got)
	}
	dep := &arenaconfig.DeployConfig{Environments: map[string]*arenaconfig.DeployEnvironment{
		"staging": {}, "production": {}, DefaultEnv: {},
	}}
	want := []string{DefaultEnv, "production", "staging"}
	if got := Environments(dep); !reflect.DeepEqual(got, want) {
		t.Errorf("Environments = %v, want %v", got, want)
	}
}

// TestSession_PlanValidatesConfigFirst is #407 item 9: an adapter that only
// validates in validate_config must be validated on the deploy path.
func TestSession_PlanValidatesConfigFirst(t *testing.T) {
	fake := &fakeProvider{
		plan:     &deploy.PlanResponse{},
		status:   &deploy.StatusResponse{},
		validate: &deploy.ValidateResponse{Valid: false, Errors: []string{"region is required"}},
	}
	sess, _ := newTestSession(t, fake)

	_, _, err := sess.Plan(context.Background())
	if err == nil || !strings.Contains(err.Error(), "region is required") {
		t.Fatalf("err = %v, want the adapter's validation error", err)
	}
	if fake.sawPlanReq != nil {
		t.Error("an invalid config must not be planned")
	}
	if fake.sawValidateReq == nil || fake.sawValidateReq.Config != `{"cfg":true}` {
		t.Errorf("validate saw %+v, want the merged deploy config", fake.sawValidateReq)
	}
}

func TestSession_ValidateConfig_InvalidWithoutReasons(t *testing.T) {
	fake := &fakeProvider{validate: &deploy.ValidateResponse{Valid: false}}
	sess, _ := newTestSession(t, fake)
	if _, err := sess.ValidateConfig(context.Background()); err == nil || !strings.Contains(err.Error(), "no reason given") {
		t.Fatalf("err = %v", err)
	}
}

func TestSession_ValidateConfig_MethodNotFoundIsSkipped(t *testing.T) {
	fake := &fakeProvider{
		plan:        &deploy.PlanResponse{},
		status:      &deploy.StatusResponse{},
		validateErr: fmt.Errorf("adapter error -32601: %w", deploy.ErrMethodNotSupported),
	}
	sess, _ := newTestSession(t, fake)
	if _, _, err := sess.Plan(context.Background()); err != nil {
		t.Fatalf("an adapter without validate_config must still plan: %v", err)
	}
}

func TestSession_ValidateConfig_TransportErrorFails(t *testing.T) {
	fake := &fakeProvider{validateErr: errors.New("pipe closed")}
	sess, _ := newTestSession(t, fake)
	if _, err := sess.PlanRequest(context.Background()); err == nil || !strings.Contains(err.Error(), "validate_config failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestSession_PlanPrependsValidationWarnings(t *testing.T) {
	fake := &fakeProvider{
		plan:     &deploy.PlanResponse{Warnings: []string{"from plan"}},
		status:   &deploy.StatusResponse{},
		validate: &deploy.ValidateResponse{Valid: true, Warnings: []string{"from validate"}},
	}
	sess, _ := newTestSession(t, fake)
	plan, _, err := sess.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"from validate", "from plan"}; !reflect.DeepEqual(plan.Warnings, want) {
		t.Errorf("warnings = %v, want %v", plan.Warnings, want)
	}
}

// TestSession_ApplyRecordsPackVersion is #407 item 2.
func TestSession_ApplyRecordsPackVersion(t *testing.T) {
	fake := &fakeProvider{info: &deploy.ProviderInfo{Version: "1"}}
	dir := t.TempDir()
	store := deploy.NewEnvStateStore(dir, "staging")
	sess := NewSession(Options{ProjectDir: dir, Env: "staging"}, &arenaconfig.Config{},
		&arenaconfig.DeployConfig{Provider: "acme"}, fake, store,
		[]byte(`{"id":"p","version":"v2.3.0"}`), `{}`, nil)
	if err := sess.Apply(context.Background(), &deploy.PlanRequest{}, nil); err != nil {
		t.Fatal(err)
	}
	st, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.PackVersion != "v2.3.0" {
		t.Errorf("pack_version = %q, want v2.3.0", st.PackVersion)
	}
}

// TestSession_SavedPlanSurvivesRecompile is #407 item 3: two compiles of the
// same pack differ only in compilation.created_at, and a plan saved from the
// first must be fresh for the second.
func TestSession_SavedPlanSurvivesRecompile(t *testing.T) {
	dir := t.TempDir()
	store := deploy.NewEnvStateStore(dir, "staging")
	mk := func(createdAt string) *Session {
		pack := `{"id":"p","compilation":{"compiled_with":"packc","created_at":"` + createdAt + `"}}`
		return NewSession(Options{ProjectDir: dir, Env: "staging"}, &arenaconfig.Config{},
			&arenaconfig.DeployConfig{Provider: "acme"}, nil, store, []byte(pack), `{}`, nil)
	}
	first := mk("2026-01-01T00:00:00Z")
	if err := first.SavePlan(&deploy.PlanResponse{}, &deploy.PlanRequest{}); err != nil {
		t.Fatal(err)
	}
	saved, err := first.LoadPlan()
	if err != nil {
		t.Fatal(err)
	}
	if !mk("2026-01-01T00:05:00Z").PlanIsFresh(saved) {
		t.Error("a recompile of an unchanged pack must not make the saved plan stale")
	}
}

func TestCheckPreflight_ListsEnvironmentsAndRejectsBadEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()
	configPath := writeArenaConfig(t, dir, `  providers: []
  provider_specs:
    p1:
      type: openai
      model: gpt-4
  defaults:
    concurrency: 1
  deploy:
    provider: nonexistent-xyz
    config: {}
    environments:
      production:
        config: {}
`)
	pf := CheckPreflight(context.Background(), Options{ConfigPath: configPath, ProjectDir: dir})
	if want := []string{DefaultEnv, "production"}; !reflect.DeepEqual(pf.Environments, want) {
		t.Errorf("Environments = %v, want %v", pf.Environments, want)
	}

	pf = CheckPreflight(context.Background(), Options{ConfigPath: configPath, ProjectDir: dir, Env: "a/b"})
	if pf.ConfigErr == nil {
		t.Error("an unsafe env name must surface as a config error")
	}
}
