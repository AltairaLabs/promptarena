package arenaconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

// workflowSchemaArena is an arena config exercising every workflow and
// composition shape the schema has to accept. %s slots let a test break one
// field: the state's control and the agent step's termination.
const workflowSchemaArena = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: workflow-schema
spec:
  providers: []
  defaults: {}
  workflow:
    version: 1
    entry: route
    engine:
      budget:
        max_total_visits: 10
      timeout_hint: 30s
    states:
      route:
        prompt_task: route
        control: %s
        persistence: transient
        max_visits: 3
        on_max_visits: done
        on_event:
          Analyze: analyze
        artifacts:
          summary:
            type: text/plain
            mode: append
      analyze:
        orchestration: composition
        composition: flow
        on_event:
          Done: done
      done:
        prompt_task: done
        terminal: true
  compositions:
    flow:
      version: 1
      output: merge
      steps:
        - id: classify
          kind: prompt
          prompt_task: route
          input: "${input}"
        - id: check
          kind: branch
          predicate:
            path: "${classify.output.type}"
            op: equals
            value: paper
          then: research
          else: merge
        - id: research
          kind: agent
          prompt_task: route
          input:
            topic: "${input.topic}"
            depth: 2
          tools: [search]
          termination:
            %s
          modifiers:
            retry:
              max_attempts: 2
        - id: merge
          kind: parallel
          branches:
            - id: a
              kind: tool
              tool: search
              args: {q: "${input}"}
            - id: b
              kind: omnia.judge
          reduce:
            strategy: barrier
            into: results
`

func workflowSchemaDoc(control, termination string) []byte {
	return []byte(fmt.Sprintf(workflowSchemaArena, control, termination))
}

func TestArenaSchema_AcceptsWorkflowAndCompositions(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	require.NoError(t, config.ValidateArenaConfig(workflowSchemaDoc("agent", "max_steps: 3")))
	require.NoError(t, config.ValidateArenaConfig(workflowSchemaDoc("user", "tool_called: done")))
}

// A typo inside a workflow or a composition must fail the SCHEMA stage — the
// first thing LoadConfig runs — rather than reaching the business-logic
// validator, or the runtime, as the only check.
func TestLoadConfig_WorkflowAndCompositionTyposFailSchemaValidation(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")

	cases := []struct {
		name, control, termination, wantField string
	}{
		{"workflow state control", "agent_but_typoed", "max_steps: 3", "control"},
		{"composition termination", "agent", "max_stepz: 3", "max_stepz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.arena.yaml")
			require.NoError(t, os.WriteFile(path, workflowSchemaDoc(tc.control, tc.termination), 0o600))

			_, err := LoadConfig(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "schema validation failed")
			assert.Contains(t, err.Error(), tc.wantField)
		})
	}
}
