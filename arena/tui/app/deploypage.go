package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
	"github.com/AltairaLabs/promptarena/arena/tui/theme"
	"github.com/AltairaLabs/promptarena/arena/tui/views"
)

// deployState enumerates the steps of the guided Deploy wizard. Phase 1
// (this task) only drives deployStatePreflight and deployStateError; later
// tasks flesh out the remaining states.
type deployState int

const (
	deployStatePreflight deployState = iota
	deployStateLogin
	deployStatePlanning
	deployStatePlan
	deployStateConfirm
	deployStateApplying
	deployStateApplyResult
	deployStateStatus
	deployStateError
)

// preflightDoneMsg carries the result of the background flow.CheckPreflight
// probe kicked off from Activate.
type preflightDoneMsg struct{ pf *flow.Preflight }

// deployErrMsg carries an error encountered while driving the wizard.
type deployErrMsg struct{ err error }

// loginStatusMsg carries a progress update from the background flow.Login
// goroutine. text is the human-readable status line ("Waiting for
// authorization…"); url is set only on the update that carries the OAuth
// authorize URL (flow.LoginHooks.OnAuthorizeURL), so it can persist as the
// headless-fallback link even after later status updates change text. gen is
// the loginGen the originating startLogin call captured, so a stale attempt's
// message (delivered after a cancel-then-retry) can be told apart from the
// current one.
type loginStatusMsg struct {
	text string
	url  string
	gen  int
}

// loginDoneMsg carries the result of the background flow.Login goroutine. gen
// is the loginGen the originating startLogin call captured — see
// loginStatusMsg.
type loginDoneMsg struct {
	err error
	gen int
}

// DeployPage is the guided deploy wizard: preflight check, adapter login,
// plan, confirm, apply, and status, driven by the arena/deploy/flow package.
type DeployPage struct {
	ctx  *AppContext
	send func(tea.Msg)
	w, h int

	state deployState

	opts flow.Options
	pf   *flow.Preflight
	err  error

	// Login-state fields, populated by the flow.Login goroutine started from
	// handlePreflightKey via startLogin and delivered through p.send.
	spinner     spinner.Model
	loginStatus string
	loginURL    string
	loginErr    error
	loginCancel context.CancelFunc
	// loginGen is bumped at the start of every startLogin call. It is stamped
	// on loginStatusMsg/loginDoneMsg so Update can ignore messages from a prior
	// (canceled-then-retried) login attempt instead of letting a stale goroutine
	// clobber the current attempt's state.
	loginGen int
}

// NewDeployPage builds the deploy wizard page for ctx. It is the entry point
// used by the Home menu's Deploy item.
func NewDeployPage(ctx *AppContext) Page {
	return &DeployPage{
		ctx:   ctx,
		state: deployStatePreflight,
		opts: flow.Options{
			ConfigPath: ctx.ConfigPath,
			ProjectDir: ctx.ProjectDir(),
		},
		spinner: newLoginSpinner(),
	}
}

// newLoginSpinner builds the spinner model used by the login screen.
func newLoginSpinner() spinner.Model {
	return spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorPrimary))),
	)
}

// Title implements Page.
func (p *DeployPage) Title() string { return "Deploy" }

// Close implements Closeable. It cancels any in-flight flow.Login goroutine
// so that popping the page (e.g. pressing esc during deployStateLogin, which
// App's global esc handler pops before handleLoginKey ever sees the key) does
// not leave the login goroutine and its loopback OAuth callback server running
// against a page nothing is listening to anymore.
func (p *DeployPage) Close() {
	if p.loginCancel != nil {
		p.loginCancel()
		p.loginCancel = nil
	}
}

// SetSize implements Page.
func (p *DeployPage) SetSize(w, h int) { p.w, p.h = w, h }

// Init implements Page. No command runs here — Activate kicks off preflight
// once the page is pushed onto the navigation stack.
func (p *DeployPage) Init() tea.Cmd { return nil }

// Activate implements Activatable. It stores the send handle and kicks off
// the background preflight probe.
func (p *DeployPage) Activate(send func(tea.Msg)) tea.Cmd {
	p.send = send
	return p.runPreflight()
}

// runPreflight returns a tea.Cmd that runs flow.CheckPreflight in the
// background and reports the result as a preflightDoneMsg.
func (p *DeployPage) runPreflight() tea.Cmd {
	opts := p.opts
	return func() tea.Msg {
		pf := flow.CheckPreflight(contextTODO(), opts)
		return preflightDoneMsg{pf: pf}
	}
}

