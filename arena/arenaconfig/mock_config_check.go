package arenaconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
)

const (
	providerTypeMock = "mock"
	mockConfigKey    = "mock_config"
)

// Keys the runtime's mock repository reads from a structured turn (or step)
// response, a tool call and a content part (providers/mock/mock_repository.go:
// parseStructuredResponse, parseToolCalls, parsePart). Anything else is dropped.
var (
	mockTurnKeys = keySet("type", "content", "text", "response", "parts", "tool_calls",
		"audio_file", "audio_sample_rate", "audio_mime_type")
	mockToolCallKeys = keySet("name", "arguments")
	mockPartKeys     = keySet("type", "text", "image_url", "audio_url", "video_url", "document_url", "metadata")
)

func keySet(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// validateMockConfigs checks every mock provider's mock_config file at load.
// The runtime parses it non-strictly, so a misspelled key (`default_response`
// for `defaultResponse`, a typo in `scenarios:`) left the provider answering
// with the generic "Mock response for provider ..." text and no warning.
func (c *Config) validateMockConfigs() error {
	for _, id := range sortedKeys(c.LoadedProviders) {
		p := c.LoadedProviders[id]
		if p == nil || p.Type != providerTypeMock {
			continue
		}
		path, _ := p.AdditionalConfig[mockConfigKey].(string)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) && c.ConfigDir != "" {
			path = filepath.Join(c.ConfigDir, path)
		}
		if err := CheckMockConfigFile(path); err != nil {
			return fmt.Errorf("provider %s: %w", id, err)
		}
	}
	return nil
}

// CheckMockConfigFile reports unknown keys in a mock response file: decoded
// strictly against the runtime's mock.Config, then each structured turn, step,
// tool call and content part checked against the keys the runtime reads. A
// file that cannot be read is left to the provider, which reports it.
func CheckMockConfigFile(path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the operator's own mock config file
	if err != nil {
		return nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg mock.Config
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		// yaml.v3 puts one problem per line; keep them on the one error line.
		msg := strings.ReplaceAll(strings.ReplaceAll(err.Error(), ":\n", ": "), "\n", "; ")
		msg = strings.Join(strings.Fields(msg), " ")
		return fmt.Errorf("mock config %s: %s (keys are defaultResponse, scenarios, selfplay; "+
			"a scenario takes defaultResponse, turns, steps)", path, msg)
	}
	var problems []string
	for _, section := range []struct {
		name    string
		entries map[string]mock.ScenarioConfig
	}{{"scenarios", cfg.Scenarios}, {"selfplay", cfg.Selfplay}} {
		for _, key := range sortedKeys(section.entries) {
			sc := section.entries[key]
			for _, turn := range sortedIntKeys(sc.Turns) {
				where := fmt.Sprintf("%s.%s.turns.%d", section.name, key, turn)
				problems = append(problems, checkMockResponse(where, sc.Turns[turn])...)
			}
			for _, step := range sortedKeys(sc.Steps) {
				where := fmt.Sprintf("%s.%s.steps.%s", section.name, key, step)
				problems = append(problems, checkMockResponse(where, sc.Steps[step])...)
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("mock config %s: %s", path, strings.Join(problems, "; "))
	}
	return nil
}

// checkMockResponse checks one turn or step response. A plain string is a text
// response; a map must use the keys the runtime reads.
func checkMockResponse(where string, response any) []string {
	m, ok := response.(map[string]any)
	if !ok {
		return nil
	}
	problems := unknownKeys(where, m, mockTurnKeys)
	if calls, ok := m["tool_calls"].([]any); ok {
		for i, call := range calls {
			if cm, ok := call.(map[string]any); ok {
				problems = append(problems, unknownKeys(fmt.Sprintf("%s.tool_calls[%d]", where, i), cm, mockToolCallKeys)...)
			}
		}
	}
	if parts, ok := m["parts"].([]any); ok {
		for i, part := range parts {
			if pm, ok := part.(map[string]any); ok {
				problems = append(problems, unknownKeys(fmt.Sprintf("%s.parts[%d]", where, i), pm, mockPartKeys)...)
			}
		}
	}
	return problems
}

func unknownKeys(where string, m map[string]any, allowed map[string]bool) []string {
	var out []string
	for _, k := range sortedKeys(m) {
		if !allowed[k] {
			out = append(out, fmt.Sprintf("%s: unknown key %q (valid: %s)", where, k, strings.Join(sortedKeys(allowed), ", ")))
		}
	}
	return out
}

func sortedIntKeys[V any](m map[int]V) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
