package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
	"github.com/AltairaLabs/promptarena/arena/tui/theme"
	"github.com/AltairaLabs/promptarena/arena/tui/viewmodels"
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

// sessionOpenedMsg carries a *flow.Session once the background startPlan
// goroutine's flow.Open call succeeds. It is delivered before planReadyMsg so
// Update can store the session (for Close and, later, Apply) even if the
// subsequent Session.Plan call fails and the wizard falls into
// deployStateError.
type sessionOpenedMsg struct{ sess *flow.Session }

// planReadyMsg carries the result of the background flow.Session.Plan call
// started from handlePreflightKey via startPlan.
type planReadyMsg struct {
	plan *deploy.PlanResponse
	req  *deploy.PlanRequest
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

	// Planning/plan-state fields, populated by the flow.Open + Session.Plan
	// goroutine started from handlePreflightKey via startPlan and delivered
	// through p.send. sess is held past the plan step so Task 5's Apply can
	// reuse the already-connected adapter subprocess; Close releases it.
	planCancel       context.CancelFunc
	sess             *flow.Session
	plan             *deploy.PlanResponse
	planReq          *deploy.PlanRequest
	planDiff         viewmodels.PlanDiffData
	collapseNoChange bool

	// Confirm-state fields. confirmInput accumulates the operator's typed
	// characters when pf.Env is non-default (the type-to-confirm guardrail);
	// it is unused (and unrendered) for the default-env [y/N] prompt.
	// confirmMismatch is set when Enter is pressed with a confirmInput that
	// doesn't match pf.Env exactly, so viewConfirm can show a "names don't
	// match" message; it is cleared as soon as the operator edits the input
	// again.
	confirmInput    string
	confirmMismatch bool
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
// against a page nothing is listening to anymore. It also cancels any
// in-flight planning goroutine and closes the flow.Session opened by
// startPlan — the session holds an adapter subprocess that MUST be released
// when the page is popped, however the wizard got here (plan failure, esc
// during planning/plan/confirm, or a later successful apply).
func (p *DeployPage) Close() {
	if p.loginCancel != nil {
		p.loginCancel()
		p.loginCancel = nil
	}
	if p.planCancel != nil {
		p.planCancel()
		p.planCancel = nil
	}
	if p.sess != nil {
		_ = p.sess.Close()
		p.sess = nil
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
	case sessionOpenedMsg:
		p.sess = m.sess
		return p, nil
	case planReadyMsg:
		p.plan = m.plan
		p.planReq = m.req
		p.planDiff = viewmodels.BuildPlanDiff(m.plan)
		p.collapseNoChange = true
		p.state = deployStatePlan
		return p, nil
	case spinner.TickMsg:
		if p.state != deployStateLogin && p.state != deployStatePlanning {
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
	case deployStatePlan:
		return p.handlePlanKey(msg)
	case deployStateConfirm:
		return p.handleConfirmKey(msg)
	// deployStatePlanning has no keys of its own (the background plan cmd
	// runs to completion or error). deployStateApplying, deployStateApplyResult,
	// deployStateStatus, and deployStateError key handling arrive in later
	// tasks (5.x).
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
			return p, tea.Batch(p.startPlan(), p.spinner.Tick)
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

// handlePlanKey handles key input while the plan diff is showing. '[space]'
// toggles whether unchanged resources are collapsed; 'a' advances to
// deployStateConfirm, but only when the plan has real (non-no-change)
// changes — an empty plan leaves nothing for the operator to apply.
func (p *DeployPage) handlePlanKey(msg tea.KeyMsg) (Page, tea.Cmd) {
	switch msg.String() {
	case " ":
		p.collapseNoChange = !p.collapseNoChange
	case "a":
		if p.planHasChanges() {
			p.state = deployStateConfirm
		}
	}
	return p, nil
}

// planHasChanges reports whether the current plan has any resource change
// other than NO_CHANGE.
func (p *DeployPage) planHasChanges() bool {
	return p.planDiff.Adds+p.planDiff.Changes+p.planDiff.Destroys+p.planDiff.Drifts > 0
}

// handleConfirmKey handles key input on the confirm screen. It implements two
// distinct guardrails depending on the target environment:
//
//   - Default env (flow.DefaultEnv): a quiet [y/N] prompt. 'y' advances to
//     deployStateApplying; every other key backs out to deployStatePlan —
//     there is nothing typed to edit, so any non-'y' key is a "no".
//   - Non-default env: the operator must type the exact environment name.
//     Printable runes append to confirmInput and backspace edits it (both
//     clear any prior mismatch flag); Enter compares confirmInput against
//     pf.Env — a match advances to deployStateApplying, a mismatch stays in
//     deployStateConfirm and sets confirmMismatch so viewConfirm can show
//     "names don't match".
//
// There is no esc case here: App's global esc handler (see Close's doc
// comment) pops the whole page before this handler ever sees the key,
// exactly as it does for deployStateLogin/deployStatePlan. Adding one would
// be dead code.
func (p *DeployPage) handleConfirmKey(msg tea.KeyMsg) (Page, tea.Cmd) {
	if p.pf == nil {
		return p, nil
	}
	if p.pf.Env == flow.DefaultEnv {
		if msg.Type == tea.KeyRunes && string(msg.Runes) == "y" {
			p.state = deployStateApplying
		} else {
			p.state = deployStatePlan
		}
		return p, nil
	}

	switch msg.Type {
	case tea.KeyEnter:
		if p.confirmInput == p.pf.Env {
			p.confirmMismatch = false
			p.state = deployStateApplying
		} else {
			p.confirmMismatch = true
		}
	case tea.KeyBackspace:
		if len(p.confirmInput) > 0 {
			p.confirmInput = p.confirmInput[:len(p.confirmInput)-1]
		}
		p.confirmMismatch = false
	case tea.KeyRunes:
		p.confirmInput += string(msg.Runes)
		p.confirmMismatch = false
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

// startPlan returns a tea.Cmd that starts a background goroutine which opens
// a flow.Session (connecting the adapter subprocess) and computes a plan,
// exactly as startLogin starts flow.Login: the returned tea.Cmd only launches
// the goroutine and returns nil, while the goroutine streams its results back
// through p.send. sessionOpenedMsg is sent as soon as Open succeeds — even if
// the subsequent Plan call fails — so Update can store the session (and
// Close can release its adapter subprocess) no matter how the wizard leaves
// deployStatePlanning. Locals are captured before the goroutine starts and
// the goroutine never touches p.* directly, mirroring startLogin's race
// discipline.
func (p *DeployPage) startPlan() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	p.planCancel = cancel
	opts := p.opts
	send := p.send

	return func() tea.Msg {
		go func() {
			sess, err := flow.Open(ctx, opts)
			if err != nil {
				send(deployErrMsg{err: err})
				return
			}
			// The page may have been popped (Close cancels ctx) while Open was
			// still connecting the adapter subprocess. If so, sending
			// sessionOpenedMsg would be dropped by App.Update (it routes to
			// whatever page is now on top of the stack), leaking sess and its
			// subprocess with no reaper. Close it here instead, on the
			// goroutine that actually holds it.
			if abandonIfCancelled(ctx, sess) {
				return
			}
			send(sessionOpenedMsg{sess: sess})

			plan, req, err := sess.Plan(ctx)
			if err != nil {
				send(deployErrMsg{err: err})
				return
			}
			// Same race, one step later: the page may have been popped while
			// Plan was still running.
			if abandonIfCancelled(ctx, sess) {
				return
			}
			send(planReadyMsg{plan: plan, req: req})
		}()
		return nil
	}
}

// abandonIfCancelled reports whether ctx was already cancelled by the time a
// blocking flow.Session step (Open, Plan) returned — meaning the page that
// started startPlan's goroutine has since been Close()d. When it has, it
// closes sess (reaping the adapter subprocess) so the caller can return
// without sending a message that would only be dropped by App.Update once
// nothing is listening for it. Extracted as a pure(ish) helper so the
// abandon-path logic is testable without racing a real goroutine against a
// real Close call.
func abandonIfCancelled(ctx context.Context, sess *flow.Session) bool {
	if ctx.Err() == nil {
		return false
	}
	_ = sess.Close()
	return true
}

// View implements Page.
func (p *DeployPage) View() string {
	body := func(_ int) string {
		switch p.state {
		case deployStatePreflight:
			return p.viewPreflight(p.w)
		case deployStateLogin:
			return p.viewLogin(p.w)
		case deployStatePlanning:
			return p.viewPlanning(p.w)
		case deployStatePlan:
			return p.viewPlan(p.w)
		case deployStateConfirm:
			return p.viewConfirm(p.w)
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
	case p.state == deployStatePlan:
		kb = append([]views.KeyBinding{{Keys: "space", Description: "toggle unchanged"}}, kb...)
		if p.planHasChanges() {
			kb = append([]views.KeyBinding{{Keys: "a", Description: "apply"}}, kb...)
		}
	case p.state == deployStateConfirm && p.pf != nil && p.pf.Env == flow.DefaultEnv:
		kb = append([]views.KeyBinding{{Keys: "y", Description: "confirm"}}, kb...)
	case p.state == deployStateConfirm:
		kb = append([]views.KeyBinding{{Keys: keyEnter, Description: "confirm"}}, kb...)
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

// viewPlanning renders the planning step: a spinner and status text while the
// background startPlan goroutine opens the adapter session and computes the
// plan. Mirrors viewLogin's spinner pattern.
func (p *DeployPage) viewPlanning(width int) string {
	line := p.spinner.View() + " " + theme.ValueStyle.Render("Computing plan…")
	return theme.BorderedBoxStyle.MaxWidth(width).Render(line)
}

// viewPlan renders the plan diff once startPlan's goroutine has delivered a
// planReadyMsg. A plan with no real changes (empty, or every change is
// deploy.ActionNoChange) renders a plain "up to date" message instead of the
// diff — handlePlanKey gates 'a' on the same condition (planHasChanges) so
// there is nothing to advance to confirm.
func (p *DeployPage) viewPlan(width int) string {
	if !p.planHasChanges() {
		body := theme.SuccessStyle.Render("No changes. Infrastructure is up to date.")
		return theme.BorderedBoxStyle.MaxWidth(width).Render(body)
	}
	body := views.RenderPlanDiff(p.planDiff, width, p.collapseNoChange)
	return theme.BorderedBoxStyle.MaxWidth(width).Render(body)
}

// viewConfirm renders the confirm step. Its shape depends entirely on
// pf.Env, mirroring handleConfirmKey's two modes: a quiet [y/N] prompt for
// flow.DefaultEnv, or a loud banner plus type-to-confirm prompt for every
// other environment.
func (p *DeployPage) viewConfirm(width int) string {
	if p.pf == nil {
		return theme.BorderedBoxStyle.MaxWidth(width).Render("")
	}
	if p.pf.Env == flow.DefaultEnv {
		return p.viewConfirmDefault(width)
	}
	return p.viewConfirmTyped(width)
}

// viewConfirmDefault renders the simple [y/N] confirm prompt used for
// flow.DefaultEnv deploys.
func (p *DeployPage) viewConfirmDefault(width int) string {
	prompt := fmt.Sprintf("Apply this plan to %s · %s? [y/N]", p.pf.Provider, p.pf.Env)
	body := theme.ValueStyle.Render(prompt)
	return theme.BorderedBoxStyle.MaxWidth(width).Render(body)
}

// viewConfirmTyped renders the type-to-confirm guardrail used for every
// non-default environment: a loud banner (ColorError for production,
// ColorWarning otherwise), the typed-so-far confirmInput, and a
// "names don't match" message once a mismatched Enter has been pressed.
func (p *DeployPage) viewConfirmTyped(width int) string {
	color := theme.ColorWarning
	if p.pf.Env == "production" {
		color = theme.ColorError
	}
	banner := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Background(lipgloss.Color(color)).
		Padding(0, 1).
		Render("⚠ Deploying to " + strings.ToUpper(p.pf.Env))

	lines := []string{
		banner,
		"",
		theme.LabelStyle.Render("Type the environment name to confirm:"),
		theme.ValueStyle.Render(p.confirmInput) + "▏",
	}
	if p.confirmMismatch {
		lines = append(lines, "", theme.ErrorStyle.Render("names don't match"))
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
