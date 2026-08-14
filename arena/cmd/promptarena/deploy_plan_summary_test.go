package main

import "testing"

// Adapters disagree on whether PlanResponse.Summary self-labels: agentcore and
// omnia build "Plan: 2 to create, ...", vertex returns a bare "1 to create".
// The label has to survive both without ever doubling.
func TestFormatPlanSummary(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		want    string
	}{
		{
			name:    "self-labelled adapter summary is not doubled",
			summary: "Plan: 2 to create, 0 to update, 0 to delete",
			want:    "Plan: 2 to create, 0 to update, 0 to delete",
		},
		{
			name:    "bare adapter summary gains the label",
			summary: "1 to create, 2 unchanged",
			want:    "Plan: 1 to create, 2 unchanged",
		},
		{
			name:    "vertex no-change summary gains the label",
			summary: "No changes",
			want:    "Plan: No changes",
		},
		{
			name:    "surrounding whitespace is normalized",
			summary: "  Plan:   3 to update  ",
			want:    "Plan: 3 to update",
		},
		{
			name:    "empty summary degrades to the bare label",
			summary: "",
			want:    "Plan:",
		},
		{
			name:    "label-only summary stays the bare label",
			summary: "Plan:",
			want:    "Plan:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPlanSummary(tt.summary); got != tt.want {
				t.Errorf("formatPlanSummary(%q) = %q, want %q", tt.summary, got, tt.want)
			}
		})
	}
}

// Applying the formatter twice must be a no-op — otherwise any future caller
// that formats an already-formatted summary reintroduces the doubling bug.
func TestFormatPlanSummaryIsIdempotent(t *testing.T) {
	for _, summary := range []string{
		"Plan: 2 to create, 0 to update, 0 to delete",
		"1 to create, 2 unchanged",
		"No changes",
		"",
	} {
		once := formatPlanSummary(summary)
		if twice := formatPlanSummary(once); twice != once {
			t.Errorf("formatPlanSummary(%q): once = %q, twice = %q", summary, once, twice)
		}
	}
}
