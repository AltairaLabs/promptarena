package flow

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

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
