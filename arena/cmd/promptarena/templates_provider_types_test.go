package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/stretchr/testify/require"
)

// `promptarena init` wrote `type: anthropic` for the claude provider, which no
// provider factory answers to, so the generated kit failed on its first run.
// Every provider type a built-in template can render must be a registered
// provider type.
func TestBuiltinTemplateProviderTypesAreRegistered(t *testing.T) {
	files, err := filepath.Glob("../../templates/builtin/*/templates/provider.yaml.tmpl")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	registered := providers.RegisteredProviderTypes()
	typeLine := regexp.MustCompile(`(?m)^\s+type:\s+(\S+)\s*$`)
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // test reads repo files
		require.NoError(t, err)
		for _, m := range typeLine.FindAllStringSubmatch(string(data), -1) {
			require.True(t, slices.Contains(registered, m[1]),
				"%s: provider type %q is not registered (have %v)", f, m[1], registered)
		}
	}
}
