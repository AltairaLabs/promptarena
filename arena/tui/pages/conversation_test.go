package pages

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/statestore"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// twoTurnResult is a small two-message run used by the navigation tests.
func twoTurnResult(runID string) *statestore.RunResult {
	return &statestore.RunResult{
		RunID: runID,
		Messages: []types.Message{
			{Role: "user", Content: "Message 1"},
			{Role: "assistant", Content: "Message 2"},
		},
		Duration: time.Second,
	}
}

func TestConversationPage_Reset(t *testing.T) {
	page := NewConversationPage()
	page.SetDimensions(100, 50)
	page.SetData("run-123", "test-scenario", "test-provider", twoTurnResult("run-123"))
	page.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 1, page.Panel().SelectedTurnIdx())

	// Reset returns the panel to its initial state: the selection is back on
	// the first turn and, until data is set again, keys are ignored.
	page.Reset()
	assert.Equal(t, 0, page.Panel().SelectedTurnIdx())
	assert.Nil(t, page.Update(tea.KeyMsg{Type: tea.KeyDown}))
	assert.Equal(t, 0, page.Panel().SelectedTurnIdx())

	// Setting the same run again rebuilds the table rather than reusing the
	// pre-Reset one.
	page.SetData("run-123", "test-scenario", "test-provider", twoTurnResult("run-123"))
	page.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, page.Panel().SelectedTurnIdx())
}

func TestConversationPage_SetDimensions(t *testing.T) {
	widest := func(s string) int {
		w := 0
		for _, line := range strings.Split(s, "\n") {
			w = max(w, lipgloss.Width(line))
		}
		return w
	}

	page := NewConversationPage()
	page.SetDimensions(80, 30)
	page.SetData("run-1", "scenario", "provider", twoTurnResult("run-1"))
	narrow := widest(page.Render())

	// SetDimensions reaches the panel: a wider page lays the turns list and
	// detail pane out wider.
	page.SetDimensions(200, 30)
	wide := widest(page.Render())
	assert.Greater(t, wide, narrow)
}

func TestConversationPage_SetData(t *testing.T) {
	page := NewConversationPage()
	page.SetDimensions(100, 50)

	res := &statestore.RunResult{
		RunID: "run-456",
		Messages: []types.Message{
			{Role: "user", Content: "Test message"},
		},
		Cost:     types.CostInfo{InputTokens: 5, OutputTokens: 10},
		Duration: time.Millisecond * 500,
	}

	page.SetData("run-456", "scenario-1", "provider-1", res)

	view := page.Render()
	assert.Contains(t, view, "Conversation")
}

func TestConversationPage_SetData_NilResult(t *testing.T) {
	page := NewConversationPage()
	page.SetDimensions(100, 50)

	page.SetData("run-789", "scenario", "provider", nil)

	view := page.Render()
	assert.Contains(t, view, "No conversation available")
}

func TestConversationPage_Update(t *testing.T) {
	page := NewConversationPage()
	page.SetDimensions(100, 50)
	page.SetData("run-update", "scenario", "provider", twoTurnResult("run-update"))

	// With the turns list focused, Down moves the selected turn.
	page.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, page.Panel().SelectedTurnIdx())

	// Right moves focus to the detail pane, so Up no longer moves the turn.
	page.Update(tea.KeyMsg{Type: tea.KeyRight})
	page.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 1, page.Panel().SelectedTurnIdx())

	// Left returns focus to the turns list.
	page.Update(tea.KeyMsg{Type: tea.KeyLeft})
	page.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, page.Panel().SelectedTurnIdx())
}

func TestConversationPage_Render_WithMessages(t *testing.T) {
	page := NewConversationPage()
	page.SetDimensions(100, 50)

	res := &statestore.RunResult{
		RunID: "render-test",
		Messages: []types.Message{
			{Role: "system", Content: "You are a helpful assistant"},
			{Role: "user", Content: "Hello!"},
			{Role: "assistant", Content: "Hi! How can I help you today?"},
		},
		Cost: types.CostInfo{
			InputTokens:  15,
			OutputTokens: 25,
			TotalCost:    0.002,
		},
		Duration: 2 * time.Second,
	}
	page.SetData("render-test", "my-scenario", "openai", res)

	view := page.Render()

	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Conversation")
	assert.Contains(t, view, "my-scenario")
	assert.Contains(t, view, "openai")
}

func TestConversationPage_GetKeyBindings(t *testing.T) {
	page := NewConversationPage()

	bindings := page.GetKeyBindings()

	assert.NotEmpty(t, bindings)
	// Should have navigation, pane switching, and back bindings
	assert.GreaterOrEqual(t, len(bindings), 3)

	// Verify expected key bindings exist
	foundNav := false
	foundSwitch := false
	foundBack := false
	for _, b := range bindings {
		if b.Description == "navigate" {
			foundNav = true
		}
		if b.Description == "switch pane" {
			foundSwitch = true
		}
		if b.Description == "back" {
			foundBack = true
		}
	}
	assert.True(t, foundNav, "should have navigate binding")
	assert.True(t, foundSwitch, "should have switch pane binding")
	assert.True(t, foundBack, "should have back binding")
}

func TestConversationPage_Panel(t *testing.T) {
	page := NewConversationPage()

	panel := page.Panel()

	assert.NotNil(t, panel)
	// Verify it's the same panel
	assert.Same(t, page.panel, panel)
}

func TestConversationPage_Render_EmptyMessages(t *testing.T) {
	page := NewConversationPage()
	page.SetDimensions(100, 50)

	res := &statestore.RunResult{
		RunID:    "empty-test",
		Messages: []types.Message{},
		Cost:     types.CostInfo{},
		Duration: 0,
	}
	page.SetData("empty-test", "scenario", "provider", res)

	view := page.Render()

	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Waiting for conversation to start")
}

func TestConversationPage_MultipleSetData(t *testing.T) {
	page := NewConversationPage()
	page.SetDimensions(100, 50)

	// Set initial data
	res1 := &statestore.RunResult{
		RunID: "run-1",
		Messages: []types.Message{
			{Role: "user", Content: "First conversation"},
		},
		Cost:     types.CostInfo{},
		Duration: time.Second,
	}
	page.SetData("run-1", "scenario-1", "provider-1", res1)

	view1 := page.Render()
	assert.Contains(t, view1, "scenario-1")

	// Set different data
	res2 := &statestore.RunResult{
		RunID: "run-2",
		Messages: []types.Message{
			{Role: "user", Content: "Second conversation"},
		},
		Cost:     types.CostInfo{},
		Duration: time.Second,
	}
	page.SetData("run-2", "scenario-2", "provider-2", res2)

	view2 := page.Render()
	assert.Contains(t, view2, "scenario-2")
}
