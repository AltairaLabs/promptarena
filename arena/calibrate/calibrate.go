// Package calibrate scores a check, such as an llm_judge assertion, against
// examples a person has labeled, so you know how far to trust it: how often
// it agrees with the labels, how it errs, and Cohen's kappa, each with a
// confidence interval where one applies.
package calibrate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"

	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/results/objectives"
)

// roleAssistant is the role of the reply a check judges.
const roleAssistant = "assistant"

// Example is one labeled reply.
type Example struct {
	// ID names the example in the report. Defaults to its line number.
	ID string `json:"id,omitempty"`
	// Input is the user message the reply answers. Optional.
	Input string `json:"input,omitempty"`
	// Content is the reply the check judges.
	Content string `json:"content,omitempty"`
	// Messages is a whole conversation instead of Input and Content; the
	// check judges it as it would a run's conversation.
	Messages []types.Message `json:"messages,omitempty"`
	// Label is a person's verdict: true when the check should pass.
	Label *bool `json:"label"`
	// Note is a free-text note carried into the report.
	Note string `json:"note,omitempty"`
}

// conversation returns the messages the check runs over.
func (ex *Example) conversation() []types.Message {
	if len(ex.Messages) > 0 {
		return ex.Messages
	}
	var msgs []types.Message
	if ex.Input != "" {
		msgs = append(msgs, types.Message{Role: "user", Content: ex.Input})
	}
	return append(msgs, types.Message{Role: roleAssistant, Content: ex.Content})
}

// LoadExamples reads labeled examples from a JSONL file, one object per
// line. Blank lines are skipped.
func LoadExamples(path string) ([]Example, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-provided file path
	if err != nil {
		return nil, fmt.Errorf("read examples: %w", err)
	}
	var out []Example
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) //nolint:mnd // a long conversation fits on one line
	for line := 1; sc.Scan(); line++ {
		text := bytes.TrimSpace(sc.Bytes())
		if len(text) == 0 {
			continue
		}
		var ex Example
		if err := json.Unmarshal(text, &ex); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, line, err)
		}
		if ex.ID == "" {
			ex.ID = fmt.Sprintf("line %d", line)
		}
		if err := validateExample(&ex, path, line, seen); err != nil {
			return nil, err
		}
		seen[ex.ID] = true
		out = append(out, ex)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read examples: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s has no examples", path)
	}
	return out, nil
}

// validateExample checks one example loaded from line of path.
func validateExample(ex *Example, path string, line int, seen map[string]bool) error {
	switch {
	case ex.Label == nil:
		return fmt.Errorf("%s line %d (%s): label is required (true when the check should pass)",
			path, line, ex.ID)
	case ex.Content == "" && len(ex.Messages) == 0:
		return fmt.Errorf("%s line %d (%s): needs content or messages", path, line, ex.ID)
	case len(ex.Messages) > 0 && (ex.Content != "" || ex.Input != ""):
		return fmt.Errorf("%s line %d (%s): set messages or input and content, not both", path, line, ex.ID)
	case len(ex.Messages) > 0 && ex.Messages[len(ex.Messages)-1].Role != roleAssistant:
		return fmt.Errorf("%s line %d (%s): messages must end with the assistant reply the check judges",
			path, line, ex.ID)
	case seen[ex.ID]:
		return fmt.Errorf("%s line %d: duplicate id %q (an example without an id is named \"line <n>\")",
			path, line, ex.ID)
	}
	return nil
}

// CheckRunner runs a check over a conversation; engine.Engine.RunCheck is one.
type CheckRunner func(ctx context.Context, check asrt.AssertionConfig, messages []types.Message,
	sessionID string) asrt.ConversationValidationResult

// Outcome is the check's verdict on one example.
type Outcome struct {
	ID     string   `json:"id"`
	Label  bool     `json:"label"`
	Passed bool     `json:"passed"`
	Score  *float64 `json:"score,omitempty"`
	// Message is why the check reached its verdict, as its handler explains.
	Message string `json:"message,omitempty"`
	Note    string `json:"note,omitempty"`
	// Error is set when the check could not be evaluated; the example is
	// then left out of every figure.
	Error string `json:"error,omitempty"`
}

// Agrees reports whether the check's verdict matches the label.
func (o *Outcome) Agrees() bool { return o.Error == "" && o.Passed == o.Label }

