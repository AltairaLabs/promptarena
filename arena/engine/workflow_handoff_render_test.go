package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/statestore"

	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// handoffKitFiles is a two-state workflow kit: intake escalates to specialist
// within the first turn (the mock calls workflow__transition on round one),
// so round two runs on the specialist prompt the handoff renders. The
// specialist prompt is filled in per test.
const (
	handoffArenaYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: handoff-render
spec:
  prompt_configs:
    - id: intake
      file: prompts/intake.yaml
    - id: specialist
      file: prompts/specialist.yaml
  providers:
    - file: providers/mock.provider.yaml
  scenarios:
    - file: scenarios/escalate.scenario.yaml
  workflow:
    version: 1
    entry: intake
    states:
      intake:
        prompt_task: intake
        on_event:
          Escalate: specialist
      specialist:
        prompt_task: specialist
        artifacts:
          summary:
            type: text/plain
        on_event:
          Resolve: intake
`
	handoffIntakeYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: intake
spec:
  task_type: intake
  version: v1.0.0
  system_template: "You are intake for {{company}}."
`
	handoffProviderYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: mock-handoff
spec:
  id: mock-handoff
  type: mock
  model: mock-model
  additional_config:
    mock_config: mock-responses.yaml
`
	handoffScenarioYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: escalate
spec:
  id: escalate
  task_type: intake
  variables:
    company: Acme
  turns:
    - role: user
      content: "I need a specialist."
`
	// handoffSecondTurn is a second scripted turn, which the specialist
	// answers from the start.
	handoffSecondTurn = `    - role: user
      content: "Are you still there?"
`
	handoffMockYAML = `scenarios:
  escalate:
    turns:
      1:
        response: "Escalating."
        tool_calls:
          - name: workflow__transition
            arguments:
              event: Escalate
              context: "customer wants a refund"
      2: "Specialist here."
      3: "Still the specialist."
`
)

// writeHandoffKit writes the kit to a temp dir with the given specialist
// prompt (a PromptConfig spec body: variables and system_template).
func writeHandoffKit(t *testing.T, specialistSpec string, extraTurns ...string) string {
	t.Helper()
	scenario := handoffScenarioYAML
	for _, turn := range extraTurns {
		scenario += turn
	}
	dir := t.TempDir()
	files := map[string]string{
		"config.arena.yaml":                handoffArenaYAML,
		"prompts/intake.yaml":              handoffIntakeYAML,
		"providers/mock.provider.yaml":     handoffProviderYAML,
		"mock-responses.yaml":              handoffMockYAML,
		"scenarios/escalate.scenario.yaml": scenario,
		"prompts/specialist.yaml": `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: specialist
spec:
  task_type: specialist
  version: v1.0.0
` + specialistSpec,
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return dir
}

// systemPromptRecorder is the kit's mock provider with every system prompt it
// is sent recorded. It stands in for the mock under the same ID, so it sets
// the scenario ID the arena's mock-context stage would have set: that stage
// keys on the concrete mock type.
type systemPromptRecorder struct {
	*mock.ToolProvider
	scenarioID string

	mu      sync.Mutex
	systems []string
}

func (p *systemPromptRecorder) record(req *providers.PredictionRequest) {
	p.mu.Lock()
	p.systems = append(p.systems, req.System)
	p.mu.Unlock()
	md := make(map[string]any, len(req.Metadata)+1)
	for k, v := range req.Metadata {
		md[k] = v
	}
	md["mock_scenario_id"] = p.scenarioID
	req.Metadata = md
}

func (p *systemPromptRecorder) PredictWithTools(
	ctx context.Context, req providers.PredictionRequest, tools providers.ProviderTools, toolChoice string,
) (providers.PredictionResponse, []types.MessageToolCall, error) {
	p.record(&req)
	return p.ToolProvider.PredictWithTools(ctx, req, tools, toolChoice)
}

func (p *systemPromptRecorder) PredictStreamWithTools(
	ctx context.Context, req providers.PredictionRequest, tools interface{}, toolChoice string,
) (<-chan providers.StreamChunk, error) {
	p.record(&req)
	return p.ToolProvider.PredictStreamWithTools(ctx, req, tools, toolChoice)
}

func (p *systemPromptRecorder) prompts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.systems...)
}

