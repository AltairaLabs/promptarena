package flow

import (
	"context"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

type fakeProvider struct {
	plan    *deploy.PlanResponse
	events  []*deploy.ApplyEvent
	status  *deploy.StatusResponse
	applied bool
}

func (f *fakeProvider) GetProviderInfo(context.Context) (*deploy.ProviderInfo, error) {
	return &deploy.ProviderInfo{Name: "fake", Version: "9.9.9", Capabilities: []string{deploy.LoginCapability}}, nil
}
func (f *fakeProvider) ValidateConfig(context.Context, *deploy.ValidateRequest) (*deploy.ValidateResponse, error) {
	return &deploy.ValidateResponse{Valid: true}, nil
}
func (f *fakeProvider) Plan(context.Context, *deploy.PlanRequest) (*deploy.PlanResponse, error) {
	return f.plan, nil
}
func (f *fakeProvider) Apply(_ context.Context, _ *deploy.PlanRequest, cb deploy.ApplyCallback) (string, error) {
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
	return f.status, nil
}
func (f *fakeProvider) Import(context.Context, *deploy.ImportRequest) (*deploy.ImportResponse, error) {
	return &deploy.ImportResponse{}, nil
}

func newTestSession(t *testing.T, fp *fakeProvider) *Session {
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
