package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/AltairaLabs/promptarena/arena/tui/theme"
	"github.com/AltairaLabs/promptarena/arena/tui/viewmodels"
)

// SummaryView renders the final execution summary
type SummaryView struct {
	width  int
	ciMode bool
}

// NewSummaryView creates a new summary view
func NewSummaryView(width int, ciMode bool) *SummaryView {
	return &SummaryView{
		width:  width,
		ciMode: ciMode,
	}
}

// Render renders the summary using the provided ViewModel
func (v *SummaryView) Render(vm *viewmodels.SummaryViewModel) string {
	if v.ciMode {
		return v.renderCIMode(vm)
	}
	return v.renderTUIMode(vm)
}

func (v *SummaryView) renderTUIMode(vm *viewmodels.SummaryViewModel) string {
	var sb strings.Builder

	width := v.width
	if width <= 0 {
		width = 80
	}
	ruleWidth := min(width, summaryRuleWidth)

	labelStyle := lipgloss.NewStyle().Foreground(theme.Colors().TextMuted)
	valueStyle := lipgloss.NewStyle().Foreground(theme.Colors().TextHeading)
	successStyle := lipgloss.NewStyle().Foreground(theme.Colors().StatusHealthyText)
	failStyle := lipgloss.NewStyle().Foreground(theme.Colors().StatusErrorText)
	headingStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Colors().TextHeading)
	eyebrowStyle := lipgloss.NewStyle().Foreground(theme.Colors().TextMuted)
	ruleStyle := lipgloss.NewStyle().Foreground(theme.Colors().BorderDefault)

	// Title: bold heading over a hairline rule (the Atlas "the line defines the
	// section" idiom, no ASCII box).
	sb.WriteString(headingStyle.Render("Run Summary"))
	sb.WriteString("\n")
	sb.WriteString(ruleStyle.Render(strings.Repeat("─", ruleWidth)))
	sb.WriteString("\n\n")

	// Headline counts.
	sb.WriteString(summaryRow("Total runs", vm.GetFormattedTotalRuns(), labelStyle, valueStyle))
	sb.WriteString(summaryRow("Passed", vm.GetFormattedSuccessful(), labelStyle, successStyle))
	sb.WriteString(summaryRow("Failed", vm.GetFormattedFailed(), labelStyle, failStyle))
	sb.WriteString("\n")

	// Assertions (if any).
	if vm.HasAssertions() {
		sb.WriteString(summaryRow("Assertions", vm.GetFormattedAssertionTotal(), labelStyle, valueStyle))
		if vm.HasFailedAssertions() {
			sb.WriteString(summaryRow("Assertions Fail", vm.GetFormattedAssertionFailed(), labelStyle, failStyle))
		} else {
			sb.WriteString(summaryRow("Assertions Pass", "all passed", labelStyle, successStyle))
		}
		sb.WriteString("\n")
	}

	// Cost and performance.
	sb.WriteString(summaryRow("Total cost", vm.GetFormattedTotalCost(), labelStyle, valueStyle))
	sb.WriteString(summaryRow("Total tokens", vm.GetFormattedTotalTokens(), labelStyle, valueStyle))
	sb.WriteString(summaryRow("Total duration", vm.GetFormattedTotalDuration(), labelStyle, valueStyle))
	sb.WriteString(summaryRow("Avg duration", vm.GetFormattedAvgDurationWithSuffix(), labelStyle, valueStyle))
	sb.WriteString("\n")

	// Breakdown.
	if vm.HasProviders() {
		sb.WriteString(summaryRow("Providers", vm.GetFormattedProviders(), labelStyle, valueStyle))
	}
	sb.WriteString(summaryRow("Scenarios", vm.GetFormattedScenarios(), labelStyle, valueStyle))
	if vm.HasRegions() {
		sb.WriteString(summaryRow("Regions", vm.GetFormattedRegions(), labelStyle, valueStyle))
	}

	// Failures: an eyebrow header then one truncated line per failure, so long
	// validation messages never wrap into a ragged block.
	if vm.HasErrors() {
		sb.WriteString("\n")
		sb.WriteString(eyebrowStyle.Render(theme.Eyebrow("Failures")))
		sb.WriteString("\n")
		for _, errStr := range vm.GetFormattedErrors() {
			line := truncateToWidth("• "+errStr, width-summaryErrorIndent)
			sb.WriteString(strings.Repeat(" ", summaryErrorIndent))
			sb.WriteString(failStyle.Render(line))
			sb.WriteString("\n")
		}
	}

	// Output location.
	sb.WriteString("\n")
	sb.WriteString(summaryRow("Saved to", vm.GetOutputDir(), labelStyle, valueStyle))

	return sb.String()
}