// Report is how far a check agrees with labeled examples. A positive is an
// example the check should pass; the counts compare the check's verdicts
// with the labels.
type Report struct {
	Check     asrt.AssertionConfig `json:"check"`
	Examples  int                  `json:"examples"`
	Evaluated int                  `json:"evaluated"`
	Errored   int                  `json:"errored,omitempty"`
	// TruePass and TrueFail are verdicts that match their labels. FalsePass
	// passed an example labeled fail; FalseFail failed one labeled pass.
	TruePass  int `json:"true_pass"`
	FalsePass int `json:"false_pass"`
	TrueFail  int `json:"true_fail"`
	FalseFail int `json:"false_fail"`
	// Agreement is the share of evaluated examples the check got right, with
	// a 95% Wilson interval.
	Agreement     float64 `json:"agreement"`
	AgreementLow  float64 `json:"agreement_ci_low"`
	AgreementHigh float64 `json:"agreement_ci_high"`
	// PassRecall is the share of pass-labeled examples the check passed;
	// FailRecall the share of fail-labeled examples it failed. Each has a
	// 95% Wilson interval. With no examples of that label, the rate and
	// both bounds are 0.
	PassRecall     float64 `json:"pass_recall"`
	PassRecallLow  float64 `json:"pass_recall_ci_low"`
	PassRecallHigh float64 `json:"pass_recall_ci_high"`
	FailRecall     float64 `json:"fail_recall"`
	FailRecallLow  float64 `json:"fail_recall_ci_low"`
	FailRecallHigh float64 `json:"fail_recall_ci_high"`
	// PassPrecision is the share of the check's passes that were labeled
	// pass.
	PassPrecision float64 `json:"pass_precision"`
	// Kappa is Cohen's kappa: agreement corrected for the agreement chance
	// alone would give. 1 is perfect, 0 no better than chance.
	Kappa float64 `json:"kappa"`
	// Outcomes lists every example's verdict, in file order.
	Outcomes []Outcome `json:"outcomes"`
}

// Run scores the check against every example, in order.
func Run(ctx context.Context, check asrt.AssertionConfig, examples []Example, run CheckRunner) *Report {
	r := &Report{Check: check, Examples: len(examples)}
	for i := range examples {
		ex := &examples[i]
		if ctx.Err() != nil {
			// Canceled: the examples not yet scored are reported as such.
			r.Outcomes = append(r.Outcomes, Outcome{ID: ex.ID, Label: *ex.Label, Note: ex.Note,
				Error: "not run: " + ctx.Err().Error()})
			continue
		}
		res := run(ctx, check, ex.conversation(), "calibrate-"+ex.ID)
		o := Outcome{ID: ex.ID, Label: *ex.Label, Passed: res.Passed, Message: res.Message, Note: ex.Note}
		// A check with a configured message reports it in place of the
		// handler's explanation, which says why this verdict was reached.
		if why, _ := res.Details["explanation"].(string); why != "" {
			o.Message = why
		}
		if s, ok := res.Details["score"].(float64); ok {
			o.Score = &s
		}
		if errMsg, _ := res.Details["error"].(string); errMsg != "" {
			o.Error = errMsg
		} else if skipped, _ := res.Details["skipped"].(bool); skipped {
			o.Error = "skipped"
		}
		r.Outcomes = append(r.Outcomes, o)
	}
	r.tally()
	return r
}

func (r *Report) tally() {
	for i := range r.Outcomes {
		o := &r.Outcomes[i]
		if o.Error != "" {
			r.Errored++
			continue
		}
		r.Evaluated++
		switch {
		case o.Label && o.Passed:
			r.TruePass++
		case o.Label:
			r.FalseFail++
		case o.Passed:
			r.FalsePass++
		default:
			r.TrueFail++
		}
	}
	agree := r.TruePass + r.TrueFail
	r.Agreement = ratio(agree, r.Evaluated)
	r.AgreementLow, r.AgreementHigh = interval(agree, r.Evaluated)
	r.PassRecall = ratio(r.TruePass, r.TruePass+r.FalseFail)
	r.PassRecallLow, r.PassRecallHigh = interval(r.TruePass, r.TruePass+r.FalseFail)
	r.FailRecall = ratio(r.TrueFail, r.TrueFail+r.FalsePass)
	r.FailRecallLow, r.FailRecallHigh = interval(r.TrueFail, r.TrueFail+r.FalsePass)
	r.PassPrecision = ratio(r.TruePass, r.TruePass+r.FalsePass)
	r.Kappa = kappa(r.TruePass, r.FalsePass, r.TrueFail, r.FalseFail)
}

// interval is the 95% Wilson interval for k of n, or 0 to 0 with nothing to
// measure.
func interval(k, n int) (low, high float64) {
	if n == 0 {
		return 0, 0
	}
	return objectives.Wilson(k, n)
}

func ratio(k, n int) float64 {
	if n == 0 {
		return 0
	}
	return float64(k) / float64(n)
}

// kappaEpsilon absorbs float error when the chance agreement is 1.
const kappaEpsilon = 1e-12

// kappa is Cohen's kappa for two raters, the check and the labels. When the
// agreement chance would give is already 1 (every verdict and label the
// same), kappa is 1 if they agree and 0 otherwise.
func kappa(truePass, falsePass, trueFail, falseFail int) float64 {
	n := float64(truePass + falsePass + trueFail + falseFail)
	if n == 0 {
		return 0
	}
	observed := float64(truePass+trueFail) / n
	checkPass := float64(truePass+falsePass) / n
	labelPass := float64(truePass+falseFail) / n
	chance := checkPass*labelPass + (1-checkPass)*(1-labelPass)
	if math.Abs(1-chance) < kappaEpsilon {
		if observed == 1 {
			return 1
		}
		return 0
	}
	return (observed - chance) / (1 - chance)
}
