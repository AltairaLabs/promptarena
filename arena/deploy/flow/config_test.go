package flow

import (
	"encoding/json"
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
