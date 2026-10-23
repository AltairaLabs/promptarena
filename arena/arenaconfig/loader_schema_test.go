package arenaconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PromptKit's loaders check a file only against the Go types, so these loads
// must run promptarena's schema too. Each document below decodes into its Go
// type but breaks a rule only the schema states.

func writeLoaderSchemaFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadScenario_ChecksTheSchema(t *testing.T) {
	path := writeLoaderSchemaFile(t, `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: s
spec:
  id: s
  task_type: support
  turns:
    - role: user
      content: hi
`)
	_, err := LoadScenario(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scenario configuration does not match schema")
	assert.Contains(t, err.Error(), "description")
}

func TestLoadEval_ChecksTheSchema(t *testing.T) {
	path := writeLoaderSchemaFile(t, `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Eval
metadata:
  name: e
spec:
  id: e
  recording:
    path: rec.json
    type: session
`)
	_, err := LoadEval(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "eval configuration does not match schema")
	assert.Contains(t, err.Error(), "description")
}

func TestLoadProvider_ChecksTheSchema(t *testing.T) {
	path := writeLoaderSchemaFile(t, `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: p
spec:
  id: p
  type: openai
  model: gpt-4o
  defaults:
    temperature: 0.2
`)
	_, err := loadProvider(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider configuration does not match schema")
	assert.Contains(t, err.Error(), "max_tokens")
}

func TestLoadProvider_AcceptsMessageMetadataTemplate(t *testing.T) {
	path := writeLoaderSchemaFile(t, `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: p
spec:
  id: p
  type: openai
  model: gpt-4o
  message_metadata_template: "[{{.order_id}}] {{.text}}"
`)
	p, err := loadProvider(path)
	require.NoError(t, err)
	assert.Equal(t, "[{{.order_id}}] {{.text}}", p.MessageMetadataTemplate)
}

func TestCheckFile_MissingFile(t *testing.T) {
	_, err := LoadScenario(filepath.Join(t.TempDir(), "missing.yaml"))
	assert.ErrorContains(t, err, "failed to read")
}
