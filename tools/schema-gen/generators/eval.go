package generators

import (
	"github.com/invopop/jsonschema"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

// GenerateEvalSchema generates the JSON Schema for Eval configuration
func GenerateEvalSchema() (interface{}, error) {
	return Generate(&SchemaConfig{
		Target:      &arenaconfig.EvalConfig{},
		Filename:    "eval.json",
		Title:       "PromptArena Eval Configuration",
		Description: "Eval configuration for replaying and validating saved conversations",
		Customize: func(schema *jsonschema.Schema) {
			addEvalExample(schema)
			applyKnownTypeSuggestions(schema)
		},
	})
}

// exampleJudgeMinScore is the pass threshold in the schema example.
const exampleJudgeMinScore = 0.8

func addEvalExample(schema *jsonschema.Schema) {
	schema.Examples = []interface{}{
		map[string]interface{}{
			"apiVersion": "promptkit.altairalabs.ai/v1alpha1",
			"kind":       "Eval",
			"metadata": map[string]interface{}{
				"name": "my-eval",
			},
			"spec": map[string]interface{}{
				"id":          "eval-saved-conversation",
				"description": "Evaluate saved customer support conversation",
				"recording": map[string]interface{}{
					"path": "recordings/session-2024-01-15.recording.json",
					"type": "session",
				},
				// Judges come from spec.judges in the arena config; `judge`
				// selects one by name, and an unknown name fails at load.
				"conversation_assertions": []interface{}{
					map[string]interface{}{
						"type":    "assertion",
						"message": "Customer issue was resolved",
						"params": map[string]interface{}{
							"eval_type": "llm_judge_session",
							"eval_params": map[string]interface{}{
								"judge":    "default",
								"criteria": "Was the customer issue resolved satisfactorily?",
							},
							"min_score": exampleJudgeMinScore,
						},
					},
				},
				"tags": []string{"customer-support", "evaluation"},
			},
		},
	}
}