// runHandoffKit runs the kit's one scenario through the real engine and
// returns the system prompt of every provider call and the stored result.
func runHandoffKit(t *testing.T, dir string) ([]string, *statestore.RunResult) {
	t.Helper()
	eng, err := NewEngineFromConfigFile(filepath.Join(dir, "config.arena.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close() })

	prov, ok := eng.providerRegistry.Get("mock-handoff")
	require.True(t, ok)
	toolMock, ok := prov.(*mock.ToolProvider)
	require.True(t, ok, "the kit's provider is the mock tool provider, got %T", prov)
	rec := &systemPromptRecorder{ToolProvider: toolMock, scenarioID: "escalate"}
	eng.providerRegistry.Register(rec)

	plan := &RunPlan{Combinations: []RunCombination{
		{Region: "default", ScenarioID: "escalate", ProviderID: "mock-handoff"},
	}}
	runIDs, err := eng.ExecuteRuns(context.Background(), plan, 1)
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	store, ok := eng.GetStateStore().(*statestore.ArenaStateStore)
	require.True(t, ok)
	result, err := store.GetResult(context.Background(), runIDs[0])
	require.NoError(t, err)
	return rec.prompts(), result
}

// The handoff renders the destination's prompt with the run's variables (the
// scenario's company), the brief the outgoing state wrote, and an artifact the
// state declares but nothing has set, which renders empty. Before the fix the
// render failed on {{company}} and the provider was sent the raw template.
func TestWorkflowHandoff_RendersDestinationWithRunVariables(t *testing.T) {
	dir := writeHandoffKit(t, `  system_template: "You are the {{company}} specialist. Summary: [{{artifacts.summary}}]. Brief: {{workflow_context}}"
`)

	prompts, result := runHandoffKit(t, dir)
	require.Empty(t, result.Error)
	require.Len(t, prompts, 2, "round one as intake, round two as specialist")
	assert.Equal(t, "You are intake for Acme.", prompts[0])
	assert.Equal(t, "You are the Acme specialist. Summary: []. Brief: customer wants a refund", prompts[1])
}

// A required variable nothing supplies fails the run rather than sending the
// destination's raw template.
func TestWorkflowHandoff_UnsetRequiredVariableFailsTheRun(t *testing.T) {
	dir := writeHandoffKit(t, `  variables:
    - name: ticket_id
      required: true
  system_template: "Specialist for ticket {{ticket_id}}."
`)

	prompts, result := runHandoffKit(t, dir)
	assert.Contains(t, result.Error, "system prompt has unresolved variables")
	assert.Contains(t, result.Error, "missing required variable ticket_id")
	assert.Equal(t, []string{"You are intake for Acme."}, prompts,
		"the provider must never be sent the specialist's raw template")
}

// An artifact no state declares is a mistake in the pack, not an empty value:
// it fails the run rather than sending the raw template.
func TestWorkflowHandoff_UndeclaredArtifactFailsTheRun(t *testing.T) {
	dir := writeHandoffKit(t, `  system_template: "Specialist. Notes: {{artifacts.notes}}"
`)

	prompts, result := runHandoffKit(t, dir)
	assert.Contains(t, result.Error, "system prompt has unresolved variables")
	assert.Contains(t, result.Error, "artifacts.notes")
	assert.Equal(t, []string{"You are intake for Acme."}, prompts,
		"the provider must never be sent the specialist's raw template")
}

// A scripted turn after the handoff renders the specialist's prompt through
// the template stage, which must bind what the handoff bound: the declared
// artifact (empty until set) and the brief. Without them the turn failed on
// {{artifacts.summary}}, which no turn bound.
func TestWorkflowHandoff_LaterTurnRendersTheStateLikeTheHandoff(t *testing.T) {
	dir := writeHandoffKit(t, `  system_template: "You are the {{company}} specialist. Summary: [{{artifacts.summary}}]. Brief: {{workflow_context}}"
`, handoffSecondTurn)

	prompts, result := runHandoffKit(t, dir)
	require.Empty(t, result.Error)
	specialist := "You are the Acme specialist. Summary: []. Brief: customer wants a refund"
	require.Len(t, prompts, 3)
	assert.Equal(t, []string{"You are intake for Acme.", specialist}, prompts[:2])
	// The second turn's request also carries the first turn's stored system
	// message, which the provider request folds in after this turn's prompt
	// (PredictionRequest.NormalizeMessages); this turn's own prompt comes first.
	turnTwo, _, _ := strings.Cut(prompts[2], "\n\n")
	assert.Equal(t, specialist, turnTwo)
}

// replaceInKitFile rewrites one kit file, failing if old is not in it.
func replaceInKitFile(t *testing.T, dir, name, old, replacement string) {
	t.Helper()
	path := filepath.Join(dir, name)
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	updated := strings.Replace(string(b), old, replacement, 1)
	require.NotEqual(t, string(b), updated, "fixture drifted: %q not in %s", old, name)
	require.NoError(t, os.WriteFile(path, []byte(updated), 0o600))
}

// A scripted turn renders with its own task's arena.yaml prompt-config vars,
// so the handoff into a state renders with the destination's, not the ones
// the turn started with.
func TestWorkflowHandoff_UsesTheDestinationsPromptConfigVars(t *testing.T) {
	dir := writeHandoffKit(t, `  system_template: "Specialist for {{team}}."
`)
	replaceInKitFile(t, dir, "config.arena.yaml", `      file: prompts/intake.yaml
`, `      file: prompts/intake.yaml
      vars:
        team: Front desk
`)
	replaceInKitFile(t, dir, "config.arena.yaml", `      file: prompts/specialist.yaml
`, `      file: prompts/specialist.yaml
      vars:
        team: Billing
`)

	prompts, result := runHandoffKit(t, dir)
	require.Empty(t, result.Error)
	assert.Equal(t, []string{"You are intake for Acme.", "Specialist for Billing."}, prompts)
}

// An artifact set during the turn outranks a variable of the same name the
// turn started with: the handoff renders the value the tool just set.
func TestWorkflowHandoff_ArtifactSetThisTurnOutranksTurnVariables(t *testing.T) {
	dir := writeHandoffKit(t, `  system_template: "Summary: [{{artifacts.summary}}]"
`)
	replaceInKitFile(t, dir, "config.arena.yaml", `        prompt_task: intake
`, `        prompt_task: intake
        artifacts:
          summary:
            type: text/plain
`)
	replaceInKitFile(t, dir, "scenarios/escalate.scenario.yaml", `    company: Acme
`, `    company: Acme
    artifacts.summary: from-scenario
`)
	replaceInKitFile(t, dir, "mock-responses.yaml", `        tool_calls:
`, `        tool_calls:
          - name: workflow__set_artifact
            arguments:
              name: summary
              value: from-tool
`)

	prompts, result := runHandoffKit(t, dir)
	require.Empty(t, result.Error)
	assert.Equal(t, []string{"You are intake for Acme.", "Summary: [from-tool]"}, prompts)
}
