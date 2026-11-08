package arenaconfig

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"
)

// Config fields the schema accepts but nothing reads. Each one stays loadable,
// so configs that set it keep working, and loading warns that it is ignored.
// Remove the field (and its warning) once the deprecation has had a release.

const ignoredFieldSuffix = "is ignored and will be removed"

const stateStoreTypeFile = "file"

// checkStateStore rejects state_store.type: file at load. The type is in
// PromptKit's StateStoreConfig, but promptarena builds only memory and redis
// stores, so such a config used to load and then fail at engine build.
func (c *Config) checkStateStore() error {
	if c.StateStore != nil && c.StateStore.Type == stateStoreTypeFile {
		return fmt.Errorf("state_store.type: file is not supported by promptarena; use memory or redis")
	}
	return nil
}

// ignoredFieldWarnings lists the set fields nothing reads, one warning each.
func (c *Config) ignoredFieldWarnings() []string {
	var out []string
	if len(c.Defaults.FailOn) > 0 {
		out = append(out, "defaults.fail_on "+ignoredFieldSuffix+
			": with --ci, any run error or failed assertion fails the run")
	}
	if c.StateStore != nil && c.StateStore.File != nil {
		out = append(out, "state_store.file "+ignoredFieldSuffix)
	}
	for _, id := range sortedKeys(c.LoadedScenarios) {
		if s := c.LoadedScenarios[id]; s != nil && s.ContextMetadata != nil && s.ContextMetadata.ProjectStage != "" {
			out = append(out, fmt.Sprintf("scenario %s: context_metadata.project_stage %s", id, ignoredFieldSuffix))
		}
	}
	for _, id := range sortedKeys(c.LoadedEvals) {
		ev := c.LoadedEvals[id]
		if ev == nil {
			continue
		}
		if ev.Mode != "" {
			out = append(out, fmt.Sprintf("eval %s: mode %s", id, ignoredFieldSuffix))
		}
		if ev.Speed != 0 {
			out = append(out, fmt.Sprintf("eval %s: speed %s", id, ignoredFieldSuffix))
		}
	}
	return out
}

// recordIgnoredFields logs each ignored-field warning and adds it to the
// config's validation warnings, which `promptarena validate` reports.
func (c *Config) recordIgnoredFields() {
	for _, w := range c.ignoredFieldWarnings() {
		logger.Warn(w)
		c.ValidationWarnings = append(c.ValidationWarnings, w)
	}
}

// warnProviderLabels warns when a provider file sets metadata.labels.
// PromptKit's provider loader does not keep them, so nothing can read them.
func warnProviderLabels(filename string) {
	data, err := os.ReadFile(filename) //nosec G304 -- operator-provided config file path
	if err != nil {
		return
	}
	var doc struct {
		Metadata struct {
			Labels map[string]string `yaml:"labels"`
		} `yaml:"metadata"`
	}
	if yaml.Unmarshal(data, &doc) != nil || len(doc.Metadata.Labels) == 0 {
		return
	}
	logger.Warn("provider metadata.labels "+ignoredFieldSuffix, "file", filename)
}

// runtimeFieldsArenaReads are the spec.runtime fields Arena consumes:
// evals (exec eval handlers), hooks and sandboxes (runtime hooks).
var runtimeFieldsArenaReads = map[string]bool{"evals": true, "hooks": true, "sandboxes": true}

// ignoredRuntimeFieldWarnings warns once per set spec.runtime field Arena does
// not read. They are warnings, not errors: a runtime config shared with the
// SDK may legitimately carry providers, state_store and the rest.
func (c *Config) ignoredRuntimeFieldWarnings() []string {
	if c.Runtime == nil {
		return nil
	}
	var out []string
	v := reflect.ValueOf(*c.Runtime)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name == "" || name == "-" || runtimeFieldsArenaReads[name] || isEmptyValue(v.Field(i)) {
			continue
		}
		out = append(out, fmt.Sprintf(
			"runtime.%s is ignored: Arena reads only runtime.evals, runtime.hooks and runtime.sandboxes", name))
	}
	return out
}

// isEmptyValue reports whether a field is unset: zero, or an empty slice or map.
func isEmptyValue(f reflect.Value) bool {
	switch f.Kind() { //nolint:exhaustive // other kinds fall back to IsZero
	case reflect.Slice, reflect.Map:
		return f.Len() == 0
	default:
		return f.IsZero()
	}
}
