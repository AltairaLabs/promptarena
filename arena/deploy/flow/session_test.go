package flow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

var (
	errPlanBoom   = errors.New("boom")
	errApplyBoom  = errors.New("boom")
	errStatusBoom = errors.New("boom")
)

type fakeProvider struct {
	plan      *deploy.PlanResponse
	planErr   error
	applyErr  error
	statusErr error
	events    []*deploy.ApplyEvent
	status    *deploy.StatusResponse
	applied   bool
}

func (f *fakeProvider) GetProviderInfo(context.Context) (*deploy.ProviderInfo, error) {
	return &deploy.ProviderInfo{Name: "fake", Version: "9.9.9", Capabilities: []string{deploy.LoginCapability}}, nil
}
func (f *fakeProvider) ValidateConfig(context.Context, *deploy.ValidateRequest) (*deploy.ValidateResponse, error) {
	return &deploy.ValidateResponse{Valid: true}, nil
}
func (f *fakeProvider) Plan(context.Context, *deploy.PlanRequest) (*deploy.PlanResponse, error) {
	if f.planErr != nil {
		return nil, f.planErr
	}
	return f.plan, nil
}
func (f *fakeProvider) Apply(_ context.Context, _ *deploy.PlanRequest, cb deploy.ApplyCallback) (string, error) {
	if f.applyErr != nil {
		return "", f.applyErr
	}
	f.applied = true
	for _, e := range f.events {
		if err := cb(e); err != nil {
			return "", err
		}
	}
	return "opaque-state", nil
}
func (f *fakeProvider) Destroy(context.Context, *deploy.DestroyRequest, deploy.DestroyCallback) error {
	return nil
}
func (f *fakeProvider) Status(context.Context, *deploy.StatusRequest) (*deploy.StatusResponse, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return f.status, nil
}
func (f *fakeProvider) Import(context.Context, *deploy.ImportRequest) (*deploy.ImportResponse, error) {
	return &deploy.ImportResponse{}, nil
}

func newTestSession(t *testing.T, fp deploy.Provider) *Session {
	t.Helper()
	dir := t.TempDir()
	dc := &arenaconfig.DeployConfig{Provider: "fake", Config: map[string]interface{}{}}
	return NewSession(
		Options{ProjectDir: dir},
		&arenaconfig.Config{Deploy: dc},
		dc,
		fp,
		deploy.NewStateStore(dir),
		[]byte(`{"pack":true}`),
		`{}`,
		func() error { return nil },
	)
}

