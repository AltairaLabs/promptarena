package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// TestInspectPrinters pins the exact text each `packc inspect` / `packc compile`
// printer writes to stdout: which fields appear, their order, the sorting of
// map-backed sections, and that an absent section prints nothing at all.
func TestInspectPrinters(t *testing.T) {
	enabled := packspec.Ptr(true)
	disabled := packspec.Ptr(false)

	cases := []struct {
		name  string
		print func()
		want  string
	}{
		{
			name: "pack info with description",
			print: func() {
				printPackInfo(&prompt.Pack{Pack: packspec.Pack{
					ID: "test-pack", Name: "Test Pack", Version: "1.0.0", Description: "Test description",
				}})
			},
			want: "\n=== Pack Information ===\nPack: Test Pack\nID: test-pack\nVersion: 1.0.0\n" +
				"Description: Test description\n",
		},
		{
			name: "pack info without description",
			print: func() {
				printPackInfo(&prompt.Pack{Pack: packspec.Pack{ID: "p", Name: "P", Version: "2.0.0"}})
			},
			want: "\n=== Pack Information ===\nPack: P\nID: p\nVersion: 2.0.0\n",
		},
		{
			name: "template engine",
			print: func() {
				printTemplateEngine(&prompt.Pack{Pack: packspec.Pack{TemplateEngine: &prompt.TemplateEngineInfo{
					Version: "v1", Syntax: "handlebars", Features: []string{"loops", "conditionals"},
				}}})
			},
			want: "\nTemplate Engine: v1 (handlebars)\nFeatures: [loops conditionals]\n",
		},
		{name: "no template engine", print: func() { printTemplateEngine(&prompt.Pack{}) }},
		{
			name: "fragments are listed sorted",
			print: func() {
				printFragments(&prompt.Pack{Pack: packspec.Pack{
					Fragments: map[string]string{"outro": "{{outro}}", "intro": "{{intro}}"},
				}})
			},
			want: "\nShared Fragments (2): [intro outro]\n",
		},
		{name: "no fragments", print: func() { printFragments(&prompt.Pack{}) }},
		{
			name: "metadata",
			print: func() {
				printMetadata(&prompt.Pack{Pack: packspec.Pack{Metadata: &prompt.Metadata{
					Domain: "customer-support", Language: "en", Tags: []string{"support", "predict"},
				}}})
			},
			want: "\nPack Metadata:\n  Domain: customer-support\n  Language: en\n  Tags: [support predict]\n",
		},
		{name: "no metadata", print: func() { printMetadata(&prompt.Pack{}) }},
		{
			name: "compilation info",
			print: func() {
				printCompilationInfo(&prompt.Pack{Pack: packspec.Pack{Compilation: &prompt.CompilationInfo{
					CompiledWith: "packc-v0.1.0", CreatedAt: "2025-11-06T12:00:00Z", Schema: "v1",
				}}})
			},
			want: "\nCompilation: packc-v0.1.0, 2025-11-06T12:00:00Z, Schema: v1\n",
		},
		{name: "no compilation info", print: func() { printCompilationInfo(&prompt.Pack{}) }},
		{
			name: "variables split into required and optional",
			print: func() {
				printPromptVariables(&prompt.PackPrompt{Variables: []*prompt.Variable{
					{Name: "required1", Required: true},
					{Name: "optional1", Required: false},
					{Name: "required2", Required: true},
				}})
			},
			want: "  Variables: 2 required, 1 optional\n    Required: [required1 required2]\n",
		},
		{
			name:  "no variables still prints the count line",
			print: func() { printPromptVariables(&prompt.PackPrompt{}) },
			want:  "  Variables: 0 required, 0 optional\n",
		},
		{
			name:  "tools",
			print: func() { printPromptTools(&prompt.PackPrompt{Tools: []string{"search", "calculator"}}) },
			want:  "  Tools (2): [search calculator]\n",
		},
		{name: "no tools", print: func() { printPromptTools(&prompt.PackPrompt{}) }},
		{
			name: "validators: an unset enabled flag reads as enabled",
			print: func() {
				printPromptValidators(&prompt.PackPrompt{Validators: []*prompt.Validator{
					{Type: "length", Enabled: enabled},
					{Type: "sentiment", Enabled: disabled},
					{Type: "pii"},
				}})
			},
			want: "  Validators (3):\n    - length (enabled)\n    - sentiment (disabled)\n    - pii (enabled)\n",
		},
		{name: "no validators", print: func() { printPromptValidators(&prompt.PackPrompt{}) }},
		{
			name: "tested models scale success rate to a percentage",
			print: func() {
				printPromptTestedModels(&prompt.PackPrompt{TestedModels: []*prompt.ModelTestResultRef{
					{Provider: "openai", Model: "gpt-4", SuccessRate: packspec.Ptr(0.95), AvgTokens: packspec.Ptr(150.0)},
					{Provider: "claude", Model: "sonnet"},
				}})
			},
			want: "  Tested Models (2):\n    - openai/gpt-4: 95.0% success, avg 150 tokens\n" +
				"    - claude/sonnet: 0.0% success, avg 0 tokens\n",
		},
		{name: "no tested models", print: func() { printPromptTestedModels(&prompt.PackPrompt{}) }},
		{
			name: "prompt details print every section in order",
			print: func() {
				printPromptDetails("test", &prompt.PackPrompt{
					ID: "test-prompt", Name: "Test Prompt", Description: "A test prompt", Version: "1.0.0",
					Variables:  []*prompt.Variable{{Name: "var1", Required: true}},
					Tools:      []string{"tool1"},
					Validators: []*prompt.Validator{{Type: "length", Enabled: enabled}},
					TestedModels: []*prompt.ModelTestResultRef{
						{Provider: "openai", Model: "gpt-4", SuccessRate: packspec.Ptr(0.95), AvgTokens: packspec.Ptr(100.0)},
					},
				})
			},
			want: "\n[test]\n  Name: Test Prompt\n  Description: A test prompt\n  Version: 1.0.0\n" +
				"  Variables: 1 required, 0 optional\n    Required: [var1]\n" +
				"  Tools (1): [tool1]\n" +
				"  Validators (1):\n    - length (enabled)\n" +
				"  Tested Models (1):\n    - openai/gpt-4: 95.0% success, avg 100 tokens\n",
		},
		{
			name: "prompts are printed sorted by task type",
			print: func() {
				printPrompts(&prompt.Pack{Pack: packspec.Pack{Prompts: map[string]*prompt.PackPrompt{
					"task2": {ID: "task2", Name: "Task 2", Version: "1.0.0"},
					"task1": {ID: "task1", Name: "Task 1", Description: "First task", Version: "1.0.0"},
				}}})
			},
			want: "\n=== Prompts (2) ===\n" +
				"\n[task1]\n  Name: Task 1\n  Description: First task\n  Version: 1.0.0\n" +
				"  Variables: 0 required, 0 optional\n" +
				"\n[task2]\n  Name: Task 2\n  Version: 1.0.0\n  Variables: 0 required, 0 optional\n",
		},
		{
			name:  "no prompts still prints the header",
			print: func() { printPrompts(&prompt.Pack{}) },
			want:  "\n=== Prompts (0) ===\n",
		},
		{
			name: "workflow states are printed sorted",
			print: func() {
				printWorkflow(&prompt.Pack{Pack: packspec.Pack{Workflow: &prompt.WorkflowConfig{
					Version: 1,
					Entry:   "start",
					States: map[string]*prompt.WorkflowState{
						"start": {
							PromptTask: "greeting", Description: "Initial state",
							OnEvent:     map[string]string{"next": "end"},
							Persistence: "persistent", Orchestration: packspec.Ptr("internal"),
						},
						"end": {PromptTask: "farewell", OnEvent: map[string]string{}},
					},
				}}})
			},
			want: "\n=== Workflow (v1) ===\nEntry: start\nStates (2):\n" +
				"  [end] prompt_task=farewell\n" +
				"  [start] prompt_task=greeting\n    Description: Initial state\n    on next → end\n" +
				"    Persistence: persistent\n    Orchestration: internal\n",
		},
		{name: "no workflow", print: func() { printWorkflow(&prompt.Pack{}) }},
		{
			name: "workflow state events are printed sorted",
			print: func() {
				printWorkflowState("start", &prompt.WorkflowState{
					PromptTask: "greeting", Description: "Initial state",
					OnEvent:     map[string]string{"Error": "fallback", "Done": "end"},
					Persistence: "persistent", Orchestration: packspec.Ptr("internal"),
				})
			},
			want: "  [start] prompt_task=greeting\n    Description: Initial state\n" +
				"    on Done → end\n    on Error → fallback\n" +
				"    Persistence: persistent\n    Orchestration: internal\n",
		},
		{
			name: "minimal workflow state omits empty orchestration",
			print: func() {
				printWorkflowState("end", &prompt.WorkflowState{PromptTask: "farewell", Orchestration: packspec.Ptr("")})
			},
			want: "  [end] prompt_task=farewell\n",
		},
		{
			name: "agents are printed sorted with only the fields they set",
			print: func() {
				printAgents(&prompt.Pack{Pack: packspec.Pack{Agents: &prompt.AgentsConfig{
					Entry: "triage",
					Members: map[string]*prompt.AgentDef{
						"triage": {
							Description: "Triage agent", Tags: []string{"router"},
							InputModes: []string{"text/plain"}, OutputModes: []string{"text/plain"},
						},
						"billing": {Description: "Billing agent", Tags: []string{"billing"}},
					},
				}}})
			},
			want: "\n=== Agents ===\nEntry: triage\nMembers (2):\n" +
				"  [billing]\n    Description: Billing agent\n    Tags: [billing]\n" +
				"  [triage]\n    Description: Triage agent\n    Tags: [router]\n" +
				"    Input modes: [text/plain]\n    Output modes: [text/plain]\n",
		},
		{name: "no agents", print: func() { printAgents(&prompt.Pack{}) }},
		{
			name: "minimal pack summary",
			print: func() {
				printPackSummary(&prompt.Pack{Pack: packspec.Pack{
					Prompts: map[string]*prompt.PackPrompt{"test": {ID: "test"}},
				}}, "test.pack.json")
			},
			want: "✓ Pack compiled successfully: test.pack.json\n  Contains 1 prompts: [test]\n",
		},
		{
			name: "full pack summary",
			print: func() {
				printPackSummary(&prompt.Pack{Pack: packspec.Pack{
					Prompts: map[string]*prompt.PackPrompt{"test": {ID: "test"}},
					Tools: map[string]*prompt.PackTool{
						"search":     {Name: "search", Description: "Search"},
						"calculator": {Name: "calculator", Description: "Calc"},
					},
					Evals: []*evals.EvalDef{{ID: "eval1", Type: "contains"}},
					Workflow: &prompt.WorkflowConfig{
						Version: 1, Entry: "start",
						States: map[string]*prompt.WorkflowState{"start": {PromptTask: "test"}},
					},
					Agents: &prompt.AgentsConfig{
						Entry:   "agent1",
						Members: map[string]*prompt.AgentDef{"agent1": {Description: "Agent 1"}},
					},
				}}, "full.pack.json")
			},
			want: "✓ Pack compiled successfully: full.pack.json\n  Contains 1 prompts: [test]\n" +
				"  Contains 2 tools: [calculator search]\n  Pack-level evals: 1\n" +
				"  Workflow: 1 states (entry: start)\n  Agents: 1 members (entry: agent1)\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runWithArgs(t, os.Args, tc.print)
			assert.Equal(t, tc.want, out)
		})
	}
}
