// Package configschema validates PromptArena config files against the JSON
// schemas this repo generates and embeds (arena/agentkb). The schemas belong to
// promptarena: PromptKit checks config only against its Go types, so every
// schema check promptarena wants happens here.
package configschema

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	promptschema "github.com/AltairaLabs/PromptKit/runtime/v2/prompt/schema"

	"github.com/AltairaLabs/promptarena/v2/arena/agentkb"
)

// Disabled skips validation. Tests that load fixtures the schemas do not
// describe set it.
var Disabled atomic.Bool

const (
	maxValidValuesShown       = 8
	keywordAdditionalProperty = "additional_property_not_allowed"
	keywordEnum               = "enum"
)

// compiled holds one schema per config type, compiled on first use. The schemas
// are embedded, so they never change while the process runs.
type compiled struct {
	schema *gojsonschema.Schema
	raw    map[string]any
}

var cache sync.Map // config.ConfigType -> *compiled

// Validate checks YAML data against the embedded schema for configType and
// returns every violation, with valid alternatives and nearest matches for
// unknown keys and enum values.
func Validate(yamlData []byte, configType config.ConfigType) (*config.SchemaValidationResult, error) {
	if Disabled.Load() {
		return &config.SchemaValidationResult{Valid: true, Errors: []config.SchemaValidationError{}}, nil
	}
	jsonData, err := yamlToJSON(yamlData)
	if err != nil {
		return nil, err
	}
	c, err := schemaFor(configType)
	if err != nil {
		return nil, err
	}
	result, err := c.schema.Validate(gojsonschema.NewBytesLoader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("schema validation failed: %w", err)
	}
	return convert(result, c.raw), nil
}

// Check validates YAML data against the schema for configType and returns one
// error listing every violation, or nil when the data is valid.
func Check(configType config.ConfigType, yamlData []byte) error {
	result, err := Validate(yamlData, configType)
	if err != nil {
		return err
	}
	if result.Valid {
		return nil
	}
	msgs := make([]string, 0, len(result.Errors))
	for _, e := range result.Errors {
		msgs = append(msgs, "  - "+e.Error())
	}
	return fmt.Errorf("%s configuration does not match schema:\n%s", configType, strings.Join(msgs, "\n"))
}

// kinds maps a manifest's kind to its config type.
var kinds = map[string]config.ConfigType{
	"Arena":        config.ConfigTypeArena,
	"Scenario":     config.ConfigTypeScenario,
	"Eval":         config.ConfigTypeEval,
	"Provider":     config.ConfigTypeProvider,
	"PromptConfig": config.ConfigTypePromptConfig,
	"Tool":         config.ConfigTypeTool,
	"Persona":      config.ConfigTypePersona,
}

// DetectType reads a manifest's kind and returns its config type.
func DetectType(yamlData []byte) (config.ConfigType, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(yamlData, &doc); err != nil {
		return "", fmt.Errorf("failed to parse YAML: %w", err)
	}
	if kind, ok := doc["kind"].(string); ok {
		if ct, ok := kinds[kind]; ok {
			return ct, nil
		}
	}
	return "", fmt.Errorf("unable to detect configuration type: missing or unknown 'kind' field")
}

func schemaFor(configType config.ConfigType) (*compiled, error) {
	if c, ok := cache.Load(configType); ok {
		return c.(*compiled), nil
	}
	data, err := fs.ReadFile(agentkb.SchemaFS(), string(configType)+".json")
	if err != nil {
		return nil, fmt.Errorf("no schema for config type %q: %w", configType, err)
	}
	schema, err := gojsonschema.NewSchema(gojsonschema.NewBytesLoader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to load schema %s: %w", configType, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse schema %s: %w", configType, err)
	}
	c, _ := cache.LoadOrStore(configType, &compiled{schema: schema, raw: raw})
	return c.(*compiled), nil
}

func yamlToJSON(yamlData []byte) ([]byte, error) {
	var data any
	if err := yaml.Unmarshal(yamlData, &data); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON: %w", err)
	}
	return jsonData, nil
}

// convert turns a gojsonschema result into a config.SchemaValidationResult,
// adding the valid keys or enum values to errors about either.
func convert(result *gojsonschema.Result, raw map[string]any) *config.SchemaValidationResult {
	shared := promptschema.ConvertResult(result)
	out := &config.SchemaValidationResult{
		Valid:  shared.Valid,
		Errors: make([]config.SchemaValidationError, 0, len(shared.Errors)),
	}
	for _, e := range shared.Errors {
		converted := config.SchemaValidationError{
			Field:       e.Field,
			Description: e.Description,
			Value:       e.Value,
			Keyword:     e.Keyword,
			ValidValues: e.ValidValues,
			Suggestions: e.Suggestions,
		}
		switch e.Keyword {
		case keywordAdditionalProperty:
			enrichAdditionalProperty(&converted, raw)
		case keywordEnum:
			enrichEnum(&converted, raw)
		}
		out.Errors = append(out.Errors, converted)
	}
	return out
}

func enrichAdditionalProperty(e *config.SchemaValidationError, raw map[string]any) {
	const prefix = "Additional property "
	const suffix = " is not allowed"
	valid := promptschema.LookupProperties(raw, e.Field)
	if len(valid) == 0 {
		return
	}
	if strings.HasPrefix(e.Description, prefix) && strings.HasSuffix(e.Description, suffix) {
		offending := strings.TrimSuffix(strings.TrimPrefix(e.Description, prefix), suffix)
		e.Suggestions = promptschema.NearestMatches(offending, valid)
	}
	e.ValidValues = truncate(valid, maxValidValuesShown)
}

func enrichEnum(e *config.SchemaValidationError, raw map[string]any) {
	allowed := promptschema.LookupEnumValues(raw, e.Field)
	if len(allowed) == 0 {
		return
	}
	e.ValidValues = allowed
	if s, ok := e.Value.(string); ok {
		e.Suggestions = promptschema.NearestMatches(s, allowed)
	}
}

// truncate caps the listed valid values, sorted, with a "+N more" entry.
func truncate(values []string, limit int) []string {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	if len(sorted) <= limit {
		return sorted
	}
	return append(sorted[:limit:limit], fmt.Sprintf("+%d more", len(sorted)-limit))
}
