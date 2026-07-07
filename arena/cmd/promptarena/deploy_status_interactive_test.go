package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRunDeployStatus_NoPriorState_ValidConfig locks in the intended "no
// deployment state found" happy path after restoring the config-validation
// call (flow.MergedConfigJSON) ahead of the priorState-nil early return in
// runDeployStatus: a valid deploy config with no prior deployment state must
// still print the friendly message and return nil, not surface a spurious
// error from the newly-restored validation call.
//
// A genuinely broken config (e.g. a value that fails JSON marshaling) can't
// be exercised end-to-end here: arenaconfig.LoadConfig (invoked earlier, by
// loadDeployConfig, before this test's code under change ever runs) already
// JSON-converts the whole raw document for schema validation and rejects
// anything unmarshalable at that point — so such a config already errors out
// one line before reaching the restored flow.MergedConfigJSON call, both
// before and after this fix. That specific error path is instead covered
// directly against flow.MergedConfigJSON in
// arena/deploy/flow/config_test.go (TestMergedConfigJSON_MarshalError), which
// builds an arenaconfig.DeployConfig programmatically (bypassing the YAML
// schema gate) to prove MergedConfigJSON itself still returns an error for
// an unmarshalable config value.
func TestRunDeployStatus_NoPriorState_ValidConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "arena.yaml")
	manifest := "apiVersion: promptkit.altairalabs.ai/v1alpha1\nkind: Arena\n" +
		"metadata:\n  name: t\nspec:\n  prompt_configs: []\n  providers: []\n" +
		"  deploy:\n    provider: omnia\n    config:\n      api_endpoint: https://x\n      workspace: demo\n"
	if err := os.WriteFile(cfgPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origCfg := deployConfig
	deployConfig = cfgPath
	t.Cleanup(func() { deployConfig = origCfg })

	if err := runDeployStatus(nil, nil); err != nil {
		t.Fatalf("runDeployStatus with valid config and no prior state: %v", err)
	}
}
