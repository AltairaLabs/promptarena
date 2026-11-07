package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

// env and headers values used to reach the server literally, so
// `GITHUB_TOKEN: ${GITHUB_TOKEN}` sent the string "${GITHUB_TOKEN}".
func TestNewRegistryFromConfig_ExpandsEnvReferences(t *testing.T) {
	t.Setenv("ARENA_MCP_TEST_TOKEN", "s3cret")
	cfg := &arenaconfig.Config{MCPServers: []config.MCPServerConfig{
		{
			Name:    "local",
			Command: "node",
			Env: map[string]string{
				"GITHUB_TOKEN": "${ARENA_MCP_TEST_TOKEN}",
				"MIXED":        "a-${ARENA_MCP_TEST_TOKEN}-b",
				"UNSET":        "${ARENA_MCP_TEST_UNSET_VAR}",
				"BARE":         "$ARENA_MCP_TEST_TOKEN and $5",
			},
		},
		{
			Name:    "remote",
			URL:     "https://mcp.example.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer ${ARENA_MCP_TEST_TOKEN}"},
		},
	}}

	registry, err := NewRegistryFromConfig(cfg)
	require.NoError(t, err)

	local, ok := registry.GetServerConfig("local")
	require.True(t, ok)
	assert.Equal(t, map[string]string{
		"GITHUB_TOKEN": "s3cret",
		"MIXED":        "a-s3cret-b",
		"UNSET":        "",
		"BARE":         "$ARENA_MCP_TEST_TOKEN and $5",
	}, local.Env)

	remote, ok := registry.GetServerConfig("remote")
	require.True(t, ok)
	assert.Equal(t, "Bearer s3cret", remote.Headers["Authorization"])

	assert.Equal(t, "${ARENA_MCP_TEST_TOKEN}", cfg.MCPServers[0].Env["GITHUB_TOKEN"],
		"the config itself is not modified")
}
