package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"

	"github.com/AltairaLabs/promptarena/arena/tui"
)

// recordingPage counts the lifecycle events and key presses it receives.
type recordingPage struct {
	name         string
	runCompleted int
	keys         int
}

func (p *recordingPage) Init() tea.Cmd { return nil }
func (p *recordingPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg.(type) {
	case tui.RunCompletedMsg:
		p.runCompleted++
	case tea.KeyMsg:
		p.keys++
	}
	return p, nil
}
func (p *recordingPage) View() string     { return p.name }
func (p *recordingPage) Title() string    { return p.name }
func (p *recordingPage) SetSize(_, _ int) {}

// TestApp_BackgroundEventsReachAllPages guards the fix for runs staying
// "Running" after drilling into a conversation: run-lifecycle events must reach
// the backgrounded RunPage, not just the top ConversationViewPage. User input
// still targets the top page only.
func TestApp_BackgroundEventsReachAllPages(t *testing.T) {
	root := &recordingPage{name: "root"} // stands in for the RunPage
	a := New(&AppContext{}, root)
	top := &recordingPage{name: "top"} // stands in for a pushed conversation page
	a.stack = append(a.stack, top)

	a.Update(tui.RunCompletedMsg{RunID: "r"})
	assert.Equal(t, 1, root.runCompleted, "backgrounded page must receive RunCompletedMsg")
	assert.Equal(t, 1, top.runCompleted, "top page must receive RunCompletedMsg")

	// User input is not a background event — top page only.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	assert.Equal(t, 0, root.keys, "backgrounded page must NOT receive key input")
	assert.Equal(t, 1, top.keys, "top page receives key input")
}
