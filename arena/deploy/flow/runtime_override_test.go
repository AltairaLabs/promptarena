package flow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/deploy"
)

func newOverrideSession(t *testing.T, client deploy.Provider, configJSON string) *Session {
	t.Helper()
	dir := t.TempDir()
	return NewSession(Options{ProjectDir: dir, Env: "staging"}, &arenaconfig.Config{},
		&arenaconfig.DeployConfig{Provider: "acme"}, client,
		deploy.NewEnvStateStore(dir, "staging"), []byte(`{"pack":"v1"}`), configJSON, nil)
}

const overrideConfig = `{"project":"p","runtime_override":{"image":"registry.example/other:1"}}`

// An adapter that does not advertise the capability would ignore the block and
// deploy its own runtime. The CLI must stop before the adapter is asked to
// validate or plan anything.
func TestSession_RuntimeOverrideRefusedWithoutCapability(t *testing.T) {
	fake := &fakeProvider{info: &deploy.ProviderInfo{Name: "acme", Version: "0.1.0"}}
	sess := newOverrideSession(t, fake, overrideConfig)

	_, _, err := sess.Plan(context.Background())
	if err == nil {
		t.Fatal("Plan succeeded against an adapter that cannot honor runtime_override")
	}
	for _, want := range []string{"runtime_override", `"acme"`, "version 0.1.0", "upgrade the adapter"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if fake.sawValidateReq != nil || fake.sawPlanReq != nil {
		t.Error("the adapter was consulted after the capability check failed")
	}
}

func TestSession_RuntimeOverrideAllowedWithCapability(t *testing.T) {
	fake := &fakeProvider{
		info: &deploy.ProviderInfo{Name: "acme", Capabilities: []string{"login", deploy.RuntimeOverrideCapability}},
		plan: &deploy.PlanResponse{Summary: "1 to create"},
	}
	sess := newOverrideSession(t, fake, overrideConfig)

	plan, req, err := sess.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Summary != "1 to create" {
		t.Errorf("Summary = %q", plan.Summary)
	}
	if req.DeployConfig != overrideConfig {
		t.Errorf("DeployConfig = %q, want the config passed through unchanged", req.DeployConfig)
	}
}

// A config without the block must not cost a get_provider_info round trip or
// depend on the adapter's capabilities at all.
func TestSession_NoRuntimeOverrideSkipsTheCheck(t *testing.T) {
	fake := &fakeProvider{plan: &deploy.PlanResponse{Summary: "ok"}} // info is nil
	sess := newOverrideSession(t, fake, `{"project":"p","runtime_override":{}}`)

	if _, err := sess.ValidateConfig(context.Background()); err != nil {
		t.Fatalf("ValidateConfig with an empty override: %v", err)
	}
	if fake.sawValidateReq == nil {
		t.Error("the adapter's validate_config was not called")
	}
}

func TestSession_MalformedRuntimeOverrideIsAConfigError(t *testing.T) {
	fake := &fakeProvider{}
	sess := newOverrideSession(t, fake, `{"runtime_override":{"env":["not","a","map"]}}`)

	_, err := sess.ValidateConfig(context.Background())
	if err == nil || !strings.Contains(err.Error(), "deploy config is invalid") {
		t.Fatalf("err = %v, want a config error", err)
	}
}

// deploy config import validates through the adapter directly rather than a
// Session, so the exported check must refuse on its own.
func TestRequireRuntimeOverrideSupport_Direct(t *testing.T) {
	ctx := context.Background()
	without := &fakeProvider{info: &deploy.ProviderInfo{Name: "acme"}}
	err := RequireRuntimeOverrideSupport(ctx, without, "acme", overrideConfig)
	if err == nil || !strings.Contains(err.Error(), "version unknown") {
		t.Errorf("err = %v, want a refusal naming the unknown version", err)
	}
	with := &fakeProvider{info: &deploy.ProviderInfo{Capabilities: []string{deploy.RuntimeOverrideCapability}}}
	if err := RequireRuntimeOverrideSupport(ctx, with, "acme", overrideConfig); err != nil {
		t.Errorf("capable adapter refused: %v", err)
	}
}

// The environment merge is one level deep, so an environment that sets its own
// runtime_override replaces the base block whole. A production entry that only
// adds env therefore drops the base image; this pins that documented behavior.
func TestMergedConfig_EnvironmentReplacesRuntimeOverride(t *testing.T) {
	dep := &arenaconfig.DeployConfig{
		Provider: "acme",
		Config: map[string]interface{}{
			"runtime_override": map[string]interface{}{"image": "base:1"},
		},
		Environments: map[string]*arenaconfig.DeployEnvironment{
			"production": {Config: map[string]interface{}{
				"runtime_override": map[string]interface{}{"env": map[string]interface{}{"A": "1"}},
			}},
		},
	}
	cfg, err := MergedConfigJSON(dep, "production", filepath.Join(t.TempDir(), "arena.yaml"))
	if err != nil {
		t.Fatalf("MergedConfigJSON: %v", err)
	}
	o, err := deploy.RuntimeOverrideFromConfig(cfg)
	if err != nil {
		t.Fatalf("RuntimeOverrideFromConfig: %v", err)
	}
	if o.Image != "" || o.Env["A"] != "1" {
		t.Errorf("override = %+v, want the environment's block alone", o)
	}
}