func (v *SummaryView) renderCIMode(vm *viewmodels.SummaryViewModel) string {
	var sb strings.Builder

	// Header — plain and deterministic for CI logs.
	sb.WriteString("Run Summary\n===========\n\n")

	// Execution stats
	sb.WriteString(fmt.Sprintf("Total Runs:       %s\n", vm.GetFormattedTotalRuns()))
	sb.WriteString(fmt.Sprintf("Successful:       %s\n", vm.GetFormattedSuccessful()))
	sb.WriteString(fmt.Sprintf("Failed:           %s\n", vm.GetFormattedFailed()))
	sb.WriteString("\n")

	// Assertions (if any)
	if vm.HasAssertions() {
		sb.WriteString(fmt.Sprintf("Assertions:       %s\n", vm.GetFormattedAssertionTotal()))
		if vm.HasFailedAssertions() {
			sb.WriteString(fmt.Sprintf("Assertions Fail:  %s\n", vm.GetFormattedAssertionFailed()))
		} else {
			sb.WriteString("Assertions Pass:  all passed\n")
		}
		sb.WriteString("\n")
	}

	// Cost and performance
	sb.WriteString(fmt.Sprintf("Total Cost:       %s\n", vm.GetFormattedTotalCost()))
	sb.WriteString(fmt.Sprintf("Total Tokens:     %s\n", vm.GetFormattedTotalTokens()))
	sb.WriteString(fmt.Sprintf("Total Duration:   %s\n", vm.GetFormattedTotalDuration()))
	sb.WriteString(fmt.Sprintf("Avg Duration:     %s\n", vm.GetFormattedAvgDurationWithSuffix()))
	sb.WriteString("\n")

	// Provider breakdown
	if vm.HasProviders() {
		sb.WriteString(fmt.Sprintf("Providers:        %s\n", vm.GetFormattedProviders()))
	}

	sb.WriteString(fmt.Sprintf("Scenarios:        %s\n", vm.GetFormattedScenarios()))

	if vm.HasRegions() {
		sb.WriteString(fmt.Sprintf("Regions:          %s\n", vm.GetFormattedRegions()))
	}

	// Errors
	if vm.HasErrors() {
		sb.WriteString("\nErrors:\n")
		for _, errStr := range vm.GetFormattedErrors() {
			sb.WriteString(fmt.Sprintf("  • %s\n", errStr))
		}
	}

	// Output info
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Results saved to: %s\n", vm.GetOutputDir()))

	return sb.String()
}

const (
	// summaryLabelWidth is the fixed column the values align to.
	summaryLabelWidth = 16
	// summaryRuleWidth caps the title hairline so it stays a tidy underline
	// rather than spanning a very wide terminal.
	summaryRuleWidth = 52
	// summaryErrorIndent is the left inset of each failure line.
	summaryErrorIndent = 2
)

// summaryRow renders one aligned "label   value" line: the label padded to a
// fixed column in labelStyle, the value in valueStyle.
func summaryRow(label, value string, labelStyle, valueStyle lipgloss.Style) string {
	lbl := labelStyle.Render(fmt.Sprintf("%-*s", summaryLabelWidth, label))
	return lbl + valueStyle.Render(value) + "\n"
}

// truncateToWidth clips s to max display columns, appending an ellipsis when it
// would overflow, so a long line never wraps into a ragged block.
func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 1 || lipgloss.Width(s) <= maxWidth {
		return s
	}
	r := []rune(s)
	if len(r) <= maxWidth-1 {
		return s
	}
	return string(r[:maxWidth-1]) + "…"
}