// Update implements Page.
func (p *DeployPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch m := msg.(type) {
	case preflightDoneMsg:
		p.pf = m.pf
		if m.pf.ConfigErr != nil || m.pf.ProbeErr != nil {
			p.err, p.state = firstErr(m.pf), deployStateError
		}
		return p, nil
	case deployErrMsg:
		p.err, p.state = m.err, deployStateError
		return p, nil
	case loginStatusMsg:
		if m.gen != p.loginGen {
			// Stale progress update from a canceled-then-retried attempt; drop it.
			return p, nil
		}
		p.loginStatus = m.text
		if m.url != "" {
			p.loginURL = m.url
		}
		return p, nil
	case loginDoneMsg:
		if m.gen != p.loginGen {
			// Stale completion from a canceled-then-retried attempt: a later
			// login is already in flight (or the operator backed out entirely).
			// Processing this would wipe the current attempt's loginCancel and
			// bounce the operator to preflight with a confusing stale error.
			return p, nil
		}
		p.loginCancel = nil
		if m.err != nil {
			// Recoverable: surface the error on the preflight screen rather
			// than dead-ending the wizard in deployStateError, so the
			// operator can retry login ('l') or the probe ('r') directly.
			p.loginErr = m.err
			p.state = deployStatePreflight
			return p, nil
		}
		p.loginErr = nil
		p.state = deployStatePreflight
		return p, p.runPreflight()
	case spinner.TickMsg:
		if p.state != deployStateLogin {
			return p, nil
		}
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(m)
		return p, cmd
	case tea.KeyMsg:
		return p.handleKey(m)
	}
	return p, nil
}

// handleKey dispatches a key message to the handler for the current state.
func (p *DeployPage) handleKey(msg tea.KeyMsg) (Page, tea.Cmd) {
	switch p.state {
	case deployStatePreflight:
		return p.handlePreflightKey(msg)
	case deployStateLogin:
		return p.handleLoginKey(msg)
	// deployStatePlanning, deployStatePlan, deployStateConfirm,
	// deployStateApplying, deployStateApplyResult, deployStateStatus, and
	// deployStateError key handling arrive in later tasks (4.x, 5.x).
	default:
		return p, nil
	}
}

// handlePreflightKey handles key input while the preflight probe is running
// or has just completed. Every transition is gated on the relevant pf field
// so a stale or partial preflight snapshot can't be bypassed:
//
//   - 'l' (login) only fires when the adapter supports login and isn't
//     already authenticated.
//   - 'p' (plan) only fires once pf.Ready() — adapter installed, connected
//     without error, and authenticated.
//   - 'r' (retry) always re-runs the preflight probe.
func (p *DeployPage) handlePreflightKey(msg tea.KeyMsg) (Page, tea.Cmd) {
	if p.pf == nil || msg.Type != tea.KeyRunes {
		return p, nil
	}
	switch string(msg.Runes) {
	case "l":
		if p.pf.SupportsLogin && !p.pf.Authenticated {
			p.state = deployStateLogin
			return p, tea.Batch(p.startLogin(), p.spinner.Tick)
		}
	case "p":
		if p.pf.Ready() {
			p.state = deployStatePlanning
			// Phase 4 wires the actual plan command; entering the state is
			// enough for now.
		}
	case "r":
		p.loginErr = nil
		return p, p.runPreflight()
	}
	return p, nil
}

// handleLoginKey handles key input while the login screen is showing.
// 'c' cancels the in-flight login and returns to preflight.
func (p *DeployPage) handleLoginKey(msg tea.KeyMsg) (Page, tea.Cmd) {
	if msg.Type != tea.KeyRunes {
		return p, nil
	}
	if string(msg.Runes) == "c" {
		if p.loginCancel != nil {
			p.loginCancel()
			p.loginCancel = nil
		}
		p.state = deployStatePreflight
	}
	return p, nil
}

// startLogin returns a tea.Cmd that starts flow.Login in a background
// goroutine, exactly as RunPage.Activate starts ExecuteRuns: the returned
// tea.Cmd only launches the goroutine and returns nil, while the goroutine
// itself streams progress and its final result through p.send — the same
// send handle stored by Activate (Task 3.1) and used by the run engine's
// EventAdapter.
func (p *DeployPage) startLogin() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	p.loginCancel = cancel
	p.loginStatus = "Starting login…"
	p.loginURL = ""
	p.loginErr = nil

	// Bump the generation and capture it into a local for the goroutine below.
	// This is the only thing that ties a background attempt's messages back to
	// "is this still the current attempt" — the goroutine must never read
	// p.loginGen directly (that field is only ever touched from the bubbletea
	// update goroutine).
	p.loginGen++
	gen := p.loginGen

	provider := p.pf.Provider
	opts := p.opts
	send := p.send

	return func() tea.Msg {
		go func() {
			hooks := flow.LoginHooks{
				OnStatus:       func(s string) { send(loginStatusMsg{text: s, gen: gen}) },
				OnAuthorizeURL: func(u string) { send(loginStatusMsg{text: "Waiting for authorization…", url: u, gen: gen}) },
			}
			err := flow.Login(ctx, provider, opts, hooks)
			send(loginDoneMsg{err: err, gen: gen})
		}()
		return nil
	}
}

// View implements Page.
func (p *DeployPage) View() string {
	body := func(_ int) string {
		switch p.state {
		case deployStatePreflight:
			return p.viewPreflight(p.w)
		case deployStateLogin:
			return p.viewLogin(p.w)
		case deployStateError:
			return p.viewError()
		default:
			return ""
		}
	}
	return views.RenderWithChrome(p.chrome(), body)
}

