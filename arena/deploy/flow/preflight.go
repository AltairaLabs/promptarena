package flow

import (
	"context"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

// Preflight is the deploy readiness snapshot shown before planning.
type Preflight struct {
	Provider       string
	Env            string
	AdapterFound   bool
	AdapterPath    string
	AdapterVersion string
	Capabilities   []string
	SupportsLogin  bool
	Authenticated  bool
	InstallCommand string
	ConfigErr      error
	ProbeErr       error
}

// Ready reports whether a plan can proceed.
func (p *Preflight) Ready() bool {
	return p.ConfigErr == nil && p.ProbeErr == nil && p.AdapterFound && p.Authenticated
}

// CheckPreflight resolves config, adapter presence/version, capabilities, and auth
// state. It never returns an error — failures are captured in the struct so the UI
// can render a partial gate.
func CheckPreflight(ctx context.Context, opts Options) *Preflight {
	pf := &Preflight{Env: ResolveEnv(opts)}

	_, dep, err := LoadConfig(opts)
	if err != nil {
		pf.ConfigErr = err
		return pf
	}
	pf.Provider = dep.Provider
	pf.InstallCommand = InstallCommand(dep.Provider)

	dir, err := opts.dir()
	if err != nil {
		pf.ConfigErr = err
		return pf
	}
	path, found := AdapterInstalled(dep.Provider, dir)
	pf.AdapterFound, pf.AdapterPath = found, path
	if !found {
		return pf // no point probing a missing adapter
	}

	client, err := Connect(ctx, dep.Provider, dir)
	if err != nil {
		pf.ProbeErr = err
		return pf
	}
	defer client.Close()

	info, err := client.GetProviderInfo(ctx)
	if err != nil {
		pf.ProbeErr = err
		return pf
	}
	pf.AdapterVersion = info.Version
	pf.Capabilities = info.Capabilities
	for _, c := range info.Capabilities {
		if c == deploy.LoginCapability {
			pf.SupportsLogin = true
		}
	}

	// Authenticated if the merged config carries a token (explicit or stored).
	if cfgJSON, err := MergedConfigJSON(dep, pf.Env, opts.config()); err == nil {
		pf.Authenticated = configHasToken(cfgJSON)
	}
	return pf
}

func configHasToken(cfgJSON string) bool {
	// Cheap check without a full unmarshal dependency footprint.
	// api_token is injected by MergedConfigJSON when a credential exists.
	var m map[string]interface{}
	if err := jsonUnmarshalString(cfgJSON, &m); err != nil {
		return false
	}
	tok, _ := m["api_token"].(string)
	return tok != ""
}
