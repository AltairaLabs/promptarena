package configschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

const testProvider = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: p
spec:
  id: p
  type: openai
  model: gpt-4o
`

// Validation uses only the schemas embedded in the binary: with the hosted
// copy unreachable and no schemas/ directory nearby, it still validates.
func TestValidate_UsesTheEmbeddedSchemas(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "remote")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1") // any fetch of the hosted copy fails

	result, err := Validate([]byte(testProvider), config.ConfigTypeProvider)
	require.NoError(t, err)
	assert.True(t, result.Valid, "errors: %+v", result.Errors)
}

// promptarena's schema carries every field PromptKit's provider type declares,
// message_metadata_template among them (PromptKit#2249).
func TestValidate_AcceptsMessageMetadataTemplate(t *testing.T) {
	doc := testProvider + "  message_metadata_template: \"[{{.order_id}}] {{.text}}\"\n"
	assert.NoError(t, Check(config.ConfigTypeProvider, []byte(doc)))
}

func TestCheck_UnknownKeyNamesTypeKeyAndNearestMatch(t *testing.T) {
	doc := testProvider + "  modle: gpt-4o\n"

	err := Check(config.ConfigTypeProvider, []byte(doc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider configuration does not match schema")
	assert.Contains(t, err.Error(), "modle")

	result, err := Validate([]byte(doc), config.ConfigTypeProvider)
	require.NoError(t, err)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, keywordAdditionalProperty, result.Errors[0].Keyword)
	assert.Contains(t, result.Errors[0].Suggestions, "model")
	assert.NotEmpty(t, result.Errors[0].ValidValues)
}

func TestValidate_EnumErrorListsAllowedValues(t *testing.T) {
	doc := testProvider + "  role: ttts\n"

	result, err := Validate([]byte(doc), config.ConfigTypeProvider)
	require.NoError(t, err)
	require.False(t, result.Valid)
	var enumErr *config.SchemaValidationError
	for i := range result.Errors {
		if result.Errors[i].Keyword == keywordEnum {
			enumErr = &result.Errors[i]
		}
	}
	require.NotNil(t, enumErr, "errors: %+v", result.Errors)
	assert.Contains(t, enumErr.ValidValues, "tts")
	assert.Contains(t, enumErr.Suggestions, "tts")
}

func TestValidate_Errors(t *testing.T) {
	_, err := Validate([]byte("a: [\n"), config.ConfigTypeProvider)
	assert.ErrorContains(t, err, "failed to parse YAML")

	_, err = Validate([]byte(testProvider), config.ConfigType("nosuch"))
	assert.ErrorContains(t, err, `no schema for config type "nosuch"`)
}

func TestDisabled_SkipsValidation(t *testing.T) {
	Disabled.Store(true)
	t.Cleanup(func() { Disabled.Store(false) })

	assert.NoError(t, Check(config.ConfigTypeProvider, []byte(testProvider+"  modle: x\n")))
}

func TestDetectType(t *testing.T) {
	for kind, want := range kinds {
		got, err := DetectType([]byte("kind: " + kind + "\n"))
		require.NoError(t, err, kind)
		assert.Equal(t, want, got, kind)
	}
	for _, doc := range []string{"metadata: {}\n", "kind: Unknown\n"} {
		_, err := DetectType([]byte(doc))
		assert.ErrorContains(t, err, "missing or unknown 'kind'", doc)
	}
	_, err := DetectType([]byte("a: [\n"))
	assert.ErrorContains(t, err, "failed to parse YAML")
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, truncate([]string{"b", "a"}, 3))
	assert.Equal(t, []string{"a", "b", "+2 more"}, truncate([]string{"d", "c", "b", "a"}, 2))
}
