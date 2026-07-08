package views

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"

	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
	"github.com/AltairaLabs/promptarena/arena/tui/theme"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

const (
	deploySymbolColWidth = 2
	deployTypeColWidth   = 20
	deployNameColWidth   = 24
	deployStatusColWidth = 10
	deployNotesColWidth  = 30
	deployColFloor       = 4
	deployTableHeight    = 10
	deployWidthPadding   = 8

	// deployNotesColTitle is the "Notes" column header.
	deployNotesColTitle = "Notes"
	// deployStatusColTitle is the "Status" column header.
	deployStatusColTitle = "Status"
)

// DeployResourceRow is a row of the deploy resource table, shared by the
// Apply-result and Status screens.
type DeployResourceRow struct {
	Symbol, Type, Name, Status, Detail string
}

// DeployRowsFromResults maps apply/destroy resource results to table rows.
func DeployRowsFromResults(results []*deploy.ResourceResult) []DeployResourceRow {
	rows := make([]DeployResourceRow, 0, len(results))
	for _, r := range results {
		rows = append(rows, DeployResourceRow{
			Symbol: flow.StatusSymbol(r.Status), Type: r.Type, Name: r.Name,
			Status: r.Status, Detail: r.Detail,
		})
	}
	return rows
}

// deployStatusSymbol maps a live status ("healthy"/"unhealthy"/"missing") to its glyph.
func deployStatusSymbol(status string) string {
	switch status {
	case "healthy":
		return "✓"
	case "unhealthy":
		return "✗"
	case "missing":
		return "?"
	default:
		return " "
	}
}

// DeployRowsFromStatus maps a status-check response to table rows.
func DeployRowsFromStatus(statuses []deploy.ResourceStatus) []DeployResourceRow {
	rows := make([]DeployResourceRow, 0, len(statuses))
	for _, s := range statuses {
		rows = append(rows, DeployResourceRow{
			Symbol: deployStatusSymbol(s.Status), Type: s.Type, Name: s.Name,
			Status: s.Status, Detail: s.Detail,
		})
	}
	return rows
}

// responsiveDeployColumns scales the fixed column proportions to fit avail,
// mirroring panels.responsiveRunsColumns so the table never overflows.
func responsiveDeployColumns(avail int) []table.Column {
	titles := []string{"", "Type", "Name", deployStatusColTitle, deployNotesColTitle}
	weights := []int{
		deploySymbolColWidth, deployTypeColWidth, deployNameColWidth,
		deployStatusColWidth, deployNotesColWidth,
	}
	target := avail - len(weights)
	if target < deployColFloor*len(weights) {
		target = deployColFloor * len(weights)
	}
	sum := 0
	for _, w := range weights {
		sum += w
	}
	cols := make([]table.Column, len(weights))
	used := 0
	for i, w := range weights {
		cw := w * target / sum
		if cw < deployColFloor {
			cw = deployColFloor
		}
		cols[i] = table.Column{Title: titles[i], Width: cw}
		used += cw
	}
	if rem := target - used; rem > 0 {
		cols[2].Width += rem // hand rounding slack to the flexible Name column
	}
	return cols
}

// RenderDeployResources renders a resource table for the Apply-result and
// Status screens, following the panels.RunsPanel idiom: a bubbles/table with
// responsive columns inside a rounded, unfocused-styled border.
func RenderDeployResources(rows []DeployResourceRow, width int) string {
	tableRows := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		tableRows = append(tableRows, table.Row{r.Symbol, r.Type, r.Name, r.Status, r.Detail})
	}
	t := table.New(
		table.WithColumns(responsiveDeployColumns(width-deployWidthPadding)),
		table.WithRows(tableRows),
		table.WithHeight(deployTableHeight),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(theme.BorderColorFocused()).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorViolet))
	s.Selected = lipgloss.NewStyle()
	t.SetStyles(s)
	t.SetWidth(width - deployWidthPadding)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderColorUnfocused()).
		Padding(theme.BoxPaddingVertical, theme.BoxPaddingHorizontal).
		Render(t.View())
}
