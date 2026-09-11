package generate

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

const (
	apiVersion      = "promptkit.altairalabs.ai/v1alpha1"
	kindScenario    = "Scenario"
	maxIDLength     = 63
	roleUser        = "user"
	defaultTaskType = "conversation"
)

// ConvertOptions controls how a session is converted into a scenario.
type ConvertOptions struct {
	// TaskType sets the scenario's task_type when the session does not record
	// a workflow entry state. Defaults to "conversation".
	TaskType string
	// Expectations supply thresholds for measurements the session recorded
	// without a verdict and the pack did not bound. See Decision.
	Expectations []Expectation
}

// Conversion is the result of converting one session.
type Conversion struct {
	Scenario *arenaconfig.ScenarioConfig
	// Warnings are things the user should know about the scenario, such as a
	// multi-turn replay being unlikely to behave as recorded.
	Warnings []string
	// Decisions record, per recorded eval, what it became and why.
	Decisions []Decision
}

// Convert turns a session into a scenario document plus the warnings and
// per-eval decisions that explain it.
func Convert(session *SessionDetail, opts ConvertOptions) (*Conversion, error) {
	if session == nil {
		return nil, fmt.Errorf("session is nil")
	}

	scenarioID := sanitizeID(session.ID)
	scenario := arenaconfig.Scenario{
		ID:        scenarioID,
		TaskType:  resolveTaskType(session, opts),
		Turns:     buildTurns(session),
		Variables: session.Variables,
	}
	if len(scenario.Variables) == 0 {
		scenario.Variables = nil
	}

	conv := &Conversion{}
	if n := len(scenario.Turns); n > 1 {
		conv.Warnings = append(conv.Warnings, fmt.Sprintf(
			"session %s has %d user turns; later turns were written against the recorded answers, "+
				"so a replay may not behave as recorded", session.ID, n))
	}
	scenario.Description = buildDescription(session, len(scenario.Turns))

	applyEvalDecisions(&scenario, session, opts, conv)

	conv.Scenario = &arenaconfig.ScenarioConfig{
		APIVersion: apiVersion,
		Kind:       kindScenario,
		Metadata:   config.ObjectMeta{Name: scenarioID},
		Spec:       scenario,
	}
	return conv, nil
}

// resolveTaskType: the recorded workflow entry state's prompt_task wins,
// because that is how arena's resolveWorkflowStartState maps a scenario back
// to the state it starts in; then the option; then the default.
func resolveTaskType(session *SessionDetail, opts ConvertOptions) string {
	if session.Workflow != nil && session.Workflow.EntryPromptTask != "" {
		return session.Workflow.EntryPromptTask
	}
	if opts.TaskType != "" {
		return opts.TaskType
	}
	return defaultTaskType
}

// sanitizeID produces a valid scenario ID: lowercase alphanumeric with hyphens, max 63 chars.
func sanitizeID(raw string) string {
	re := regexp.MustCompile(`[^a-z0-9]+`)
	id := re.ReplaceAllString(strings.ToLower(raw), "-")
	id = strings.Trim(id, "-")
	if id == "" {
		id = "generated"
	}
	if len(id) > maxIDLength {
		id = strings.TrimRight(id[:maxIDLength], "-")
	}
	return id
}

func buildDescription(session *SessionDetail, userTurns int) string {
	parts := []string{fmt.Sprintf("Generated from session %s", session.ID)}
	if !session.Timestamp.IsZero() {
		parts = append(parts, fmt.Sprintf("recorded %s", session.Timestamp.UTC().Format(time.RFC3339)))
	}
	if p := session.Pack; p != nil && p.Name != "" {
		ref := "pack " + p.Name
		if p.Version != "" {
			ref += " " + p.Version
		}
		if p.Digest != "" {
			ref += " (" + p.Digest + ")"
		}
		parts = append(parts, ref)
	}
	if userTurns > 1 {
		parts = append(parts, fmt.Sprintf("%d user turns: replay may not behave as recorded", userTurns))
	}
	return strings.Join(parts, ", ")
}

// buildTurns makes one scenario turn per user message. Text stays in Content
// for readability; multimodal parts are carried across as parts.
func buildTurns(session *SessionDetail) []arenaconfig.TurnDefinition {
	var turns []arenaconfig.TurnDefinition
	for i := range session.Messages {
		msg := &session.Messages[i]
		if msg.Role != roleUser {
			continue
		}
		turn := arenaconfig.TurnDefinition{Role: roleUser, Content: msg.GetContent()}
		if hasMedia(msg.Parts) {
			turn.Parts = convertParts(msg.Parts)
		}
		turns = append(turns, turn)
	}
	return turns
}

func hasMedia(parts []types.ContentPart) bool {
	for _, p := range parts {
		if p.Media != nil {
			return true
		}
	}
	return false
}

func convertParts(parts []types.ContentPart) []arenaconfig.TurnContentPart {
	out := make([]arenaconfig.TurnContentPart, 0, len(parts))
	for _, p := range parts {
		tp := arenaconfig.TurnContentPart{Type: p.Type}
		if p.Text != nil {
			tp.Text = *p.Text
		}
		if m := p.Media; m != nil {
			tp.Media = &arenaconfig.TurnMediaContent{
				MIMEType:         m.MIMEType,
				URL:              deref(m.URL),
				FilePath:         deref(m.FilePath),
				Data:             deref(m.Data),
				StorageReference: deref(m.StorageReference),
				Detail:           deref(m.Detail),
				Caption:          deref(m.Caption),
			}
		}
		out = append(out, tp)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Decision is replaced by decision.go in the next task.
type Decision struct{}

// applyEvalDecisions is implemented in decision.go in the next task.
func applyEvalDecisions(*arenaconfig.Scenario, *SessionDetail, ConvertOptions, *Conversion) {}
