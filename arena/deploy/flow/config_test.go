package flow

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

// writeArenaConfig writes a minimal but schema-valid arena manifest (the
// K8s-style envelope LoadConfig expects) wrapping the given spec fragment,
// and returns its path. Mirrors arenaconfig's own inline_specs_test.go helper.
func writeArenaConfig(t *testing.T, dir, specFragment string) string {
	t.Helper()
	configPath := filepath.Join(dir, "arena.yaml")
	content := "apiVersion: promptkit.altairalabs.ai/v1alpha1\nkind: Arena\nmetadata:\n  name: test-arena\nspec:\n" + specFragment
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

func TestResolveEnv(t *testing.T) {
	if got := ResolveEnv(Options{}); got != DefaultEnv {
		t.Fatalf("ResolveEnv default = %q, want %q", got, DefaultEnv)
	}
	if got := ResolveEnv(Options{Env: "production"}); got != "production" {
		t.Fatalf("ResolveEnv = %q, want production", got)
	}
}

// TestMergedConfigJSON_MarshalError locks in the error path that
// runDeployStatus (arena/cmd/promptarena/deploy_status_interactive.go) relies
// on to surface a broken deploy config before falling through to the "no
// prior state" early return. NaN can't be constructed from a config file
// (arenaconfig.LoadConfig's schema validation JSON-converts the whole
// document first and would already reject it there), so this exercises
// MergedConfigJSON directly with a value only reachable via a
// programmatically-built DeployConfig.
func TestMergedConfigJSON_MarshalError(t *testing.T) {
	dc := &arenaconfig.DeployConfig{
		Provider: "omnia",
		Config:   map[string]interface{}{"bad": math.NaN()},
	}
	if _, err := MergedConfigJSON(dc, "default", "arena.yaml"); err == nil {
		t.Fatal("expected marshal error for unmarshalable config value")
	}
}

func TestMergedConfigJSON_OverlaysEnvAndInjectsToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfgPath := "arena.yaml"
	_ = StoreCredential("omnia", cfgPath, Credential{Token: "stored-tok"})

	dc := &arenaconfig.DeployConfig{
		Provider: "omnia",
		Config:   map[string]interface{}{"region": "us"},
		Environments: map[string]*arenaconfig.DeployEnvironment{
			"production": {Config: map[string]interface{}{"region": "eu"}},
		},
	}
	out, err := MergedConfigJSON(dc, "production", cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	_ = json.Unmarshal([]byte(out), &m)
	if m["region"] != "eu" {
		t.Fatalf("region = %v, want eu (env override)", m["region"])
	}
	if m["api_token"] != "stored-tok" {
		t.Fatalf("api_token = %v, want stored-tok (injected)", m["api_token"])
	}
}

func TestResolvePack_PackFileOverride(t *testing.T) {
	dir := t.TempDir()
	packPath := filepath.Join(dir, "arena.pack.json")
	want := []byte(`{"scenarios":[{"id":"greeting"}]}`)
	if err := os.WriteFile(packPath, want, 0o600); err != nil {
		t.Fatalf("write pack file: %v", err)
	}

	got, err := ResolvePack(Options{PackFile: packPath})
	if err != nil {
		t.Fatalf("ResolvePack: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("ResolvePack = %s, want %s (should read the override verbatim, not recompile)", got, want)
	}
}

func TestResolvePack_PackFileMissing(t *testing.T) {
	_, err := ResolvePack(Options{PackFile: filepath.Join(t.TempDir(), "nope.pack.json")})
	if err == nil {
		t.Fatal("expected error for missing pack file")
	}
	if !strings.Contains(err.Error(), "failed to read pack file") {
		t.Fatalf("error = %q, want it to mention the pack file read failure", err.Error())
	}
}

// TestResolvePack_CompilesFromConfig_Error covers ResolvePack's "no
// PackFile override" branch: it must fall through to compiling opts.config()
// on the fly, and a config that fails to compile (here: a scenario-less
// config, so the compiler has nothing to build a pack from) must surface a
// wrapped "failed to compile pack from" error rather than a bare compiler error.
func TestResolvePack_CompilesFromConfig_Error(t *testing.T) {
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

	_, err := ResolvePack(Options{ConfigPath: configPath})
	if err == nil {
		t.Fatal("expected ResolvePack to fail compiling a config with no prompts/scenarios")
	}
	if !strings.Contains(err.Error(), "failed to compile pack from") {
		t.Fatalf("error = %q, want it to mention the compile failure", err.Error())
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, _, err := LoadConfig(Options{ConfigPath: filepath.Join(t.TempDir(), "nope.yaml")})
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
	if !strings.Contains(err.Error(), ConfigureDocsURL) {
		t.Fatalf("error = %q, want it to mention %q", err.Error(), ConfigureDocsURL)
	}
}

func TestLoadConfig_NoDeploySection(t *testing.T) {
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

	_, _, err := LoadConfig(Options{ConfigPath: configPath})
	if err == nil {
		t.Fatal("expected error for config with no deploy: section")
	}
	if !strings.Contains(err.Error(), ConfigureDocsURL) || !strings.Contains(err.Error(), "no deploy:") {
		t.Fatalf("error = %q, want it to mention the missing deploy section and %q", err.Error(), ConfigureDocsURL)
	}
}

func TestLoadConfig_ValidConfigWithDeploy(t *testing.T) {
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
      region: us-east-1
`)

	arena, dep, err := LoadConfig(Options{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if arena == nil {
		t.Fatal("expected non-nil arena config")
	}
	if dep == nil || dep.Provider != "omnia" {
		t.Fatalf("dep = %+v, want Provider=omnia", dep)
	}
	if dep.Config["region"] != "us-east-1" {
		t.Fatalf("dep.Config[region] = %v, want us-east-1", dep.Config["region"])
	}
	if arena.Deploy != dep {
		t.Fatal("LoadConfig should return arena.Deploy as the second value")
	}
}

func TestJSONUnmarshalString(t *testing.T) {
	var m map[string]interface{}
	if err := jsonUnmarshalString(`{"a":1}`, &m); err != nil {
		t.Fatalf("jsonUnmarshalString: %v", err)
	}
	if m["a"] != 1.0 {
		t.Fatalf("m[a] = %v, want 1", m["a"])
	}
	if err := jsonUnmarshalString(`not-json`, &m); err == nil {
		t.Fatal("expected error unmarshaling invalid JSON")
	}
}

func TestSerializeArenaConfig(t *testing.T) {
	cfg := &arenaconfig.Config{
		Deploy: &arenaconfig.DeployConfig{Provider: "omnia"},
	}
	out := SerializeArenaConfig(cfg)
	if out == "" {
		t.Fatal("expected non-empty JSON")
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	deploy, ok := m["deploy"].(map[string]interface{})
	if !ok || deploy["provider"] != "omnia" {
		t.Fatalf("deploy.provider = %v, want omnia", m["deploy"])
	}
}

// TestSerializeArenaConfig_MarshalError covers the documented fallback: a
// config that cannot round-trip through JSON (a channel value nested in the
// deploy config's opaque map) must yield "" rather than a panic or a
// half-written string.
func TestSerializeArenaConfig_MarshalError(t *testing.T) {
	cfg := &arenaconfig.Config{
		Deploy: &arenaconfig.DeployConfig{
			Provider: "omnia",
			Config:   map[string]interface{}{"bad": make(chan int)},
		},
	}
	if out := SerializeArenaConfig(cfg); out != "" {
		t.Fatalf("SerializeArenaConfig = %q, want empty string on marshal failure", out)
	}
}