func TestSession_PlanApplyRoundtrip(t *testing.T) {
	fp := &fakeProvider{
		plan: &deploy.PlanResponse{
			Summary: "1 to add",
			Changes: []deploy.ResourceChange{{Type: "agent_runtime", Name: "bot", Action: deploy.ActionCreate}},
		},
		events: []*deploy.ApplyEvent{
			{Type: "resource", Resource: &deploy.ResourceResult{Type: "agent_runtime", Name: "bot", Status: "created"}},
			{Type: "complete", Message: "done"},
		},
	}
	s := newTestSession(t, fp)
	ctx := context.Background()

	plan, req, err := s.Plan(ctx)
	if err != nil || plan.Summary != "1 to add" {
		t.Fatalf("Plan: %v %+v", err, plan)
	}

	if err := s.SavePlan(plan, req); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if saved, err := s.LoadPlan(); err != nil || saved == nil {
		t.Fatalf("expected saved plan before apply: saved=%+v err=%v", saved, err)
	}

	var seen []string
	if err := s.Apply(ctx, req, func(e *deploy.ApplyEvent) error {
		seen = append(seen, e.Type)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !fp.applied || len(seen) != 2 {
		t.Fatalf("apply not driven: applied=%v seen=%v", fp.applied, seen)
	}
	// State saved, plan cleared.
	if st, _ := s.Store.Load(); st == nil {
		t.Fatal("expected state saved after apply")
	}
	if saved, err := s.LoadPlan(); err != nil {
		t.Fatalf("LoadPlan after apply: %v", err)
	} else if saved != nil {
		t.Fatal("expected saved plan deleted after apply")
	}
}

// TestSession_Status_NoPriorState covers the "no saved state yet" branch:
// PriorState should be empty and the fake's canned status returned verbatim.
func TestSession_Status_NoPriorState(t *testing.T) {
	fp := &fakeProvider{status: &deploy.StatusResponse{Status: "deployed"}}
	s := newTestSession(t, fp)

	got, err := s.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got != fp.status {
		t.Fatalf("Status() = %+v, want the fake's canned response", got)
	}
}

// TestSession_Status_ReadsPriorState covers the branch where a previously
// saved deploy state exists: Status must read it from the store and thread it
// into the StatusRequest as PriorState.
func TestSession_Status_ReadsPriorState(t *testing.T) {
	var gotReq *deploy.StatusRequest
	fp := &statusCapturingProvider{fakeProvider: fakeProvider{status: &deploy.StatusResponse{Status: "deployed"}}, capture: &gotReq}
	s := newTestSession(t, fp)

	st := deploy.NewState(s.ProviderName, s.Env, "", s.PackChecksum(), "1.0.0")
	st.State = "opaque-prior-state"
	if err := s.Store.Save(st); err != nil {
		t.Fatalf("Store.Save: %v", err)
	}

	if _, err := s.Status(context.Background()); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if gotReq == nil {
		t.Fatal("expected Status to call the provider")
	}
	if gotReq.PriorState != "opaque-prior-state" {
		t.Fatalf("PriorState = %q, want the saved state to be threaded through", gotReq.PriorState)
	}
	if gotReq.Environment != s.Env {
		t.Fatalf("Environment = %q, want %q", gotReq.Environment, s.Env)
	}
}

// statusCapturingProvider wraps fakeProvider to record the StatusRequest it
// was called with, so tests can assert on PriorState/Environment plumbing.
type statusCapturingProvider struct {
	fakeProvider
	capture **deploy.StatusRequest
}

func (s *statusCapturingProvider) Status(ctx context.Context, req *deploy.StatusRequest) (*deploy.StatusResponse, error) {
	*s.capture = req
	return s.fakeProvider.Status(ctx, req)
}

func TestSession_Close_InvokesCloser(t *testing.T) {
	s := newTestSession(t, &fakeProvider{})
	called := 0
	s.closer = func() error { called++; return nil }

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if called != 1 {
		t.Fatalf("closer invoked %d times, want 1", called)
	}
	// Idempotent: calling again should not panic and should invoke it again
	// (Close has no "already closed" guard — it just delegates every time).
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if called != 2 {
		t.Fatalf("closer invoked %d times after second Close, want 2", called)
	}
}

func TestSession_Close_NilSafe(t *testing.T) {
	s := newTestSession(t, &fakeProvider{})
	s.closer = nil
	if err := s.Close(); err != nil {
		t.Fatalf("Close with nil closer should be a no-op, got %v", err)
	}
}

// TestSession_PlanRequest_BuildsAndRefreshesPriorState locks in what
// PlanRequest assembles: PackJSON/DeployConfig/ArenaConfig/Environment from
// session state, and — when a prior state exists — a fresh call to Status to
// refresh PriorState before building the request.
func TestSession_PlanRequest_BuildsAndRefreshesPriorState(t *testing.T) {
	fp := &fakeProvider{status: &deploy.StatusResponse{State: "refreshed-state"}}
	s := newTestSession(t, fp)

	st := deploy.NewState(s.ProviderName, s.Env, "", s.PackChecksum(), "1.0.0")
	st.State = "stale-state"
	if err := s.Store.Save(st); err != nil {
		t.Fatalf("Store.Save: %v", err)
	}

	req, err := s.PlanRequest(context.Background())
	if err != nil {
		t.Fatalf("PlanRequest: %v", err)
	}
	if req.PackJSON != `{"pack":true}` {
		t.Fatalf("PackJSON = %q", req.PackJSON)
	}
	if req.DeployConfig != "{}" {
		t.Fatalf("DeployConfig = %q", req.DeployConfig)
	}
	if req.Environment != s.Env {
		t.Fatalf("Environment = %q, want %q", req.Environment, s.Env)
	}
	if req.ArenaConfig == "" {
		t.Fatal("expected non-empty serialized ArenaConfig")
	}
	if req.PriorState != "refreshed-state" {
		t.Fatalf("PriorState = %q, want the refreshed status state (not the stale saved one)", req.PriorState)
	}

	// The refreshed state must also have been persisted back to the store.
	reloaded, err := s.Store.Load()
	if err != nil {
		t.Fatalf("Store.Load: %v", err)
	}
	if reloaded.State != "refreshed-state" {
		t.Fatalf("stored state = %q, want it updated to the refreshed value", reloaded.State)
	}
}

// TestSession_PlanRequest_NoPriorState covers the branch with nothing saved
// yet: PriorState should be empty and no refresh attempted.
func TestSession_PlanRequest_NoPriorState(t *testing.T) {
	s := newTestSession(t, &fakeProvider{})
	req, err := s.PlanRequest(context.Background())
	if err != nil {
		t.Fatalf("PlanRequest: %v", err)
	}
	if req.PriorState != "" {
		t.Fatalf("PriorState = %q, want empty with nothing saved", req.PriorState)
	}
}

// TestSession_Plan_WrapsClientError covers the branch where the adapter's
// Plan call itself fails: Session.Plan must wrap it with "plan failed: ".
func TestSession_Plan_WrapsClientError(t *testing.T) {
	fp := &fakeProvider{planErr: errPlanBoom}
	s := newTestSession(t, fp)

	_, _, err := s.Plan(context.Background())
	if err == nil || !strings.Contains(err.Error(), "plan failed") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Plan error = %v, want it to wrap the client's plan error", err)
	}
}

// TestSession_Apply_WrapsClientError covers the branch where the adapter's
// Apply call fails: Session.Apply must wrap it with "apply failed: " and must
// not persist any state.
func TestSession_Apply_WrapsClientError(t *testing.T) {
	fp := &fakeProvider{applyErr: errApplyBoom}
	s := newTestSession(t, fp)

	err := s.Apply(context.Background(), &deploy.PlanRequest{}, func(*deploy.ApplyEvent) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "apply failed") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Apply error = %v, want it to wrap the client's apply error", err)
	}
	if st, _ := s.Store.Load(); st != nil {
		t.Fatal("no state should be saved when Apply fails")
	}
}

