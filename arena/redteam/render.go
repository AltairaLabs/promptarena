package redteam

import (
	"bytes"
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
)

// The documents below mirror the parts of the scenario schema the generator
// writes, in the order a person reads them.

const (
	apiVersion   = "promptkit.altairalabs.ai/v1alpha1"
	scenarioKind = "Scenario"
	schemaLine   = "# yaml-language-server: $schema=https://promptkit.altairalabs.ai/schemas/latest/scenario.json\n"
	yamlIndent   = 2
)

type scenarioDoc struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Metadata   metadata     `yaml:"metadata"`
	Spec       scenarioSpec `yaml:"spec"`
}

type metadata struct {
	Name string `yaml:"name"`
}

type scenarioSpec struct {
	ID          string       `yaml:"id"`
	TaskType    string       `yaml:"task_type"`
	Description string       `yaml:"description"`
	Data        dataRef      `yaml:"data"`
	Trials      int          `yaml:"trials,omitempty"`
	TrialBudget *trialBudget `yaml:"trial_budget,omitempty"`
	Turns       []turn       `yaml:"turns"`
}

type dataRef struct {
	File string `yaml:"file"`
}

type trialBudget struct {
	StopOnAchieved bool    `yaml:"stop_on_achieved"`
	MaxCostUSD     float64 `yaml:"max_cost_usd,omitempty"`
}

type turn struct {
	Role       string      `yaml:"role"`
	Content    string      `yaml:"content,omitempty"`
	Persona    string      `yaml:"persona,omitempty"`
	Turns      int         `yaml:"turns,omitempty"`
	Transforms []transform `yaml:"transforms,omitempty"`
	Objective  *objective  `yaml:"objective,omitempty"`
	Assertions []check     `yaml:"assertions,omitempty"`
}

type transform struct {
	Converter string `yaml:"converter,omitempty"`
	Template  string `yaml:"template,omitempty"`
}

type objective struct {
	Description string   `yaml:"description"`
	Taxonomy    []string `yaml:"taxonomy,omitempty"`
	Expect      string   `yaml:"expect"`
	Baseline    bool     `yaml:"baseline,omitempty"`
	Success     []check  `yaml:"success"`
	Rewind      *rewind  `yaml:"rewind,omitempty"`
}

type rewind struct {
	Max  int     `yaml:"max"`
	When []check `yaml:"when"`
}

type check struct {
	Type    string         `yaml:"type"`
	Params  map[string]any `yaml:"params,omitempty"`
	Message string         `yaml:"message,omitempty"`
}

// renderScenario returns the scenario as YAML under a comment header.
func renderScenario(header string, doc *scenarioDoc) []byte {
	var buf bytes.Buffer
	buf.WriteString(schemaLine)
	buf.WriteString(header)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	// Strings, numbers and maps of them into a buffer: encoding cannot fail.
	_ = enc.Encode(doc)
	_ = enc.Close()
	return buf.Bytes()
}

// renderJSONL writes one JSON object per row, with keys in a fixed order.
func renderJSONL(columns []string, rows []map[string]string) []byte {
	var buf bytes.Buffer
	for _, row := range rows {
		buf.WriteByte('{')
		for i, c := range columns {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(jsonString(c))
			buf.WriteString(": ")
			buf.WriteString(jsonString(row[c]))
		}
		buf.WriteString("}\n")
	}
	return buf.Bytes()
}

// jsonString quotes s as a JSON string, leaving <, > and & as written.
func jsonString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // encoding a string into a buffer cannot fail
	return strings.TrimSuffix(b.String(), "\n")
}
