package flow

import (
	"context"
	"fmt"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
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
// adapter, and opens the state store.
func Open(ctx context.Context, opts Options) (*Session, error) {
	arena, dep, err := LoadConfig(opts)
	if err != nil {
		return nil, err
	}
	dir, err := opts.dir()
	if err != nil {
		return nil, err
	}
	packData, err := ResolvePack(opts)
	if err != nil {
		return nil, err
	}
	env := ResolveEnv(opts)
	cfgJSON, err := MergedConfigJSON(dep, env, opts.config())
	if err != nil {
		return nil, err
	}
	client, err := Connect(ctx, dep.Provider, dir)
	if err != nil {
		return nil, err
	}
	return NewSession(opts, arena, dep, client, deploy.NewStateStore(dir), packData, cfgJSON, client.Close), nil
}

// Close releases the adapter subprocess.
func (s *Session) Close() error {
	if s.closer != nil {
		return s.closer()
	}
	return nil
}

// PackChecksum returns the checksum of the resolved pack.
func (s *Session) PackChecksum() string { return deploy.ComputePackChecksum(s.packData) }

// PlanRequest builds a PlanRequest, refreshing prior adapter state first (soft-fail).
func (s *Session) PlanRequest(ctx context.Context) (*deploy.PlanRequest, error) {
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
	req, err := s.PlanRequest(ctx)
	if err != nil {
		return nil, nil, err
	}
	plan, err := s.Client.Plan(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("plan failed: %w", err)
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
	st := deploy.NewState(s.ProviderName, s.Env, "", s.PackChecksum(), version)
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
