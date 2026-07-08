package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
)

// fakeDeployProvider is a deploy.Provider test double modeled on the
// unexported fakeProvider in arena/deploy/flow/session_test.go (reimplemented
// here since that type lives in a different package). Plan/Status return
// canned responses; Apply replays events through the callback exactly as a
// real adapter's Apply does and counts invocations so the integration test
// can assert Apply ran exactly once.
type fakeDeployProvider struct {
	mu         sync.Mutex
	plan       *deploy.PlanResponse
	events     []*deploy.ApplyEvent
	status     *deploy.StatusResponse
	applyCalls int
}

func (f *fakeDeployProvider) GetProviderInfo(context.Context) (*deploy.ProviderInfo, error) {
	return &deploy.ProviderInfo{Name: "fake", Version: "9.9.9"}, nil
}

func (f *fakeDeployProvider) ValidateConfig(context.Context, *deploy.ValidateRequest) (*deploy.ValidateResponse, error) {
	return &deploy.ValidateResponse{Valid: true}, nil
}

func (f *fakeDeployProvider) Plan(context.Context, *deploy.PlanRequest) (*deploy.PlanResponse, error) {
	return f.plan, nil
}

func (f *fakeDeployProvider) Apply(_ context.Context, _ *deploy.PlanRequest, cb deploy.ApplyCallback) (string, error) {
	f.mu.Lock()
	f.applyCalls++
	f.mu.Unlock()
	for _, e := range f.events {
		if err := cb(e); err != nil {
			return "", err
		}
	}
	return "opaque-state", nil
}

func (f *fakeDeployProvider) Destroy(context.Context, *deploy.DestroyRequest, deploy.DestroyCallback) error {
	return nil
}

func (f *fakeDeployProvider) Status(context.Context, *deploy.StatusRequest) (*deploy.StatusResponse, error) {
	return f.status, nil
}

func (f *fakeDeployProvider) Import(context.Context, *deploy.ImportRequest) (*deploy.ImportResponse, error) {
	return &deploy.ImportResponse{}, nil
}

func (f *fakeDeployProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.applyCalls
}

// newDeployPageWithSession builds a DeployPage with a pre-injected *flow.Session
// and *flow.Preflight, bypassing flow.CheckPreflight and (via the p.sess seam
// added to startPlan) flow.Open. It exists so integration tests can drive the
// whole wizard state machine against a fake deploy.Provider without spawning a
// real adapter subprocess. Test-only: production code always goes through
// NewDeployPage + Activate's real preflight probe and startPlan's flow.Open.
func newDeployPageWithSession(ctx *AppContext, sess *flow.Session, pf *flow.Preflight) *DeployPage {
	p := NewDeployPage(ctx).(*DeployPage)
	p.pf = pf
	p.sess = sess
	return p
}

// runCmd executes cmd, recursing into any tea.BatchMsg it returns so every
// sub-command actually runs. This is what launches startPlan/startApply/
// startStatus's goroutines when driving the wizard through Update instead of
// calling them directly: Update returns tea.Batch(startX(), spinner.Tick), and
// a bare tea.Cmd is inert (its goroutine never starts) until invoked — a real
// tea.Program's event loop would invoke every leaf of that batch itself.
func runCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			runCmd(c)
		}
	}
}

// msgDrainer replays messages a background goroutine delivers via sink.send
// through Update, in arrival order, once a message satisfying match has
// arrived. It only ever replays each message once (tracked via next), so
// repeated drainUntil calls compose to drive the wizard forward one
// background step (plan, apply burst, status) at a time — mirroring the
// waitForLoginDone / sink-polling idiom used elsewhere in this package, but
// generalized to feed every drained message back into Update rather than just
// inspecting the sink.
type msgDrainer struct {
	sink *loginMsgSink
	next int
}

