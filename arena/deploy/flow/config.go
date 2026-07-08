package flow

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/packc/compiler"
)

// LoadConfig loads the arena config and its deploy section, erroring (with the
// configure docs URL) when the file or the deploy: section is missing.
func LoadConfig(opts Options) (*arenaconfig.Config, *arenaconfig.DeployConfig, error) {
	path := opts.config()
	if _, err := os.Stat(path); err != nil {
		return nil, nil, fmt.Errorf("deploy config %q not found: %w\nSee %s", path, err, ConfigureDocsURL)
	}
	cfg, err := arenaconfig.LoadConfig(path)
	if err != nil {
		return nil, nil, err
	}
	if cfg.Deploy == nil {
		return nil, nil, fmt.Errorf("no deploy: section in %q\nSee %s", path, ConfigureDocsURL)
	}
	return cfg, cfg.Deploy, nil
}

// ResolveEnv returns opts.Env or DefaultEnv.
func ResolveEnv(opts Options) string {
	if opts.Env != "" {
		return opts.Env
	}
	return DefaultEnv
}

// ResolvePack resolves the pack JSON to deploy. If opts.PackFile is set, it
// reads that pre-compiled *.pack.json file (explicit override). Otherwise it
// compiles the arena config (opts.config()) on the fly and returns the
// freshly compiled JSON, so users do not need to run a separate compile step
// before deploying. Compile warnings are not surfaced here — callers that want
// to display them should inspect them via their own compile path.
func ResolvePack(opts Options) ([]byte, error) {
	if opts.PackFile != "" {
		data, err := os.ReadFile(opts.PackFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read pack file %s: %w", opts.PackFile, err)
		}
		return data, nil
	}
	result, err := compiler.Compile(opts.config())
	if err != nil {
		return nil, fmt.Errorf("failed to compile pack from %s: %w", opts.config(), err)
	}
	return result.JSON, nil
}

// MergedConfigJSON merges base + env config and injects the stored login token
// into api_token when the config does not already carry one.
func MergedConfigJSON(deployCfg *arenaconfig.DeployConfig, env, configPath string) (string, error) {
	merged := map[string]interface{}{}
	for k, v := range deployCfg.Config {
		merged[k] = v
	}
	if envCfg, ok := deployCfg.Environments[env]; ok && envCfg != nil {
		for k, v := range envCfg.Config {
			merged[k] = v
		}
	}
	if tok, _ := merged["api_token"].(string); tok == "" {
		if stored, ok := LookupCredential(deployCfg.Provider, configPath); ok {
			merged["api_token"] = stored
		}
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// jsonUnmarshalString is a tiny wrapper so callers elsewhere in the package
// (e.g. preflight.go) can unmarshal a JSON string without importing
// encoding/json a second time.
func jsonUnmarshalString(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}

// SerializeArenaConfig serializes the full arena config as JSON for adapter
// consumption. Returns "" (rather than erroring) on marshal failure since
// arena config always round-trips through JSON-tagged structs.
//
// Exported (the brief's sample kept this unexported): the CLI's runDeploy/
// runDeployApply/runDeployPlan populate PlanRequest.ArenaConfig with it from
// outside this package, so it must be callable as flow.SerializeArenaConfig.
func SerializeArenaConfig(cfg *arenaconfig.Config) string {
	b, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return string(b)
}
