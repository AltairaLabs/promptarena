package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

// TestNewRegistryFromConfig_RegistersEachServer checks which servers end up in
// the registry. The fields each one carries are pinned by
// TestNewRegistryFromConfig_CarriesEveryServerField.
func TestNewRegistryFromConfig_RegistersEachServer(t *testing.T) {
	tests := []struct {
		name    string
		servers []config.MCPServerConfig
		want    []string
	}{
		{name: "nil servers", servers: nil, want: []string{}},
		{name: "empty servers", servers: []config.MCPServerConfig{}, want: []string{}},
		{
			name:    "single server with nil env",
			servers: []config.MCPServerConfig{{Name: "no-env", Command: "test-cmd", Env: nil}},
			want:    []string{"no-env"},
		},
		{
			name: "multiple servers",
			servers: []config.MCPServerConfig{
				{Name: "memory-server", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-memory"}},
				{Name: "filesystem-server", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem"}},
				{Name: "remote", URL: "https://mcp.example.com/mcp"},
			},
			want: []string{"memory-server", "filesystem-server", "remote"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := NewRegistryFromConfig(&arenaconfig.Config{MCPServers: tt.servers})
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, registry.ListServers())
		})
	}
}

func TestNewRegistryFromConfig_RejectsUnregistrableServers(t *testing.T) {
	tests := []struct {
		name    string
		servers []config.MCPServerConfig
		want    string
	}{
		{
			name:    "duplicate name",
			servers: []config.MCPServerConfig{{Name: "duplicate", Command: "cmd1"}, {Name: "duplicate", Command: "cmd2"}},
			want:    "failed to register MCP server duplicate: server duplicate already registered",
		},
		{
			name:    "empty name",
			servers: []config.MCPServerConfig{{Name: "", Command: "test-cmd"}},
			want:    "server name cannot be empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := NewRegistryFromConfig(&arenaconfig.Config{MCPServers: tt.servers})
			assert.ErrorContains(t, err, tt.want)
			assert.Nil(t, registry)
		})
	}
}
