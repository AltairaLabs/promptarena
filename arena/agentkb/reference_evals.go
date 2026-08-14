package agentkb

import (
	"bytes"
	"embed"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	_ "github.com/AltairaLabs/PromptKit/runtime/evals/handlers" // register default handlers
)

//go:embed evalmeta.yaml
var evalMetaFS embed.FS

type evalHandlerMeta struct {
	Level       string `yaml:"level"`
	Description string `yaml:"description"`
	Score       string `yaml:"score"`
}

type evalMeta struct {
	Handlers map[string]evalHandlerMeta `yaml:"handlers"`
}

func loadEvalMeta() (evalMeta, error) {
	b, err := evalMetaFS.ReadFile("evalmeta.yaml")
	if err != nil {
		return evalMeta{}, fmt.Errorf("read evalmeta.yaml: %w", err)
	}
	var m evalMeta
	if err := yaml.Unmarshal(b, &m); err != nil {
		return evalMeta{}, fmt.Errorf("parse evalmeta.yaml: %w", err)
	}
	return m, nil
}

// canonicalEvalTypes returns the registry's eval type names with alias names
// removed, sorted. These are the ids evalmeta.yaml must describe exactly.
func canonicalEvalTypes() []string {
	aliasNames := map[string]bool{}
	for _, pair := range evals.DefaultAliases() {
		aliasNames[pair[0]] = true
	}
	var out []string
	for _, t := range evals.NewEvalTypeRegistry().Types() {
		if !aliasNames[t] {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// GenerateEvalsReference renders the unified eval/assertion catalog markdown
// from the live handler registry + evalmeta.yaml + the alias table.
func GenerateEvalsReference() ([]byte, error) {
	meta, err := loadEvalMeta()
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("# Evals & Assertions\n\n")
	b.WriteString("Generated from the handler registry — do not edit by hand; run the reference generator.\n\n")
	b.WriteString("An **eval** emits a raw `Score` (0..1) or a boolean gate; a **threshold** turns ")
	b.WriteString("that score into pass/fail. Thresholds live on the **`type: assertion` wrapper, ")
	b.WriteString("inside its `params`** — never on the inner eval, which rejects them outright:\n\n")
	b.WriteString("```yaml\n- type: assertion\n  params:\n    eval_type: llm_judge      # inner eval (required)\n")
	b.WriteString("    eval_params: {...}        # params for the inner eval\n")
	b.WriteString("    min_score: 0.8            # optional; defaults to 1.0\n")
	b.WriteString("    max_score: 1.0            # optional\n```\n\n")
	b.WriteString("Two envelopes carry that entry, and they are **not** the same schema:\n\n")
	b.WriteString("- **Scenario turn assertions** (`spec.turns[].assertions[]`) accept only `type`, ")
	b.WriteString("`params`, `message`, `when`, `pass_threshold`. The schema is closed — `min_score` ")
	b.WriteString("or `eval` as a *sibling* of `type` is rejected. They belong under `params`. ")
	b.WriteString("Note `pass_threshold` is a different knob: the minimum pass **rate** across ")
	b.WriteString("trials, not a score threshold.\n")
	b.WriteString("- **Arena pack evals** (`spec.pack_evals[]`) accept `id`, `type`, `trigger`, ")
	b.WriteString("`params`, `metric`, and a `threshold: {min_score, max_score}` object.\n\n")
	b.WriteString("| id | level | score | description |\n|----|-------|-------|-------------|\n")

	for _, id := range canonicalEvalTypes() {
		m := meta.Handlers[id]
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", id, dash(m.Level), dash(m.Score), dash(m.Description))
	}

	b.WriteString("\n## Aliases\n\nLegacy names that map to a canonical handler:\n\n")
	b.WriteString("| alias | canonical |\n|-------|-----------|\n")
	for _, pair := range evals.DefaultAliases() {
		fmt.Fprintf(&b, "| `%s` | `%s` |\n", pair[0], pair[1])
	}
	return b.Bytes(), nil
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
