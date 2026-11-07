package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/promptarena/v2/deploy"
)

// Session bundles resolved config, a connected adapter, and the state store.
type Session struct {
	Opts         Options
	ProviderName string
	Env          string
	Arena        *arenaconfig.Config
	Deploy       *arenaconfig.DeployConfig
	Store        *deploy.StateStore
	Client       deploy.Provider

	closer     func() error
	packData   []byte
	configJSON string
}

// NewSession builds a Session from already-resolved parts (used by Open and tests).
func NewSession(opts Options, arena *arenaconfig.Config, dep *arenaconfig.DeployConfig,
	client deploy.Provider, store *deploy.StateStore, packData []byte, configJSON string, closer func() error) *Session {
	return &Session{
		Opts: opts, ProviderName: dep.Provider, Env: ResolveEnv(opts),
		Arena: arena, Deploy: dep, Store: store, Client: client,
		closer: closer, packData: packData, configJSON: configJSON,
	}
}

// Open resolves config, compiles the pack, merges config JSON, connects the
// adapter, and opens the environment's state store.
func Open(ctx context.Context, opts Options) (*Session, error) {
	return open(ctx, opts, true)
}

// OpenWithoutPack is Open for commands that never send the pack (status): it
// skips pack resolution, so a config that does not compile can still be
// inspected. PlanRequest and Apply must not be used on the result.
func OpenWithoutPack(ctx context.Context, opts Options) (*Session, error) {
	return open(ctx, opts, false)
}

func open(ctx context.Context, opts Options, withPack bool) (*Session, error) {
	arena, dep, err := LoadConfig(opts)
	if err != nil {
		return nil, err
	}
	dir, err := opts.dir()
	if err != nil {
		return nil, err
	}
	env := ResolveEnv(opts)
	if envErr := deploy.ValidateEnvironmentName(env); envErr != nil {
		return nil, envErr
	}
	var packData []byte
	if withPack {
		if packData, err = ResolvePack(opts); err != nil {
			return nil, err
		}
	}
	cfgJSON, err := MergedConfigJSON(dep, env, opts.config())
	if err != nil {
		return nil, err
	}
	client, err := Connect(ctx, dep.Provider, dir)
	if err != nil {
		return nil, err
	}
	return NewSession(opts, arena, dep, client, deploy.NewStateStore(dir, env), packData, cfgJSON, client.Close), nil
}

// Close releases the adapter subprocess.
func (s *Session) Close() error {
	if s.closer != nil {
		return s.closer()
	}
	return nil
}

// PackChecksum returns the identity checksum of the resolved pack: its content
// minus the compile timestamp, so recompiling an unchanged pack keeps it.
func (s *Session) PackChecksum() string { return deploy.PackIdentityChecksum(s.packData) }

// PackVersion returns the resolved pack's version ("" when it has none).
func (s *Session) PackVersion() string { return deploy.PackVersion(s.packData) }

// ValidateConfig asks the adapter to validate the merged deploy config and
// returns its warnings. An invalid config is an error listing the adapter's
// reasons. An adapter that answers method-not-found is treated as having
// nothing to report, so adapters that only validate inside plan keep working.
func (s *Session) ValidateConfig(ctx context.Context) ([]string, error) {
	resp, err := s.Client.ValidateConfig(ctx, &deploy.ValidateRequest{Config: s.configJSON})
	if errors.Is(err, deploy.ErrMethodNotSupported) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("validate_config failed: %w", err)
	}
	if !resp.Valid {
		reasons := "no reason given"
		if len(resp.Errors) > 0 {
			reasons = strings.Join(resp.Errors, "; ")
		}
		return nil, fmt.Errorf("deploy config is invalid for %q: %s", s.ProviderName, reasons)
	}
	return resp.Warnings, nil
}

// PlanRequest builds a PlanRequest after validating the deploy config with
// the adapter, refreshing prior adapter state first (soft-fail).
func (s *Session) PlanRequest(ctx context.Context) (*deploy.PlanRequest, error) {
	req, _, err := s.planRequest(ctx)
	return req, err
}

// planRequest is PlanRequest that also returns the config warnings.
func (s *Session) planRequest(ctx context.Context) (*deploy.PlanRequest, []string, error) {
	warnings, err := s.ValidateConfig(ctx)
	if err != nil {
		return nil, nil, err
	}
	req, err := s.buildPlanRequest(ctx)
	return req, warnings, err
}

// buildPlanRequest assembles the request, refreshing prior state.
func (s *Session) buildPlanRequest(ctx context.Context) (*deploy.PlanRequest, error) {
	prior := ""
	st, err := s.Store.Load()
	if err != nil {
		return nil, err
	}
	if st != nil {
		prior = st.State
		if refreshed, err := s.Status(ctx); err == nil && refreshed.State != "" {
			prior = refreshed.State
			st.State = refreshed.State
			st.LastRefreshed = time.Now().UTC().Format(time.RFC3339)
			_ = s.Store.Save(st)
		}
	}
	return &deploy.PlanRequest{
		PackJSON:     string(s.packData),
		DeployConfig: s.configJSON,
		ArenaConfig:  SerializeArenaConfig(s.Arena),
		Environment:  s.Env,
		PriorState:   prior,
	}, nil
}

// Plan computes and returns the plan and the request that produced it.
func (s *Session) Plan(ctx context.Context) (*deploy.PlanResponse, *deploy.PlanRequest, error) {
	req, warnings, err := s.planRequest(ctx)
	if err != nil {
		return nil, nil, err
	}
	plan, err := s.Client.Plan(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("plan failed: %w", err)
	}
	if plan != nil && len(warnings) > 0 {
		plan.Warnings = append(warnings, plan.Warnings...)
	}
	return plan, req, nil
}

// Apply runs the plan, saves state, and clears the saved plan. Caller holds the lock.
func (s *Session) Apply(ctx context.Context, req *deploy.PlanRequest, cb deploy.ApplyCallback) error {
	adapterState, err := s.Client.Apply(ctx, req, cb)
	if err != nil {
		return fmt.Errorf("apply failed: %w", err)
	}
	version := ""
	if info, _ := s.Client.GetProviderInfo(ctx); info != nil {
		version = info.Version
	}
	st := deploy.NewState(s.ProviderName, s.Env, s.PackVersion(), s.PackChecksum(), version)
	st.State = adapterState
	if err := s.Store.Save(st); err != nil {
		return err
	}
	_ = s.Store.DeletePlan()
	return nil
}

// Status queries current deployment health.
func (s *Session) Status(ctx context.Context) (*deploy.StatusResponse, error) {
	prior := ""
	st, err := s.Store.Load()
	if err != nil {
		return nil, err
	}
	if st != nil {
		prior = st.State
	}
	return s.Client.Status(ctx, &deploy.StatusRequest{
		DeployConfig: s.configJSON, Environment: s.Env, PriorState: prior,
	})
}

// SavePlan persists a plan for a later apply.
func (s *Session) SavePlan(plan *deploy.PlanResponse, req *deploy.PlanRequest) error {
	return s.Store.SavePlan(deploy.NewSavedPlan(s.ProviderName, s.Env, s.PackChecksum(), plan, req))
}

// LoadPlan loads a previously saved plan (nil if none).
func (s *Session) LoadPlan() (*deploy.SavedPlan, error) { return s.Store.LoadPlan() }

// PlanIsFresh reports whether a saved plan matches the current pack checksum and env.
func (s *Session) PlanIsFresh(saved *deploy.SavedPlan) bool {
	return saved != nil && saved.PackChecksum == s.PackChecksum() && saved.Environment == s.Env
}
