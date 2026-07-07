package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
	"github.com/AltairaLabs/promptarena/arena/tui/viewmodels"
)

// loginMsgSink is a thread-safe collector for the messages a DeployPage
// delivers via its send func during a background login, mirroring the
// msgSink idiom in runpage_test.go.
type loginMsgSink struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (s *loginMsgSink) send(msg tea.Msg) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
}

func (s *loginMsgSink) snapshot() []tea.Msg {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]tea.Msg, len(s.msgs))
	copy(out, s.msgs)
	return out
}

// waitForLoginDone polls sink until a loginDoneMsg lands or the timeout
// elapses, returning it.
func waitForLoginDone(t *testing.T, sink *loginMsgSink) loginDoneMsg {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range sink.snapshot() {
			if done, ok := m.(loginDoneMsg); ok {
				return done
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("loginDoneMsg not received within timeout")
	return loginDoneMsg{}
}

// TestNewDeployPage_TitleAndInitialState verifies that NewDeployPage builds a
// *DeployPage titled "Deploy" that starts in the preflight state.
func TestNewDeployPage_TitleAndInitialState(t *testing.T) {
	ctx := newMenuTestCtx(t)
	p := NewDeployPage(ctx)
	if p.Title() != "Deploy" {
		t.Fatalf("Title() = %q, want Deploy", p.Title())
	}
	dp, ok := p.(*DeployPage)
	if !ok {
		t.Fatal("NewDeployPage did not return *DeployPage")
	}
	if dp.state != deployStatePreflight {
		t.Fatalf("initial state = %d, want preflight", dp.state)
	}
}

// keyRunes builds a tea.KeyMsg for the given rune-key string (e.g. "p").
func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// TestPreflight_AdapterMissingBlocksPlan verifies the install command is
// rendered and that pressing 'p' cannot advance past preflight when the
// adapter isn't installed.
func TestPreflight_AdapterMissingBlocksPlan(t *testing.T) {
	p := &DeployPage{state: deployStatePreflight, pf: &flow.Preflight{
		Provider: "omnia", Env: "default", AdapterFound: false,
		InstallCommand: "promptarena deploy adapter install omnia",
	}}
	out := stripANSI(p.viewPreflight(80))
	if !strings.Contains(out, "promptarena deploy adapter install omnia") {
		t.Fatalf("missing install command:\n%s", out)
	}
	// Pressing 'p' with no adapter must not advance.
	np, _ := p.handlePreflightKey(keyRunes("p"))
	if np.(*DeployPage).state != deployStatePreflight {
		t.Fatal("plan should be blocked when adapter missing")
	}
}

// TestPreflight_ProductionShownLoudly verifies a production environment is
// surfaced loudly in the preflight view.
func TestPreflight_ProductionShownLoudly(t *testing.T) {
	p := &DeployPage{state: deployStatePreflight, pf: &flow.Preflight{
		Provider: "omnia", Env: "production", AdapterFound: true, Authenticated: true, AdapterVersion: "1.0.0",
	}}
	out := stripANSI(p.viewPreflight(80))
	if !strings.Contains(strings.ToUpper(out), "PRODUCTION") {
		t.Fatalf("production not surfaced:\n%s", out)
	}
}

// TestPreflight_LoginKeyGatedOnSupport verifies 'l' only transitions to the
// login state when the adapter supports login and isn't authenticated yet.
func TestPreflight_LoginKeyGatedOnSupport(t *testing.T) {
	p := &DeployPage{state: deployStatePreflight, pf: &flow.Preflight{
		Provider: "omnia", Env: "default", AdapterFound: true, SupportsLogin: false,
	}}
	np, _ := p.handlePreflightKey(keyRunes("l"))
	if np.(*DeployPage).state != deployStatePreflight {
		t.Fatal("login should be blocked when adapter doesn't support login")
	}

	p2 := &DeployPage{state: deployStatePreflight, pf: &flow.Preflight{
		Provider: "omnia", Env: "default", AdapterFound: true, SupportsLogin: true, Authenticated: false,
	}}
	np2, _ := p2.handlePreflightKey(keyRunes("l"))
	if np2.(*DeployPage).state != deployStateLogin {
		t.Fatal("login should be allowed when supported and not yet authenticated")
	}
}

// TestPreflight_PlanKeyRequiresReady verifies 'p' only transitions to
// planning once pf.Ready() is true.
func TestPreflight_PlanKeyRequiresReady(t *testing.T) {
	p := &DeployPage{state: deployStatePreflight, pf: &flow.Preflight{
		Provider: "omnia", Env: "default", AdapterFound: true, Authenticated: false,
	}}
	np, _ := p.handlePreflightKey(keyRunes("p"))
	if np.(*DeployPage).state != deployStatePreflight {
		t.Fatal("plan should be blocked when not authenticated")
	}

	p2 := &DeployPage{state: deployStatePreflight, pf: &flow.Preflight{
		Provider: "omnia", Env: "default", AdapterFound: true, Authenticated: true,
	}}
	np2, _ := p2.handlePreflightKey(keyRunes("p"))
	if np2.(*DeployPage).state != deployStatePlanning {
		t.Fatal("plan should be allowed once preflight is ready")
	}
}

// TestPreflight_RetryKeyReRunsPreflight verifies 'r' returns a non-nil
// command (re-running the preflight probe) without changing state.
func TestPreflight_RetryKeyReRunsPreflight(t *testing.T) {
	p := &DeployPage{state: deployStatePreflight, pf: &flow.Preflight{
		Provider: "omnia", Env: "default", AdapterFound: false,
	}}
	np, cmd := p.handlePreflightKey(keyRunes("r"))
	if cmd == nil {
		t.Fatal("expected a re-run command from 'r'")
	}
	if np.(*DeployPage).state != deployStatePreflight {
		t.Fatal("retry should keep state at preflight")
	}
}

// TestGoldenDeployPreflight_Ready captures a stable snapshot of the preflight
// view once the adapter is installed and authenticated.
func TestGoldenDeployPreflight_Ready(t *testing.T) {
	p := &DeployPage{state: deployStatePreflight, pf: &flow.Preflight{
		Provider: "omnia", Env: "production", AdapterFound: true,
		AdapterVersion: "1.0.0", Authenticated: true, SupportsLogin: true,
	}}
	p.SetSize(80, 24)
	out := stripANSI(p.View())
	teatest.RequireEqualOutput(t, []byte(out))
}

// TestLogin_StatusMsgUpdatesText verifies loginStatusMsg updates the
// displayed status text (and, when it carries an authorize URL, the
// persisted headless-fallback URL).
func TestLogin_StatusMsgUpdatesText(t *testing.T) {
	p := &DeployPage{state: deployStateLogin, pf: &flow.Preflight{Provider: "omnia"}}

	np, _ := p.Update(loginStatusMsg{text: "Opening your browser to authenticate…"})
	dp := np.(*DeployPage)
	if dp.loginStatus != "Opening your browser to authenticate…" {
		t.Fatalf("loginStatus = %q, want the status text", dp.loginStatus)
	}

	np2, _ := dp.Update(loginStatusMsg{text: "Waiting for authorization…", url: "https://auth.example/x"})
	dp2 := np2.(*DeployPage)
	if dp2.loginStatus != "Waiting for authorization…" {
		t.Fatalf("loginStatus = %q, want the latest status text", dp2.loginStatus)
	}
	if dp2.loginURL != "https://auth.example/x" {
		t.Fatalf("loginURL = %q, want the authorize URL to persist", dp2.loginURL)
	}
}

// TestLogin_DoneSuccess_ReturnsToPreflightAndReprobes verifies a nil-error
// loginDoneMsg transitions back to preflight and issues a re-probe command
// (so the wizard reflects auth ✓ on the next render).
func TestLogin_DoneSuccess_ReturnsToPreflightAndReprobes(t *testing.T) {
	p := &DeployPage{state: deployStateLogin, pf: &flow.Preflight{Provider: "omnia"}}
	np, cmd := p.Update(loginDoneMsg{err: nil})
	dp := np.(*DeployPage)
	if dp.state != deployStatePreflight {
		t.Fatalf("state = %v, want deployStatePreflight", dp.state)
	}
	if cmd == nil {
		t.Fatal("expected a re-probe command after a successful login")
	}
}

// TestLogin_DoneError_RecoverableAtPreflight verifies a failed loginDoneMsg
// (e.g. a login timeout) surfaces the error but returns to preflight rather
// than dead-ending the wizard in deployStateError.
func TestLogin_DoneError_RecoverableAtPreflight(t *testing.T) {
	p := &DeployPage{state: deployStateLogin, pf: &flow.Preflight{Provider: "omnia"}}
	wantErr := errors.New("login timed out waiting for the browser callback")
	np, _ := p.Update(loginDoneMsg{err: wantErr})
	dp := np.(*DeployPage)
	if dp.state != deployStatePreflight {
		t.Fatalf("state = %v, want deployStatePreflight (recoverable)", dp.state)
	}
	if dp.loginErr == nil || dp.loginErr.Error() != wantErr.Error() {
		t.Fatalf("loginErr = %v, want %v", dp.loginErr, wantErr)
	}
}

// TestLogin_CancelKeyReturnsToPreflight verifies 'c' in the login state
// cancels back to preflight.
func TestLogin_CancelKeyReturnsToPreflight(t *testing.T) {
	canceled := false
	p := &DeployPage{
		state:       deployStateLogin,
		pf:          &flow.Preflight{Provider: "omnia"},
		loginCancel: func() { canceled = true },
	}
	np, _ := p.handleLoginKey(keyRunes("c"))
	dp := np.(*DeployPage)
	if dp.state != deployStatePreflight {
		t.Fatalf("state = %v, want deployStatePreflight", dp.state)
	}
	if !canceled {
		t.Fatal("expected the login context to be canceled")
	}
}

// TestDeployPage_StartLogin_WiresGoroutineToSend drives the real
// flow.Login goroutine wiring (not a fake) with a provider that has no
// adapter installed, so Connect fails fast without any network/browser
// round-trip. This exercises the actual cmd-starts-goroutine-calls-send
// plumbing that the message-only tests above can't reach.
func TestDeployPage_StartLogin_WiresGoroutineToSend(t *testing.T) {
	sink := &loginMsgSink{}
	p := &DeployPage{
		pf:   &flow.Preflight{Provider: "no-such-adapter-xyz", SupportsLogin: true},
		send: sink.send,
	}
	cmd := p.startLogin()
	if cmd == nil {
		t.Fatal("expected a non-nil cmd from startLogin")
	}
	cmd() // starts the goroutine; the cmd itself returns nil (see brief note)

	done := waitForLoginDone(t, sink)
	if done.err == nil || !strings.Contains(done.err.Error(), "adapter not found") {
		t.Fatalf("loginDoneMsg.err = %v, want an adapter-not-found error", done.err)
	}
}

// TestGoldenDeployLogin_Waiting captures a stable snapshot of the login
// screen mid-flow: spinner, latest status, and the headless-fallback URL.
func TestGoldenDeployLogin_Waiting(t *testing.T) {
	p := &DeployPage{
		state:       deployStateLogin,
		pf:          &flow.Preflight{Provider: "omnia"},
		loginStatus: "Waiting for authorization…",
		loginURL:    "https://auth.example/oauth/authorize?state=abc123",
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
	p.SetSize(80, 24)
	out := stripANSI(p.View())
	teatest.RequireEqualOutput(t, []byte(out))
}

// TestDeployPage_Close_CancelsLoginGoroutine verifies DeployPage implements
// Closeable and that Close cancels an in-flight login. App.pop() calls Close
// on any popped page implementing Closeable (see TestApp_PopClosesPage in
// app_test.go and ChatPage.Close's voice-driver cancellation) — this is what
// stops the flow.Login goroutine (and its loopback OAuth callback server) when
// the operator backs out via esc, which App's global esc handler processes
// before DeployPage.handleLoginKey ever sees the key.
func TestDeployPage_Close_CancelsLoginGoroutine(t *testing.T) {
	canceled := false
	p := &DeployPage{
		state:       deployStateLogin,
		pf:          &flow.Preflight{Provider: "omnia"},
		loginCancel: func() { canceled = true },
	}
	var _ Closeable = p // compile-time assertion that DeployPage implements Closeable

	p.Close()

	if !canceled {
		t.Fatal("expected Close to cancel the in-flight login")
	}
	if p.loginCancel != nil {
		t.Fatal("expected Close to nil out loginCancel")
	}
}

// TestDeployPage_Close_NoLoginInFlight verifies Close is a safe no-op when
// there is no in-flight login (e.g. closed from the preflight state, or a
// page that never started a login at all).
func TestDeployPage_Close_NoLoginInFlight(t *testing.T) {
	p := &DeployPage{state: deployStatePreflight}
	p.Close() // must not panic
}

// TestLogin_StaleDoneMsgIgnored_CurrentGenProcessed exercises the loginGen
// guard against a cancel-then-retry race: pressing 'c' cancels attempt 0 and
// 'l' starts attempt 1 (bumping loginGen to 1). Attempt 0's goroutine can
// still deliver a late loginDoneMsg{gen: 0} — that must be ignored rather
// than wiping attempt 1's loginCancel/state and bouncing the operator out
// with a stale error. A loginDoneMsg carrying the current generation must
// still be processed normally.
func TestLogin_StaleDoneMsgIgnored_CurrentGenProcessed(t *testing.T) {
	currentCanceled := false
	p := &DeployPage{
		state:       deployStateLogin,
		pf:          &flow.Preflight{Provider: "omnia"},
		loginGen:    1, // attempt 0 was canceled; attempt 1 (current) is in flight
		loginCancel: func() { currentCanceled = true },
	}

	// Stale: attempt 0's loginDoneMsg arrives after the retry.
	np, cmd := p.Update(loginDoneMsg{
		err: errors.New("login timed out waiting for the browser callback"),
		gen: 0,
	})
	dp := np.(*DeployPage)
	if cmd != nil {
		t.Fatal("expected no cmd from a stale loginDoneMsg")
	}
	if dp.state != deployStateLogin {
		t.Fatalf("stale loginDoneMsg must not change state, got %v", dp.state)
	}
	if dp.loginCancel == nil {
		t.Fatal("stale loginDoneMsg must not reset the current attempt's loginCancel")
	}
	if currentCanceled {
		t.Fatal("stale loginDoneMsg must not invoke the current attempt's cancel func")
	}
	if dp.loginErr != nil {
		t.Fatalf("stale loginDoneMsg must not set loginErr, got %v", dp.loginErr)
	}

	// Current: attempt 1's loginDoneMsg (success) arrives and IS processed.
	np2, cmd2 := dp.Update(loginDoneMsg{err: nil, gen: 1})
	dp2 := np2.(*DeployPage)
	if dp2.state != deployStatePreflight {
		t.Fatalf("current-gen loginDoneMsg should transition to preflight, got %v", dp2.state)
	}
	if cmd2 == nil {
		t.Fatal("expected a re-probe command after the current-gen login succeeds")
	}
	if dp2.loginCancel != nil {
		t.Fatal("current-gen loginDoneMsg should clear loginCancel")
	}
}

// TestLogin_StaleStatusMsgIgnored verifies a stale loginStatusMsg (stamped
// with a prior generation) does not overwrite the current attempt's
// displayed status text or headless-fallback URL.
func TestLogin_StaleStatusMsgIgnored(t *testing.T) {
	p := &DeployPage{
		state:       deployStateLogin,
		pf:          &flow.Preflight{Provider: "omnia"},
		loginGen:    1,
		loginStatus: "Waiting for authorization…",
		loginURL:    "https://auth.example/current",
	}
	np, _ := p.Update(loginStatusMsg{text: "stale status", url: "https://auth.example/stale", gen: 0})
	dp := np.(*DeployPage)
	if dp.loginStatus != "Waiting for authorization…" {
		t.Fatalf("stale loginStatusMsg must not overwrite status, got %q", dp.loginStatus)
	}
	if dp.loginURL != "https://auth.example/current" {
		t.Fatalf("stale loginStatusMsg must not overwrite URL, got %q", dp.loginURL)
	}
}

// planWithChanges builds a *deploy.PlanResponse with one real change (a
// create) so tests can exercise the "plan has changes" path.
func planWithChanges() *deploy.PlanResponse {
	return &deploy.PlanResponse{
		Summary: "1 to add",
		Changes: []deploy.ResourceChange{
			{Type: "agent_runtime", Name: "bot", Action: deploy.ActionCreate},
		},
	}
}

// planAllNoChange builds a *deploy.PlanResponse whose only change is a
// NO_CHANGE, so tests can exercise the "empty plan" gating path.
func planAllNoChange() *deploy.PlanResponse {
	return &deploy.PlanResponse{
		Summary: "no changes",
		Changes: []deploy.ResourceChange{
			{Type: "secret", Name: "unused", Action: deploy.ActionNoChange},
		},
	}
}

// TestPlan_ReadyMsgWithChangesTransitionsAndRenders verifies that feeding a
// planReadyMsg carrying a 1-add plan transitions to deployStatePlan and
// renders the created row.
func TestPlan_ReadyMsgWithChangesTransitionsAndRenders(t *testing.T) {
	p := &DeployPage{state: deployStatePlanning, pf: &flow.Preflight{Provider: "omnia"}}
	plan := planWithChanges()
	np, _ := p.Update(planReadyMsg{plan: plan, req: &deploy.PlanRequest{}})
	dp := np.(*DeployPage)
	if dp.state != deployStatePlan {
		t.Fatalf("state = %v, want deployStatePlan", dp.state)
	}
	dp.SetSize(80, 24)
	out := stripANSI(dp.View())
	if !strings.Contains(out, "+ agent_runtime.bot") {
		t.Fatalf("expected created row in view:\n%s", out)
	}
}

// TestPlan_SpaceTogglesCollapse verifies the space key toggles
// collapseNoChange while in deployStatePlan.
func TestPlan_SpaceTogglesCollapse(t *testing.T) {
	p := &DeployPage{state: deployStatePlan, collapseNoChange: true, planDiff: viewmodels.BuildPlanDiff(planWithChanges())}
	np, _ := p.handlePlanKey(tea.KeyMsg{Type: tea.KeySpace})
	dp := np.(*DeployPage)
	if dp.collapseNoChange {
		t.Fatal("expected space to toggle collapseNoChange to false")
	}
	np2, _ := dp.handlePlanKey(tea.KeyMsg{Type: tea.KeySpace})
	dp2 := np2.(*DeployPage)
	if !dp2.collapseNoChange {
		t.Fatal("expected a second space to toggle collapseNoChange back to true")
	}
}

// TestPlan_EmptyPlanGatesApply verifies that an all-no-change (or empty) plan
// renders the "No changes" message and blocks 'a' from advancing to confirm.
func TestPlan_EmptyPlanGatesApply(t *testing.T) {
	p := &DeployPage{state: deployStatePlanning, pf: &flow.Preflight{Provider: "omnia"}}
	np, _ := p.Update(planReadyMsg{plan: planAllNoChange(), req: &deploy.PlanRequest{}})
	dp := np.(*DeployPage)
	if dp.state != deployStatePlan {
		t.Fatalf("state = %v, want deployStatePlan", dp.state)
	}

	dp.SetSize(80, 24)
	out := stripANSI(dp.View())
	if !strings.Contains(out, "No changes. Infrastructure is up to date.") {
		t.Fatalf("expected no-changes message in view:\n%s", out)
	}

	np2, _ := dp.handlePlanKey(keyRunes("a"))
	dp2 := np2.(*DeployPage)
	if dp2.state != deployStatePlan {
		t.Fatalf("'a' must not advance past an all-no-change plan, state = %v", dp2.state)
	}
}

// TestPlan_ApplyKeyAdvancesWhenChangesExist verifies 'a' advances to
// deployStateConfirm once the plan has real changes.
func TestPlan_ApplyKeyAdvancesWhenChangesExist(t *testing.T) {
	p := &DeployPage{state: deployStatePlan, planDiff: viewmodels.BuildPlanDiff(planWithChanges())}
	np, _ := p.handlePlanKey(keyRunes("a"))
	dp := np.(*DeployPage)
	if dp.state != deployStateConfirm {
		t.Fatalf("state = %v, want deployStateConfirm", dp.state)
	}
}

// TestDeployPage_Close_ClosesSession verifies Close releases an open
// flow.Session's adapter subprocess (Task 4.2 extends Task 4.1's Close to
// cover the session opened by startPlan, in addition to the existing
// login-goroutine cancellation).
func TestDeployPage_Close_ClosesSession(t *testing.T) {
	closed := false
	sess := flow.NewSession(flow.Options{}, nil, &arenaconfig.DeployConfig{Provider: "omnia"},
		nil, nil, nil, "", func() error { closed = true; return nil })

	loginCanceled := false
	planCanceled := false
	p := &DeployPage{
		state:       deployStatePlan,
		sess:        sess,
		loginCancel: func() { loginCanceled = true },
		planCancel:  func() { planCanceled = true },
	}

	p.Close()

	if !closed {
		t.Fatal("expected Close to close the open session")
	}
	if p.sess != nil {
		t.Fatal("expected Close to nil out sess")
	}
	if !loginCanceled {
		t.Fatal("expected Close to still cancel the login goroutine (Task 4.1 behavior)")
	}
	if !planCanceled {
		t.Fatal("expected Close to cancel the planning context")
	}
}

// TestAbandonIfCancelled_CancelledContextClosesSession verifies the
// after-close-during-Open/Plan abandon path: when startPlan's goroutine
// discovers ctx was already cancelled (the page was popped mid-Open/Plan),
// abandonIfCancelled must close sess so the adapter subprocess is reaped
// even though sessionOpenedMsg/planReadyMsg is never sent (nothing would be
// listening for it — App.Update would route it to whatever page is now on
// top of the stack).
func TestAbandonIfCancelled_CancelledContextClosesSession(t *testing.T) {
	closed := false
	sess := flow.NewSession(flow.Options{}, nil, &arenaconfig.DeployConfig{Provider: "omnia"},
		nil, nil, nil, "", func() error { closed = true; return nil })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !abandonIfCancelled(ctx, sess) {
		t.Fatal("expected abandonIfCancelled to report true for a cancelled context")
	}
	if !closed {
		t.Fatal("expected abandonIfCancelled to close the session on the abandon path")
	}
}

// TestAbandonIfCancelled_LiveContextLeavesSessionOpen verifies the normal
// (non-abandoned) path: a still-live context must not close sess, so
// startPlan's goroutine goes on to send sessionOpenedMsg/planReadyMsg as
// usual.
func TestAbandonIfCancelled_LiveContextLeavesSessionOpen(t *testing.T) {
	closed := false
	sess := flow.NewSession(flow.Options{}, nil, &arenaconfig.DeployConfig{Provider: "omnia"},
		nil, nil, nil, "", func() error { closed = true; return nil })

	ctx := context.Background()

	if abandonIfCancelled(ctx, sess) {
		t.Fatal("expected abandonIfCancelled to report false for a live context")
	}
	if closed {
		t.Fatal("expected abandonIfCancelled to leave the session open on the live path")
	}
}

// keyEnterMsg builds a tea.KeyMsg for the Enter key. Named to avoid colliding
// with the package-level keyEnter string constant in viewpage.go (used for
// KeyBinding descriptions).
func keyEnterMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}

// TestConfirm_ProductionRequiresTypedName verifies the non-default-env confirm
// mode: a mismatched typed name must not advance past deployStateConfirm, and
// the exact env name must advance to deployStateApplying.
func TestConfirm_ProductionRequiresTypedName(t *testing.T) {
	p := &DeployPage{state: deployStateConfirm, pf: &flow.Preflight{Env: "production"}}
	// Wrong text does not advance.
	p.confirmInput = "prod"
	np, _ := p.handleConfirmKey(keyEnterMsg())
	if np.(*DeployPage).state != deployStateConfirm {
		t.Fatal("must not advance on mismatched env name")
	}
	// Exact name advances to applying.
	p.confirmInput = "production"
	np, _ = p.handleConfirmKey(keyEnterMsg())
	if np.(*DeployPage).state != deployStateApplying {
		t.Fatal("should advance when env name typed exactly")
	}
}

// TestConfirm_DefaultEnvIsYN verifies the default-env confirm mode: a simple
// [y/N] prompt where 'y' advances to deployStateApplying.
func TestConfirm_DefaultEnvIsYN(t *testing.T) {
	p := &DeployPage{state: deployStateConfirm, pf: &flow.Preflight{Env: "default"}}
	np, _ := p.handleConfirmKey(keyRunes("y"))
	if np.(*DeployPage).state != deployStateApplying {
		t.Fatal("y should advance in default env")
	}
}

// TestConfirm_DefaultEnvOtherKeyReturnsToPlan verifies any key other than 'y'
// in the default-env confirm mode backs out to deployStatePlan.
func TestConfirm_DefaultEnvOtherKeyReturnsToPlan(t *testing.T) {
	p := &DeployPage{state: deployStateConfirm, pf: &flow.Preflight{Env: "default"}}
	np, _ := p.handleConfirmKey(keyRunes("n"))
	if np.(*DeployPage).state != deployStatePlan {
		t.Fatalf("state = %v, want deployStatePlan", np.(*DeployPage).state)
	}
}

// TestConfirm_TypedRunesAppendAndBackspaceEdits verifies rune keys append to
// confirmInput and backspace removes the last character.
func TestConfirm_TypedRunesAppendAndBackspaceEdits(t *testing.T) {
	p := &DeployPage{state: deployStateConfirm, pf: &flow.Preflight{Env: "production"}}
	np, _ := p.handleConfirmKey(keyRunes("p"))
	np, _ = np.(*DeployPage).handleConfirmKey(keyRunes("r"))
	dp := np.(*DeployPage)
	if dp.confirmInput != "pr" {
		t.Fatalf("confirmInput = %q, want %q", dp.confirmInput, "pr")
	}
	np2, _ := dp.handleConfirmKey(tea.KeyMsg{Type: tea.KeyBackspace})
	dp2 := np2.(*DeployPage)
	if dp2.confirmInput != "p" {
		t.Fatalf("confirmInput after backspace = %q, want %q", dp2.confirmInput, "p")
	}
}

// TestConfirm_MismatchShowsMessageThenClearsOnEdit verifies a mismatched
// Enter sets a "names don't match" flag rendered by viewConfirm, and further
// typing clears it.
func TestConfirm_MismatchShowsMessageThenClearsOnEdit(t *testing.T) {
	p := &DeployPage{state: deployStateConfirm, pf: &flow.Preflight{Provider: "omnia", Env: "production"}, confirmInput: "wrong"}
	np, _ := p.handleConfirmKey(keyEnterMsg())
	dp := np.(*DeployPage)
	if dp.state != deployStateConfirm {
		t.Fatal("mismatch must not advance")
	}
	out := stripANSI(dp.viewConfirm(80))
	if !strings.Contains(out, "don't match") {
		t.Fatalf("expected mismatch message in view:\n%s", out)
	}
	np2, _ := dp.handleConfirmKey(keyRunes("x"))
	dp2 := np2.(*DeployPage)
	if dp2.confirmMismatch {
		t.Fatal("expected typing to clear the mismatch flag")
	}
}

// TestGoldenDeployConfirm_Production captures a stable snapshot of the loud
// non-default-env confirm banner and typed-name prompt.
func TestGoldenDeployConfirm_Production(t *testing.T) {
	p := &DeployPage{
		state:        deployStateConfirm,
		pf:           &flow.Preflight{Provider: "omnia", Env: "production"},
		confirmInput: "prod",
	}
	p.SetSize(80, 24)
	out := stripANSI(p.View())
	teatest.RequireEqualOutput(t, []byte(out))
}

// TestGoldenDeployPlan_WithChanges captures a stable snapshot of the plan
// diff view with a mix of changes, collapsed no-change rows by default.
func TestGoldenDeployPlan_WithChanges(t *testing.T) {
	plan := &deploy.PlanResponse{
		Summary: "2 to add, 1 to change",
		Changes: []deploy.ResourceChange{
			{Type: "agent_runtime", Name: "bot", Action: deploy.ActionCreate},
			{Type: "a2a_endpoint", Name: "ep", Action: deploy.ActionCreate},
			{Type: "agent_runtime", Name: "old", Action: deploy.ActionUpdate, Detail: "image bumped"},
			{Type: "secret", Name: "unused", Action: deploy.ActionNoChange},
		},
	}
	p := &DeployPage{state: deployStatePlan, collapseNoChange: true, planDiff: viewmodels.BuildPlanDiff(plan)}
	p.SetSize(80, 24)
	out := stripANSI(p.View())
	teatest.RequireEqualOutput(t, []byte(out))
}
