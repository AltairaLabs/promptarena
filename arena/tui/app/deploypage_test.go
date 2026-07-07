package app

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
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
