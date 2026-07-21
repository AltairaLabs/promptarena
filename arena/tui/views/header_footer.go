package views

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/AltairaLabs/promptarena/arena/tui/theme"
)

// KeyBinding represents a keyboard shortcut with description
type KeyBinding struct {
	Keys        string // e.g., "↑/↓ j/k", "enter", "q/ctrl+c"
	Description string // e.g., "navigate", "select file", "quit"
}

const (
	headerProgressBarWidth = 12
)

// HeaderFooterView renders header and footer components
type HeaderFooterView struct {
	width int
}

// NewHeaderFooterView creates a new header/footer view
func NewHeaderFooterView(width int) *HeaderFooterView {
	return &HeaderFooterView{width: width}
}

// RenderHeader renders the top banner with progress
func (v *HeaderFooterView) RenderHeader(
	configFile string,
	completedCount, totalRuns int,
) string {
	// Banner. Not gold: Atlas reserves gold for the one primary thing per
	// view, so persistent chrome uses bright heading starlight instead.
	bannerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Colors().TextHeading).
		Align(lipgloss.Center).
		Width(v.width)

	infoStyle := lipgloss.NewStyle().
		Foreground(theme.Colors().TextMuted).
		Align(lipgloss.Center).
		Width(v.width)

	progressStyle := lipgloss.NewStyle().
		Foreground(theme.Colors().StatusHealthy).
		Bold(true)

	tagStyle := lipgloss.NewStyle().
		Foreground(theme.Colors().StatusPending).
		Bold(true)

	mockTag := ""
	if strings.Contains(strings.ToLower(filepath.Base(configFile)), "mock") {
		mockTag = tagStyle.Render("MOCK MODE")
	}

	banner := bannerStyle.Render("✨ PromptArena ✨")
	progressBar := buildProgressBar(completedCount, totalRuns, headerProgressBarWidth)
	progress := progressStyle.Render(fmt.Sprintf("[%s %d/%d]", progressBar, completedCount, totalRuns))

	parts := []string{filepath.Base(configFile), progress}
	if mockTag != "" {
		parts = append([]string{mockTag}, parts...)
	}

	infoLine := infoStyle.Render(strings.Join(parts, "  •  "))

	return lipgloss.JoinVertical(lipgloss.Left, banner, infoLine)
}

// RenderTitleHeader renders the banner plus a centered page-title line. Used by
// non-run pages (Home, View, Chat, Inspect, …) where the run progress bar /
// timer would be meaningless.
func (v *HeaderFooterView) RenderTitleHeader(title string) string {
	bannerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Colors().TextHeading).
		Align(lipgloss.Center).
		Width(v.width)
	titleStyle := lipgloss.NewStyle().
		Foreground(theme.Colors().TextMuted).
		Align(lipgloss.Center).
		Width(v.width)

	banner := bannerStyle.Render("✨ PromptArena ✨")
	if title == "" {
		return banner
	}
	return lipgloss.JoinVertical(lipgloss.Left, banner, titleStyle.Render(title))
}

// RenderFooter renders the bottom help text with dynamic key bindings
func (v *HeaderFooterView) RenderFooter(keyBindings []KeyBinding) string {
	legendStyle := lipgloss.NewStyle().
		Foreground(theme.Colors().TextFaint).
		Background(theme.Colors().Surface2).
		Padding(0, 1)

	keyStyle := lipgloss.NewStyle().
		Foreground(theme.Colors().AccentInter).
		Bold(true)

	var helpItems []string
	for _, kb := range keyBindings {
		helpItems = append(helpItems, keyStyle.Render(kb.Keys)+" "+kb.Description)
	}

	return legendStyle.Render(strings.Join(helpItems, " • "))
}

// buildProgressBar creates a text-based progress bar
func buildProgressBar(completed, total, width int) string {
	if total == 0 {
		return strings.Repeat("░", width)
	}

	filledCount := (completed * width) / total
	if filledCount > width {
		filledCount = width
	}

	filled := strings.Repeat("█", filledCount)
	empty := strings.Repeat("░", width-filledCount)
	return filled + empty
}
