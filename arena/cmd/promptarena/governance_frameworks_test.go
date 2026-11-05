package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGovernanceFrameworks(t *testing.T) {
	assert.Nil(t, governanceFrameworks(""))
	assert.Nil(t, governanceFrameworks("does-not-exist.arena.yaml"))

	fs := governanceFrameworks("../../../examples/adaptive-persona/config.arena.yaml")
	require.Len(t, fs, 2)
	assert.Equal(t, []string{"refund-policy"}, fs[0].IDs)
	assert.Contains(t, fs[1].IDs, "revealing-another-customers-order")
}