// TestSession_Status_PropagatesStoreLoadError covers the branch where the
// on-disk state file exists but is corrupt: Status must propagate the
// Store.Load error rather than silently treating it as "no prior state".
func TestSession_Status_PropagatesStoreLoadError(t *testing.T) {
	s := newTestSession(t, &fakeProvider{status: &deploy.StatusResponse{Status: "deployed"}})
	corruptStateFile(t, s.Opts.ProjectDir)

	if _, err := s.Status(context.Background()); err == nil {
		t.Fatal("expected Status to propagate the corrupt state file error")
	}
}

// TestSession_PlanRequest_PropagatesStoreLoadError mirrors the above for
// PlanRequest, which also calls s.Store.Load() first.
func TestSession_PlanRequest_PropagatesStoreLoadError(t *testing.T) {
	s := newTestSession(t, &fakeProvider{})
	corruptStateFile(t, s.Opts.ProjectDir)

	if _, err := s.PlanRequest(context.Background()); err == nil {
		t.Fatal("expected PlanRequest to propagate the corrupt state file error")
	}
}

// TestSession_PlanRequest_StatusRefreshErrorKeepsSavedPriorState covers the
// soft-fail refresh branch: when the adapter's Status call errors during the
// refresh attempt, PlanRequest must fall back to the previously saved state
// rather than losing it or failing outright.
func TestSession_PlanRequest_StatusRefreshErrorKeepsSavedPriorState(t *testing.T) {
	fp := &fakeProvider{statusErr: errStatusBoom}
	s := newTestSession(t, fp)

	st := deploy.NewState(s.ProviderName, s.Env, "", s.PackChecksum(), "1.0.0")
	st.State = "saved-state"
	if err := s.Store.Save(st); err != nil {
		t.Fatalf("Store.Save: %v", err)
	}

	req, err := s.PlanRequest(context.Background())
	if err != nil {
		t.Fatalf("PlanRequest: %v", err)
	}
	if req.PriorState != "saved-state" {
		t.Fatalf("PriorState = %q, want the saved state preserved when refresh fails", req.PriorState)
	}
}

// corruptStateFile writes invalid JSON to the on-disk deploy state file so
// StateStore.Load returns a genuine parse error instead of (nil, nil).
func corruptStateFile(t *testing.T, projectDir string) {
	t.Helper()
	dir := filepath.Join(projectDir, ".promptarena")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir .promptarena: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deploy.state"), []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write corrupt state file: %v", err)
	}
}

func TestSession_PlanIsFresh(t *testing.T) {
	s := newTestSession(t, &fakeProvider{})
	saved := deploy.NewSavedPlan("fake", DefaultEnv, s.PackChecksum(), &deploy.PlanResponse{}, &deploy.PlanRequest{})
	if !s.PlanIsFresh(saved) {
		t.Fatal("expected matching checksum+env to be fresh")
	}
	stale := deploy.NewSavedPlan("fake", "production", "sha256:zzz", &deploy.PlanResponse{}, &deploy.PlanRequest{})
	if s.PlanIsFresh(stale) {
		t.Fatal("expected mismatched plan to be stale")
	}
}
