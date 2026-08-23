package adaptersdk

import (
	"encoding/json"
	"fmt"

	"github.com/AltairaLabs/promptarena/deploy"

	"github.com/AltairaLabs/PromptKit/runtime/prompt"
)

// percentMultiplier converts a 0.0-1.0 fraction to a 0-100 percentage.
const percentMultiplier = 100

// maxPercent is the upper bound for a valid percentage.
const maxPercent = 100

// ParsePack deserializes a .pack.json byte slice into a prompt.Pack struct.
func ParsePack(packJSON []byte) (*prompt.Pack, error) {
	var pack prompt.Pack
	if err := json.Unmarshal(packJSON, &pack); err != nil {
		return nil, err
	}
	return &pack, nil
}

// ProgressReporter wraps a deploy.ApplyCallback and provides convenient
// methods for emitting progress, resource, and error events.
type ProgressReporter struct {
	callback deploy.ApplyCallback
}

// NewProgressReporter creates a ProgressReporter that sends events through
// the given ApplyCallback.
func NewProgressReporter(callback deploy.ApplyCallback) *ProgressReporter {
	return &ProgressReporter{callback: callback}
}

// Progress emits a progress event with a human-readable message and a
// completion percentage (0.0 to 1.0).
//
// A nil reporter or a reporter with no callback emits nothing and returns nil.
// Apply's callback is optional — every adapter's own tests call Apply(ctx, req,
// nil) — so a reporter built around a nil callback is a normal state, not a
// programming error. Dereferencing it panicked mid-apply, after resources had
// already been created, which is the worst possible moment: the caller gets a
// stack trace instead of the state telling them what exists.
//
// Adapters worked around this by only constructing a reporter when the callback
// was non-nil. That is easy to forget, and one of them did.
func (pr *ProgressReporter) Progress(message string, pct float64) error {
	if pr == nil || pr.callback == nil {
		return nil
	}
	return pr.callback(&deploy.ApplyEvent{
		Type:    "progress",
		Message: formatProgress(message, pct),
	})
}

// Resource emits a resource result event.
func (pr *ProgressReporter) Resource(result *deploy.ResourceResult) error {
	if pr == nil || pr.callback == nil {
		return nil
	}
	return pr.callback(&deploy.ApplyEvent{
		Type:     "resource",
		Resource: result,
	})
}

// Error emits an error event.
func (pr *ProgressReporter) Error(err error) error {
	if pr == nil || pr.callback == nil {
		return nil
	}
	return pr.callback(&deploy.ApplyEvent{
		Type:    "error",
		Message: err.Error(),
	})
}

// formatProgress builds a progress message string that includes the
// percentage when it is within the valid 0-100 range.
func formatProgress(message string, pct float64) string {
	pctInt := int(pct * percentMultiplier)
	if pctInt < 0 || pctInt > maxPercent {
		return message
	}
	return fmt.Sprintf("%s (%d%%)", message, pctInt)
}

// ConsoleLink builds the conventional "Console" link for a deployed resource.
// It is a convenience over constructing deploy.ResourceLink directly, so the
// common case reads the same across adapters.
//
// It returns nil when url is empty, which is the intended way to express "the
// console URL is not known". Callers can append the result unconditionally:
//
//	result.Links = append(result.Links, adaptersdk.ConsoleLink(consoleURL)...)
//
// An adapter must never substitute a guessed URL for an unknown one — a link
// that 404s or lands on the wrong workspace is worse than no link at all.
func ConsoleLink(url string) []deploy.ResourceLink {
	return Link("Console", url, "console")
}

// Link builds a single-element ResourceLink slice, or nil when url is empty.
// The nil-on-empty behavior is what makes "no link" the safe default at every
// call site rather than something each adapter has to remember to check.
func Link(label, url, rel string) []deploy.ResourceLink {
	if url == "" {
		return nil
	}
	return []deploy.ResourceLink{{Label: label, URL: url, Rel: rel}}
}
