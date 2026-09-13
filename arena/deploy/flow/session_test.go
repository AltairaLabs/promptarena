package flow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/deploy"
)

// TestOpen_ConfigMissing covers Session.Open's first exit, which was 0%.
// Open is the entry point for every deploy verb, so a bad config has to fail
// here — before a client is dialled or a pack compiled — rather than part-way
// through an apply with resources already created.
func TestOpen_ConfigMissing(t *testing.T) {
	dir := t.TempDir()
	sess, err := Open(context.Background(), Options{
		ProjectDir: dir, ConfigPath: dir + "/absent.yaml",
	})
	if err == nil {
		t.Fatal("expected an error for a missing config")
	}
	if sess != nil {
		t.Error("no session may be returned alongside an error")
	}
}

// TestOpen_UnresolvablePackFailsBeforeDialling covers Open's pack-resolution
// exit. The ordering is the point: Open compiles the pack before it dials the
// adapter, so a kit that cannot produce a pack fails locally instead of after
// a connection — and the message names the config rather than surfacing as a
// transport error the reader would chase in the wrong direction.
func TestOpen_UnresolvablePackFailsBeforeDialling(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
    provider: nonexistent-xyz
    config: {}
`)

	sess, err := Open(context.Background(), Options{ConfigPath: configPath, ProjectDir: dir})
	if err == nil {
		if sess != nil {
			_ = sess.Close()
		}
		t.Fatal("expected an error for a provider with no adapter")
	}
	if sess != nil {
		t.Error("no session may be returned alongside an error")
	}
	// The message has to name what failed; a bare transport error would send
	// the reader looking for a network problem that is not there.
	if !strings.Contains(err.Error(), "compile pack") {
		t.Errorf("the error should name pack compilation, got %q", err)
	}
	if !strings.Contains(err.Error(), "arena.yaml") {
		t.Errorf("the error should name the config it came from, got %q", err)
	}
}

// TestSession_PlanFreshnessTracksThePackAndEnv covers NewSession, Close,
// PackChecksum, SavePlan, LoadPlan and PlanIsFresh together, because
// freshness is the property they exist to provide.
//
// A saved plan is what `deploy apply` executes without re-planning. If
// freshness were judged loosely, apply would run a plan computed against a
// different pack or a different environment — the worst possible outcome for
// a deploy tool, because it succeeds.
func TestSession_PlanFreshnessTracksThePackAndEnv(t *testing.T) {
	dir := t.TempDir()
	store := deploy.NewStateStore(dir)
	opts := Options{ProjectDir: dir, Env: "staging"}
	dep := &arenaconfig.DeployConfig{Provider: "acme"}

	var closed bool
	sess := NewSession(opts, &arenaconfig.Config{}, dep, nil, store,
		[]byte(`{"pack":"v1"}`), "{}", func() error { closed = true; return nil })

	if sess.ProviderName != "acme" {
		t.Errorf("ProviderName = %q", sess.ProviderName)
	}
	if sess.Env != "staging" {
		t.Errorf("Env = %q, want the resolved option", sess.Env)
	}

	checksum := sess.PackChecksum()
	if checksum == "" {
		t.Fatal("a pack must produce a checksum; freshness depends on it")
	}

	plan := &deploy.PlanResponse{}
	if err := sess.SavePlan(plan, &deploy.PlanRequest{}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}

	saved, err := sess.LoadPlan()
	if err != nil {
		t.Fatalf("LoadPlan: %v", err)
	}
	if saved == nil {
		t.Fatal("a saved plan must load back")
	}
	if !sess.PlanIsFresh(saved) {
		t.Error("a plan saved from this session must be fresh for it")
	}

	// A different pack invalidates it.
	stale := *saved
	stale.PackChecksum = "different"
	if sess.PlanIsFresh(&stale) {
		t.Error("a plan for another pack must not be fresh")
	}

	// So does a different environment — same pack, different target.
	otherEnv := *saved
	otherEnv.Environment = "production"
	if sess.PlanIsFresh(&otherEnv) {
		t.Error("a plan for another environment must not be fresh")
	}

	if sess.PlanIsFresh(nil) {
		t.Error("no plan is not a fresh plan")
	}

	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !closed {
		t.Error("Close must invoke the closer it was given")
	}
}

// TestSession_CloseWithoutACloserIsSafe pins the nil branch: a Session built
// for a dry run has nothing to close, and that must not be an error the
// caller has to special-case.
func TestSession_CloseWithoutACloserIsSafe(t *testing.T) {
	sess := NewSession(Options{}, &arenaconfig.Config{},
		&arenaconfig.DeployConfig{Provider: "acme"}, nil, nil, nil, "", nil)
	if err := sess.Close(); err != nil {
		t.Fatalf("Close with no closer: %v", err)
	}
}

// TestSession_LoadPlanWithNoSavedPlan pins the empty case: no plan is nil and
// no error, so a caller can tell "nothing saved" from "the store is broken".
func TestSession_LoadPlanWithNoSavedPlan(t *testing.T) {
	sess := NewSession(Options{}, &arenaconfig.Config{},
		&arenaconfig.DeployConfig{Provider: "acme"}, nil,
		deploy.NewStateStore(t.TempDir()), nil, "", nil)

	saved, err := sess.LoadPlan()
	if err != nil {
		t.Fatalf("LoadPlan on an empty store: %v", err)
	}
	if saved != nil {
		t.Errorf("expected no plan, got %+v", saved)
	}
}

// fakeProvider is a deploy.Provider that records what it was asked and returns
// what the test tells it to. The adapter is a subprocess in production, so
// this is the only way to reach Session's Plan/Apply/Status paths at all.
type fakeProvider struct {
	info      *deploy.ProviderInfo
	plan      *deploy.PlanResponse
	planErr   error
	applyErr  error
	status    *deploy.StatusResponse
	statusErr error

	sawPlanReq   *deploy.PlanRequest
	sawStatusReq *deploy.StatusRequest
	appliedState string
}

func (f *fakeProvider) GetProviderInfo(context.Context) (*deploy.ProviderInfo, error) {
	return f.info, nil
}

func (f *fakeProvider) ValidateConfig(
	context.Context, *deploy.ValidateRequest,
) (*deploy.ValidateResponse, error) {
	return &deploy.ValidateResponse{}, nil
}

func (f *fakeProvider) Plan(_ context.Context, req *deploy.PlanRequest) (*deploy.PlanResponse, error) {
	f.sawPlanReq = req
	return f.plan, f.planErr
}

func (f *fakeProvider) Apply(
	_ context.Context, _ *deploy.PlanRequest, _ deploy.ApplyCallback,
) (string, error) {
	return f.appliedState, f.applyErr
}

func (f *fakeProvider) Destroy(context.Context, *deploy.DestroyRequest, deploy.DestroyCallback) error {
	return nil
}

func (f *fakeProvider) Status(_ context.Context, req *deploy.StatusRequest) (*deploy.StatusResponse, error) {
	f.sawStatusReq = req
	return f.status, f.statusErr
}

func (f *fakeProvider) Import(context.Context, *deploy.ImportRequest) (*deploy.ImportResponse, error) {
	return &deploy.ImportResponse{}, nil
}

func newTestSession(t *testing.T, client deploy.Provider) (*Session, *deploy.StateStore) {
	t.Helper()
	dir := t.TempDir()
	store := deploy.NewStateStore(dir)
	sess := NewSession(Options{ProjectDir: dir, Env: "staging"}, &arenaconfig.Config{},
		&arenaconfig.DeployConfig{Provider: "acme"}, client, store,
		[]byte(`{"pack":"v1"}`), `{"cfg":true}`, nil)
	return sess, store
}

// TestSession_StatusPassesPriorStateThrough covers Status. The adapter is
// stateless between invocations, so the prior state the session hands it is
// the only thing that lets it report drift rather than describing every
// resource as new.
func TestSession_StatusPassesPriorStateThrough(t *testing.T) {
	fake := &fakeProvider{status: &deploy.StatusResponse{State: "refreshed"}}
	sess, store := newTestSession(t, fake)

	st := deploy.NewState("acme", "staging", "", sess.PackChecksum(), "1.0")
	st.State = "saved-state"
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}

	got, err := sess.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got.State != "refreshed" {
		t.Errorf("State = %q", got.State)
	}
	if fake.sawStatusReq == nil {
		t.Fatal("the adapter was never asked")
	}
	if fake.sawStatusReq.PriorState != "saved-state" {
		t.Errorf("PriorState = %q, want the stored state", fake.sawStatusReq.PriorState)
	}
	if fake.sawStatusReq.Environment != "staging" {
		t.Errorf("Environment = %q", fake.sawStatusReq.Environment)
	}
	if fake.sawStatusReq.DeployConfig != `{"cfg":true}` {
		t.Errorf("DeployConfig = %q", fake.sawStatusReq.DeployConfig)
	}
}

// TestSession_PlanBuildsARequestAndCallsTheAdapter covers Plan and PlanRequest.
func TestSession_PlanBuildsARequestAndCallsTheAdapter(t *testing.T) {
	fake := &fakeProvider{
		plan:   &deploy.PlanResponse{},
		status: &deploy.StatusResponse{},
	}
	sess, _ := newTestSession(t, fake)

	plan, req, err := sess.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan == nil || req == nil {
		t.Fatal("Plan must return both the response and the request it sent")
	}
	if fake.sawPlanReq == nil {
		t.Fatal("the adapter's Plan was never called")
	}
}

// TestSession_PlanSurfacesAdapterFailure pins that a failed plan is an error
// rather than an empty plan — an empty plan reads as "nothing to do", which is
// the one wrong answer a deploy tool must never give by accident.
func TestSession_PlanSurfacesAdapterFailure(t *testing.T) {
	fake := &fakeProvider{planErr: errors.New("adapter exploded"), status: &deploy.StatusResponse{}}
	sess, _ := newTestSession(t, fake)

	plan, _, err := sess.Plan(context.Background())
	if err == nil {
		t.Fatal("expected the adapter failure to surface")
	}
	if plan != nil {
		t.Error("no plan may be returned alongside an error")
	}
	if !strings.Contains(err.Error(), "plan failed") {
		t.Errorf("error should say what failed, got %q", err)
	}
}

// TestSession_ApplyPersistsStateAndClearsThePlan covers Apply. Saving the
// adapter's returned state is what makes the next plan a diff rather than a
// fresh create; clearing the saved plan is what stops a second apply replaying
// a plan that has already run.
func TestSession_ApplyPersistsStateAndClearsThePlan(t *testing.T) {
	fake := &fakeProvider{
		appliedState: "post-apply-state",
		info:         &deploy.ProviderInfo{Version: "2.3.4"},
	}
	sess, store := newTestSession(t, fake)

	if err := sess.SavePlan(&deploy.PlanResponse{}, &deploy.PlanRequest{}); err != nil {
		t.Fatal(err)
	}

	if err := sess.Apply(context.Background(), &deploy.PlanRequest{}, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	saved, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if saved == nil {
		t.Fatal("apply must persist state")
	}
	if saved.State != "post-apply-state" {
		t.Errorf("State = %q, want the adapter's returned state", saved.State)
	}
	if saved.AdapterVersion != "2.3.4" {
		t.Errorf("AdapterVersion = %q, want the version the adapter reported", saved.AdapterVersion)
	}

	plan, err := sess.LoadPlan()
	if err != nil {
		t.Fatal(err)
	}
	if plan != nil {
		t.Error("a plan that has been applied must not remain saved for replay")
	}
}

// TestSession_ApplyFailureLeavesNoState pins that a failed apply does not
// record success — state written for an apply that errored would make the next
// plan diff against resources that were never created.
func TestSession_ApplyFailureLeavesNoState(t *testing.T) {
	fake := &fakeProvider{applyErr: errors.New("permission denied")}
	sess, store := newTestSession(t, fake)

	err := sess.Apply(context.Background(), &deploy.PlanRequest{}, nil)
	if err == nil {
		t.Fatal("expected the apply failure to surface")
	}
	if !strings.Contains(err.Error(), "apply failed") {
		t.Errorf("error should say what failed, got %q", err)
	}

	saved, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if saved != nil {
		t.Errorf("a failed apply must not persist state, got %+v", saved)
	}
}

// TestSession_PlanRefreshesStaleStateBeforeDiffing covers PlanRequest's refresh
// branch. The stored state is a snapshot from the last apply; anything changed
// out-of-band since then is invisible to it. Planning against the stored copy
// rather than a live refresh produces a diff that omits real drift — the plan
// looks clean and the drift survives the apply.
func TestSession_PlanRefreshesStaleStateBeforeDiffing(t *testing.T) {
	fake := &fakeProvider{
		plan:   &deploy.PlanResponse{},
		status: &deploy.StatusResponse{State: "live-state"},
	}
	sess, store := newTestSession(t, fake)

	st := deploy.NewState("acme", "staging", "", sess.PackChecksum(), "1.0")
	st.State = "stale-state"
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}

	req, err := sess.PlanRequest(context.Background())
	if err != nil {
		t.Fatalf("PlanRequest: %v", err)
	}
	if req.PriorState != "live-state" {
		t.Errorf("PriorState = %q, want the refreshed state, not the stored one", req.PriorState)
	}
	if req.Environment != "staging" || req.DeployConfig != `{"cfg":true}` {
		t.Errorf("request not populated from the session: %+v", req)
	}
	if req.PackJSON != `{"pack":"v1"}` {
		t.Errorf("PackJSON = %q", req.PackJSON)
	}

	// The refresh is written back, so the next run starts from it and stamps
	// when it happened.
	reloaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.State != "live-state" {
		t.Errorf("refreshed state not persisted, got %q", reloaded.State)
	}
	if reloaded.LastRefreshed == "" {
		t.Error("a refresh must be timestamped")
	}
}

// TestSession_PlanKeepsStoredStateWhenRefreshFails pins the soft-fail: an
// adapter that cannot report status must not lose the state we already have.
// Planning with an empty prior state would describe every existing resource as
// a create.
func TestSession_PlanKeepsStoredStateWhenRefreshFails(t *testing.T) {
	fake := &fakeProvider{
		plan:      &deploy.PlanResponse{},
		statusErr: errors.New("adapter unreachable"),
	}
	sess, store := newTestSession(t, fake)

	st := deploy.NewState("acme", "staging", "", sess.PackChecksum(), "1.0")
	st.State = "stored-state"
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}

	req, err := sess.PlanRequest(context.Background())
	if err != nil {
		t.Fatalf("PlanRequest must not fail when only the refresh did: %v", err)
	}
	if req.PriorState != "stored-state" {
		t.Errorf("PriorState = %q, want the stored state retained", req.PriorState)
	}
}

// TestOpen_ReachesConnectWithAPreCompiledPack covers Open past pack
// resolution. With PackFile set there is nothing to compile, so Open gets as
// far as dialling the adapter — which is where a deploy against an
// uninstalled provider must fail, having created nothing.
func TestOpen_ReachesConnectWithAPreCompiledPack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
    provider: nonexistent-xyz
    config: {}
`)
	packPath := filepath.Join(dir, "test.pack.json")
	if err := os.WriteFile(packPath, []byte(`{"prompts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	sess, err := Open(context.Background(), Options{
		ConfigPath: configPath, ProjectDir: dir, PackFile: packPath,
	})
	if err == nil {
		if sess != nil {
			_ = sess.Close()
		}
		t.Fatal("expected Open to fail with no adapter installed")
	}
	if sess != nil {
		t.Error("no session may be returned alongside an error")
	}
	// Past compilation, so the failure must not be about the pack.
	if strings.Contains(err.Error(), "compile pack") {
		t.Errorf("PackFile should have skipped compilation, got %q", err)
	}
}

// TestOpen_MissingPackFileIsReported pins the other PackFile branch: a path
// that does not exist has to name itself, not surface as an empty pack.
func TestOpen_MissingPackFileIsReported(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
    provider: nonexistent-xyz
    config: {}
`)

	_, err := Open(context.Background(), Options{
		ConfigPath: configPath, ProjectDir: dir,
		PackFile: filepath.Join(dir, "absent.pack.json"),
	})
	if err == nil {
		t.Fatal("expected an error for a missing pack file")
	}
	if !strings.Contains(err.Error(), "absent.pack.json") {
		t.Errorf("the error should name the missing pack, got %q", err)
	}
}
