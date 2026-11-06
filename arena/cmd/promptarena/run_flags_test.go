package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --temperature, --seed, --selfplay and --roles were listed in help and never
// read. They stay accepted, so scripts still run, but are hidden and warn.
func TestIgnoredRunFlagsAreDeprecatedAndHidden(t *testing.T) {
	for _, name := range []string{"temperature", "seed", "selfplay", "roles"} {
		f := runCmd.Flags().Lookup(name)
		require.NotNil(t, f, name)
		assert.True(t, f.Hidden, "%s is hidden", name)
		assert.Contains(t, f.Deprecated, "no effect", name)
	}
	assert.NotContains(t, runCmd.Flags().FlagUsages(), "--seed")
}

// --simple's help said it aliased --ci; the two are deliberately different
// modes, and only --ci fails the process on a failed run.
func TestSimpleAndCIHelpDescribeTheirExitStatus(t *testing.T) {
	assert.NotContains(t, runCmd.Flags().Lookup("simple").Usage, "alias")
	assert.True(t, strings.Contains(runCmd.Flags().Lookup("ci").Usage, "non-zero"))
}