// chrome builds the ChromeConfig for the current state.
func (p *DeployPage) chrome() views.ChromeConfig {
	return views.ChromeConfig{
		Width:       p.w,
		Height:      p.h,
		Title:       "Deploy",
		KeyBindings: p.keyBindings(),
	}
}

// keyBindings returns the footer key hints for the current state.
func (p *DeployPage) keyBindings() []views.KeyBinding {
	kb := []views.KeyBinding{{Keys: "esc", Description: "back"}}
	switch {
	case p.state == deployStateLogin:
		kb = append([]views.KeyBinding{{Keys: "c", Description: "cancel"}}, kb...)
	case p.state == deployStatePreflight && p.pf != nil:
		// Prepend in priority order so the most relevant action reads first.
		if p.pf.Ready() {
			kb = append([]views.KeyBinding{{Keys: "p", Description: "plan"}}, kb...)
		}
		if p.pf.SupportsLogin && !p.pf.Authenticated {
			kb = append([]views.KeyBinding{{Keys: "l", Description: "login"}}, kb...)
		}
		kb = append([]views.KeyBinding{{Keys: "r", Description: "retry"}}, kb...)
	}
	return kb
}

// viewPreflight renders the preflight-check step: target provider, a loudly
// flagged environment banner for non-default envs, adapter presence, and
// auth state. When the adapter isn't installed it renders the install
// command instead of offering to plan.
func (p *DeployPage) viewPreflight(width int) string {
	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorPrimary)).
		Render("Checking deploy adapter…")
	if p.pf == nil {
		return theme.BorderedBoxStyle.MaxWidth(width).Render(labelStyle)
	}
	pf := p.pf

	lines := []string{
		theme.LabelStyle.Render("Provider: ") + theme.ValueStyle.Render(pf.Provider),
		preflightEnvLine(pf.Env),
		"",
	}

	if pf.AdapterFound {
		lines = append(lines, theme.SuccessStyle.Render(fmt.Sprintf("✓ %s v%s", pf.Provider, pf.AdapterVersion)))
	} else {
		lines = append(lines, theme.ErrorStyle.Render("✗ not installed"))
	}

	if pf.Authenticated {
		lines = append(lines, theme.SuccessStyle.Render("✓ authenticated"))
	} else {
		lines = append(lines, theme.ErrorStyle.Render("✗ not authenticated"))
	}

	if !pf.AdapterFound {
		lines = append(lines, "", "Install with: "+pf.InstallCommand)
	}

	if p.loginErr != nil {
		lines = append(lines, "", theme.ErrorStyle.Render("Login failed: "+p.loginErr.Error()))
	}

	body := strings.Join(lines, "\n")
	return theme.BorderedBoxStyle.MaxWidth(width).Render(body)
}

// viewLogin renders the login screen: a spinner, the latest status text, the
// authorize URL as a headless fallback (once known), and the cancel hint.
func (p *DeployPage) viewLogin(width int) string {
	status := p.loginStatus
	if status == "" {
		status = "Starting login…"
	}
	lines := []string{
		p.spinner.View() + " " + theme.ValueStyle.Render(status),
	}
	if p.loginURL != "" {
		lines = append(lines,
			"",
			theme.LabelStyle.Render("If your browser didn't open, visit:"),
			theme.ValueStyle.Render(p.loginURL),
		)
	}
	body := strings.Join(lines, "\n")
	return theme.BorderedBoxStyle.MaxWidth(width).Render(body)
}

// preflightEnvLine renders the target environment. flow.DefaultEnv renders
// quietly; every other environment is rendered loudly so an operator can't
// miss it — production gets an inverse ColorError banner, anything else gets
// bold ColorWarning text.
func preflightEnvLine(env string) string {
	quiet := theme.LabelStyle.Render("Environment: ") + theme.ValueStyle.Render(env)
	if env == flow.DefaultEnv {
		return quiet
	}
	label := "ENVIRONMENT: " + strings.ToUpper(env)
	if env == "production" {
		return lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(theme.ColorWhite)).
			Background(lipgloss.Color(theme.ColorError)).
			Padding(0, 1).
			Render(label)
	}
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorWarning)).
		Render("⚠ " + label)
}

// viewError renders the error step. Fleshed out in Task 3.2.
func (p *DeployPage) viewError() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorError)).
		Render("Deploy failed")
	msg := ""
	if p.err != nil {
		msg = p.err.Error()
	}
	detail := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGray)).
		Render(msg)
	return title + "\n\n" + detail
}

// contextTODO returns the context used for background deploy operations.
// AppContext does not currently carry a cancelable context, so this returns
// context.Background(); replace with a context sourced from AppContext if one
// is added later.
func contextTODO() context.Context { return context.Background() }

// firstErr returns pf.ConfigErr if set, otherwise pf.ProbeErr.
func firstErr(pf *flow.Preflight) error {
	if pf.ConfigErr != nil {
		return pf.ConfigErr
	}
	return pf.ProbeErr
}
