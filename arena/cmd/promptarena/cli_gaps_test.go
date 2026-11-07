package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/agentkb"
)

// agent-brief --help listed only AGENTS.md and SKILL.md although the command
// also writes every bundled reference/*.md file.
func TestAgentBriefHelp_ListsReferenceFiles(t *testing.T) {
	names, err := agentkb.ReferenceNames()
	require.NoError(t, err)
	require.NotEmpty(t, names)
	assert.Contains(t, agentBriefCmd.Long, "promptarena-authoring/reference/")
	for _, name := range names {
		assert.Contains(t, agentBriefCmd.Long, name)
	}
	assert.Contains(t, agentBriefCmd.Long, "AGENTS.md")
	assert.Contains(t, agentBriefCmd.Long, "SKILL.md")
}

// run --format completion offered html, which run --help does not document.
func TestCompleteFormats_MatchesRunHelp(t *testing.T) {
	got, _ := completeFormats(nil, nil, "")
	assert.NotContains(t, got, formatHTML)
	assert.ElementsMatch(t, []string{"json", "junit", "markdown", "csv", "objectives"}, got)
}

// Fixed in #421: init --provider completion offers the names the flag accepts.
func TestCompleteInitProviders_MatchesFlag(t *testing.T) {
	got, _ := completeInitProviders(nil, nil, "")
	assert.Equal(t, []string{"mock", "openai", "claude", "gemini"}, got)
}
