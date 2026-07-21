package app

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AltairaLabs/promptarena/arena/tui/theme"
)

// splashDoneMsg is sent by the timer started in Init() to auto-dismiss the
// splash screen after ~1.5 seconds.
type splashDoneMsg struct{}

// paLogo is the PromptArena mark rendered as terminal block-art: the twin
// four-pointed sparkles from logo-promptarena.svg (a large star with a smaller
// companion). It is drawn in ion-cyan (--ion-cyan, the mark's fill color) by
// View. Keep the two stars aligned if you edit this.
const paLogo = `      ▲
     ███
    █████            ▲
◀███████████▶       ███
    █████         ◀█████▶
     ███            ███
      ▼              ▼`

// splashDuration is the time before the splash auto-dismisses.
const splashDuration = 1500 * time.Millisecond

// Splash is the transient splash-screen Page shown at launch. It displays the
// locked PromptKit logo, a wordmark with the version, and a tagline. It
// dismisses on any key press or after splashDuration.
type Splash struct {
	ctx  *AppContext
	w, h int
}

// NewSplash creates a Splash page backed by the given AppContext. The version
// string from ctx is rendered on the screen; pass ctx.Version = "vTEST" (or
// similar fixed string) in golden tests to keep output byte-stable.
func NewSplash(ctx *AppContext) *Splash {
	return &Splash{ctx: ctx}
}

// Init implements Page. It starts a ~1.5-second timer that fires splashDoneMsg.
func (s *Splash) Init() tea.Cmd {
	return tea.Tick(splashDuration, func(time.Time) tea.Msg {
		return splashDoneMsg{}
	})
}

// Update implements Page. Any key press or the expiry timer dismisses the
// splash by returning a cmd that emits PopPageMsg{}.
func (s *Splash) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg, splashDoneMsg:
		return s, func() tea.Msg { return PopPageMsg{} }
	}
	return s, nil
}

// View implements Page. It renders the logo, wordmark, version, and tagline
// centered within the allocated terminal size.
func (s *Splash) View() string {
	// The mark renders in ion-cyan — the sparkle fill from the PromptArena
	// logo (--ion-cyan / --node-tool).
	logoStyle := lipgloss.NewStyle().
		Foreground(theme.Colors().NodeTool)

	logo := logoStyle.Render(paLogo)

	wordmark := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Colors().TextHeading).
		Render("P r o m p t A r e n a   ·   " + s.ctx.Version)

	tagline := lipgloss.NewStyle().
		Foreground(theme.Colors().TextMuted).
		Render("Test, evaluate, and ship AI agents with confidence.")

	hint := lipgloss.NewStyle().
		Foreground(theme.Colors().TextMuted).
		Render("press any key ▸")

	content := strings.Join([]string{logo, "", "", "", wordmark, tagline, "", hint}, "\n")

	return lipgloss.Place(s.w, s.h, lipgloss.Center, lipgloss.Center, content)
}

// Title implements Page. The splash has no title bar.
func (s *Splash) Title() string { return "" }

// SetSize implements Page.
func (s *Splash) SetSize(w, h int) {
	s.w, s.h = w, h
}