func (d *msgDrainer) drainUntil(t *testing.T, p *DeployPage, match func(tea.Msg) bool) *DeployPage {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		msgs := d.sink.snapshot()
		pending := msgs[d.next:]
		found := false
		for _, m := range pending {
			if match(m) {
				found = true
				break
			}
		}
		if found {
			for _, m := range pending {
				np, _ := p.Update(m)
				p = np.(*DeployPage)
			}
			d.next = len(msgs)
			return p
		}
		if time.Now().After(deadline) {
			types := make([]string, len(msgs))
			for i, m := range msgs {
				types[i] = fmt.Sprintf("%T", m)
			}
			t.Fatalf("expected message not received within timeout; sink has: %v", types)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestDeployWizard_EndToEndWithFakeSession drives the entire DeployPage wizard
// state machine — preflight → planning → plan → confirm → applying →
// applyResult → status — through Update, using a *flow.Session pre-injected
// via newDeployPageWithSession and backed by fakeDeployProvider instead of a
// real adapter subprocess. It exercises the real startPlan/startApply/
// startStatus goroutines (launched via runCmd) against that fake session, so
// it proves the wiring end-to-end rather than merely asserting on
// hand-synthesized messages. This is the safety net Task 5.4 adds: without it,
// nothing exercised the whole state machine in one pass.
func TestDeployWizard_EndToEndWithFakeSession(t *testing.T) {
	ctx := newMenuTestCtx(t)
	dir := t.TempDir()

	fp := &fakeDeployProvider{
		plan: &deploy.PlanResponse{
			Summary: "1 to add",
			Changes: []deploy.ResourceChange{
				{Type: "agent_runtime", Name: "bot", Action: deploy.ActionCreate},
			},
		},
		events: []*deploy.ApplyEvent{
			{Type: "resource", Message: "created agent_runtime.bot", Resource: &deploy.ResourceResult{
				Type: "agent_runtime", Name: "bot", Action: deploy.ActionCreate, Status: "created",
			}},
			{Type: "complete", Message: "apply finished"},
		},
		status: &deploy.StatusResponse{
			Status: "deployed",
			Resources: []deploy.ResourceStatus{
				{Type: "agent_runtime", Name: "bot", Status: "healthy"},
			},
		},
	}

	dc := &arenaconfig.DeployConfig{Provider: "fake", Config: map[string]interface{}{}}
	sess := flow.NewSession(
		flow.Options{ProjectDir: dir},
		&arenaconfig.Config{Deploy: dc},
		dc,
		fp,
		deploy.NewStateStore(dir),
		[]byte(`{"pack":true}`),
		`{}`,
		func() error { return nil },
	)

	pf := &flow.Preflight{Provider: "fake", Env: flow.DefaultEnv, AdapterFound: true, Authenticated: true}

	p := newDeployPageWithSession(ctx, sess, pf)
	p.opts.ProjectDir = dir // startApply's flow.Lock(projectDir) needs a writable dir

	sink := &loginMsgSink{}
	p.send = sink.send
	drainer := &msgDrainer{sink: sink}

	if p.state != deployStatePreflight {
		t.Fatalf("initial state = %v, want deployStatePreflight", p.state)
	}

	// preflight-ready -> 'p' starts planning against the injected fake session.
	np, cmd := p.Update(keyRunes("p"))
	p = np.(*DeployPage)
	if p.state != deployStatePlanning {
		t.Fatalf("state after 'p' = %v, want deployStatePlanning", p.state)
	}
	runCmd(cmd)

	p = drainer.drainUntil(t, p, func(m tea.Msg) bool { _, ok := m.(planReadyMsg); return ok })
	if p.state != deployStatePlan {
		t.Fatalf("state after planReadyMsg = %v, want deployStatePlan", p.state)
	}
	if !p.planHasChanges() {
		t.Fatal("expected the injected plan to have changes")
	}

	// 'a' advances to confirm.
	np, _ = p.Update(keyRunes("a"))
	p = np.(*DeployPage)
	if p.state != deployStateConfirm {
		t.Fatalf("state after 'a' = %v, want deployStateConfirm", p.state)
	}

	// confirm (default env, 'y') starts apply against the injected session.
	np, cmd = p.Update(keyRunes("y"))
	p = np.(*DeployPage)
	if p.state != deployStateApplying {
		t.Fatalf("state after 'y' = %v, want deployStateApplying", p.state)
	}
	runCmd(cmd)

	p = drainer.drainUntil(t, p, func(m tea.Msg) bool { _, ok := m.(applyDoneMsg); return ok })
	if p.state != deployStateApplyResult {
		t.Fatalf("state after apply burst = %v, want deployStateApplyResult (err=%v)", p.state, p.err)
	}
	if got := fp.callCount(); got != 1 {
		t.Fatalf("fake provider Apply called %d times, want 1", got)
	}
	if len(p.applyRows) != 1 || p.applyRows[0].Symbol != "+" {
		t.Fatalf("applyRows = %+v, want one created resource", p.applyRows)
	}

	// 's' advances to status and fetches it from the injected session.
	np, cmd = p.Update(keyRunes("s"))
	p = np.(*DeployPage)
	if p.state != deployStateStatus {
		t.Fatalf("state after 's' = %v, want deployStateStatus", p.state)
	}
	runCmd(cmd)

	p = drainer.drainUntil(t, p, func(m tea.Msg) bool { _, ok := m.(statusReadyMsg); return ok })

	// Terminal state assertions.
	if p.state != deployStateStatus {
		t.Fatalf("terminal state = %v, want deployStateStatus", p.state)
	}
	if p.status == nil || p.status.Status != "deployed" {
		t.Fatalf("status = %+v, want a deployed status from the fake session", p.status)
	}
	if len(p.status.Resources) != 1 || p.status.Resources[0].Status != "healthy" {
		t.Fatalf("status resources = %+v, want one healthy resource", p.status.Resources)
	}

	p.SetSize(80, 24)
	out := stripANSI(p.View())
	if !strings.Contains(out, "bot") {
		t.Fatalf("expected the fake's healthy resource in the status view:\n%s", out)
	}
	if !strings.Contains(out, "✓") {
		t.Fatalf("expected a healthy status symbol in the status view:\n%s", out)
	}

	if got := fp.callCount(); got != 1 {
		t.Fatalf("fake provider Apply called %d times by the end of the run, want 1", got)
	}
}
