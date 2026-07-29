package app

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AltairaLabs/promptarena/arena/tui/panels"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

// consoleLink is the link an adapter typically attaches to a deployed agent.
func consoleLink() deploy.ResourceLink {
	return deploy.ResourceLink{
		Label: "Console",
		URL:   "https://ws.example/agents/support",
		Rel:   "console",
	}
}

// applyResultPageWithLink builds a DeployPage sitting on the apply-result
// screen with one adapter-supplied console link.
func applyResultPageWithLink() *DeployPage {
	return &DeployPage{
		state: deployStateApplyResult,
		applyResults: []*deploy.ResourceResult{
			{Type: "agent_runtime", Name: "support", Status: "created",
				Links: []deploy.ResourceLink{consoleLink()}},
		},
		applyLogs: panels.NewLogsPanel(),
	}
}

func pressKey(p *DeployPage, r rune) (Page, tea.Cmd) {
	return p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
}

// TestOpenLink_ApplyResultOpensFirstLink verifies [o] hands the adapter's
// first link to the browser opener verbatim.
func TestOpenLink_ApplyResultOpensFirstLink(t *testing.T) {
	var opened []string
	p := applyResultPageWithLink()
	p.openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}

	pressKey(p, 'o')

	if len(opened) != 1 {
		t.Fatalf("opener called %d times, want exactly 1: %v", len(opened), opened)
	}
	if opened[0] != consoleLink().URL {
		t.Errorf("opened %q, want the adapter's link %q", opened[0], consoleLink().URL)
	}
}

// TestOpenLink_StatusOpensFirstLink verifies the status screen opens links
// carried on the StatusResponse, the same as the apply-result screen.
func TestOpenLink_StatusOpensFirstLink(t *testing.T) {
	var opened []string
	p := &DeployPage{
		state: deployStateStatus,
		status: &deploy.StatusResponse{
			Status: "deployed",
			Links:  []deploy.ResourceLink{consoleLink()},
		},
		openURL: func(url string) error {
			opened = append(opened, url)
			return nil
		},
	}

	pressKey(p, 'o')

	if len(opened) != 1 || opened[0] != consoleLink().URL {
		t.Errorf("opened %v, want exactly [%s]", opened, consoleLink().URL)
	}
}

// TestOpenLink_NoLinksIsNoOp verifies [o] does nothing when the adapter
// supplied no links, rather than opening a synthesized URL. The client must
// never guess a console address — see deploy.ResourceLink.
func TestOpenLink_NoLinksIsNoOp(t *testing.T) {
	called := false
	p := &DeployPage{
		state:        deployStateApplyResult,
		applyResults: []*deploy.ResourceResult{{Type: "agent_runtime", Name: "support"}},
		openURL:      func(string) error { called = true; return nil },
	}

	pressKey(p, 'o')

	if called {
		t.Error("opener was called with no adapter-supplied link; the client must never synthesize a URL")
	}
}

// TestOpenLink_OpenerFailureKeepsURLVisible verifies a browser that cannot be
// launched — the normal case over SSH and in containers — leaves the page on
// the same screen with the URL still rendered, so the operator can copy it.
// It must not become an error state.
func TestOpenLink_OpenerFailureKeepsURLVisible(t *testing.T) {
	p := applyResultPageWithLink()
	p.w, p.h = 100, 30
	p.openURL = func(string) error { return errors.New("exec: \"xdg-open\": executable file not found in $PATH") }

	pressKey(p, 'o')

	if p.state != deployStateApplyResult {
		t.Fatalf("state = %v after a failed browser launch, want it to stay on deployStateApplyResult", p.state)
	}
	if !strings.Contains(p.View(), consoleLink().URL) {
		t.Error("URL is no longer visible after a failed browser launch; the operator can no longer copy it")
	}
}

// TestOpenLink_URLSurvivesNarrowTerminal verifies the URL renders in full at a
// terminal width that only just fits it. The chrome clips body lines to the
// terminal width, so anything sharing the URL's line — a "Console: " prefix,
// an indent — is width the URL loses, and a URL clipped mid-string still looks
// copy-pasteable while no longer working.
func TestOpenLink_URLSurvivesNarrowTerminal(t *testing.T) {
	url := "https://acme.omnia.example/workspaces/prod/agents/support-triage"
	p := &DeployPage{
		state: deployStateApplyResult,
		applyResults: []*deploy.ResourceResult{
			{Type: "agent_runtime", Name: "support", Status: "created",
				Links: []deploy.ResourceLink{{Label: "Console", URL: url, Rel: "console"}}},
		},
		applyLogs: panels.NewLogsPanel(),
	}
	p.w, p.h = len(url)+2, 40

	if !strings.Contains(stripANSI(p.View()), url) {
		t.Errorf("URL was clipped at a terminal %d wide; it must render on its own line "+
			"so the full %d-character URL survives", p.w, len(url))
	}
}

// TestOpenLink_NoLinksAddsNothingToTheHeadline verifies the links feature
// contributes exactly nothing when the adapter supplied none — no blank line,
// no separator. The whole feature is optional, so a deploy against an adapter
// that returns no links must look exactly as it did before links existed.
func TestOpenLink_NoLinksAddsNothingToTheHeadline(t *testing.T) {
	p := &DeployPage{
		state:        deployStateApplyResult,
		applyResults: []*deploy.ResourceResult{{Type: "agent_runtime", Name: "support"}},
	}
	headline := p.applyResultHeadline()

	if got := p.headlineWithLinks(headline); got != headline {
		t.Errorf("headlineWithLinks() = %q, want the headline unchanged (%q)", got, headline)
	}
}

// TestOpenLink_LinksOnlyOnPostApplyScreens verifies links stay off every
// screen before the deploy has actually happened. Apply results can already be
// populated while the wizard sits on an earlier state (a re-plan after an
// apply, say), and advertising a console for a deployment the operator has not
// yet confirmed would be misleading.
func TestOpenLink_LinksOnlyOnPostApplyScreens(t *testing.T) {
	applied := []*deploy.ResourceResult{
		{Type: "agent_runtime", Name: "support", Links: []deploy.ResourceLink{consoleLink()}},
	}
	for _, state := range []deployState{
		deployStatePreflight, deployStateLogin, deployStatePlanning,
		deployStatePlan, deployStateConfirm, deployStateApplying, deployStateError,
	} {
		p := &DeployPage{state: state, applyResults: applied}
		if got := p.deployLinks(); len(got) != 0 {
			t.Errorf("state %v exposed %d link(s); links belong only on the apply-result and status screens",
				state, len(got))
		}
	}
}

// TestOpenLink_DefaultOpenerIsWired guards against the page shipping with a
// nil opener, which would make [o] silently dead in the real app while every
// test that injects its own opener still passed.
func TestOpenLink_DefaultOpenerIsWired(t *testing.T) {
	p, ok := NewDeployPage(&AppContext{}).(*DeployPage)
	if !ok {
		t.Fatal("NewDeployPage did not return a *DeployPage")
	}
	if p.openURL == nil {
		t.Error("NewDeployPage left openURL nil; [o] would do nothing in the real app")
	}
}
