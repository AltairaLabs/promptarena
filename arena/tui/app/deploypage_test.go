package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
)

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
