package calibrate

import (
	"fmt"
	"strings"
)

const percent = 100

// RenderMarkdown renders the report as Markdown: the figures, then every
// example the check got wrong or could not evaluate.
func RenderMarkdown(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Check Calibration: %s\n\n", r.Check.Type)
	b.WriteString("How far the check agrees with labeled examples. A pass label means a person judged the " +
		"check should pass. Rates have 95% Wilson intervals. Kappa corrects agreement for what chance " +
		"alone would give: 1 is perfect, 0 no better than chance.\n\n")
	fmt.Fprintf(&b, "Examples: %d, evaluated: %d", r.Examples, r.Evaluated)
	if r.Errored > 0 {
		fmt.Fprintf(&b, ", not evaluated: %d", r.Errored)
	}
	b.WriteString("\n\n| Measure | Value | 95% CI |\n|---|---|---|\n")
	row := func(name string, k, n int, v, low, high float64) {
		if n == 0 {
			fmt.Fprintf(&b, "| %s | – | – |\n", name)
			return
		}
		fmt.Fprintf(&b, "| %s | %d / %d (%.1f%%) | %.1f%%–%.1f%% |\n", name, k, n, v*percent, low*percent,
			high*percent)
	}
	row("Agreement", r.TruePass+r.TrueFail, r.Evaluated, r.Agreement, r.AgreementLow, r.AgreementHigh)
	row("Passes pass-labeled examples", r.TruePass, r.TruePass+r.FalseFail, r.PassRecall,
		r.PassRecallLow, r.PassRecallHigh)
	row("Fails fail-labeled examples", r.TrueFail, r.TrueFail+r.FalsePass, r.FailRecall,
		r.FailRecallLow, r.FailRecallHigh)
	fmt.Fprintf(&b, "| Cohen's kappa | %.2f | – |\n\n", r.Kappa)

	b.WriteString("| | Labeled pass | Labeled fail |\n|---|---|---|\n")
	fmt.Fprintf(&b, "| Check passed | %d | %d |\n| Check failed | %d | %d |\n", r.TruePass, r.FalsePass,
		r.FalseFail, r.TrueFail)

	var wrong []Outcome
	for i := range r.Outcomes {
		if !r.Outcomes[i].Agrees() {
			wrong = append(wrong, r.Outcomes[i])
		}
	}
	if len(wrong) == 0 {
		b.WriteString("\nThe check agreed with every evaluated example.\n")
		return b.String()
	}
	b.WriteString("\n## Disagreements\n\n| Example | Label | Check | Message | Note |\n|---|---|---|---|---|\n")
	for i := range wrong {
		o := &wrong[i]
		verdict := verdictName(o.Passed)
		if o.Error != "" {
			verdict = "not evaluated: " + o.Error
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", cell(o.ID), verdictName(o.Label), cell(verdict),
			cell(o.Message), cell(o.Note))
	}
	return b.String()
}

func verdictName(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}

// cell keeps a value inside one Markdown table cell.
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.Join(strings.Fields(s), " ")
}
