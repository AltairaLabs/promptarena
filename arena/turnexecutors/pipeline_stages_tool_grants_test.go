package turnexecutors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The provider stage merges ToolGrants into allowedTools on every build and
// re-reads it after each tool round, so arena must hand it the run's live
// accessor rather than a snapshot. Without this the grants a skill declares
// never reach the provider's tools array.
func TestBuildProviderConfig_PassesSkillToolGrantsThrough(t *testing.T) {
	req := &TurnRequest{
		SkillToolGrants: func() []string { return []string{"issue_refund"} },
	}

	cfg := buildProviderConfig(req)

	require.NotNil(t, cfg.ToolGrants, "ToolGrants must be set when the run has a skills executor")
	assert.Equal(t, []string{"issue_refund"}, cfg.ToolGrants())
}

func TestBuildProviderConfig_NoGrantsAccessorLeavesToolGrantsNil(t *testing.T) {
	cfg := buildProviderConfig(&TurnRequest{})
	assert.Nil(t, cfg.ToolGrants, "runs without skills must not install a grants accessor")
}

// The accessor is live, not a snapshot: a skill activated mid-turn must be
// visible to the next round's rebuild.
func TestBuildProviderConfig_ToolGrantsAccessorIsLive(t *testing.T) {
	granted := []string{}
	req := &TurnRequest{
		SkillToolGrants: func() []string { return granted },
	}

	cfg := buildProviderConfig(req)
	require.Empty(t, cfg.ToolGrants())

	granted = []string{"issue_refund"}
	assert.Equal(t, []string{"issue_refund"}, cfg.ToolGrants())
}
