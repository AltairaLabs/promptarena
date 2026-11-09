package arenaconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadMCPConfig(t *testing.T, servers string) error {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.arena.yaml")
	body := "apiVersion: promptkit.altairalabs.ai/v1alpha1\nkind: Arena\nmetadata:\n  name: mcp\nspec:\n  providers: []\n  defaults: {}\n  mcp_servers:\n" + servers
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	_, err := LoadConfig(path)
	return err
}

func TestLoadConfig_MCPServers(t *testing.T) {
	cases := []struct {
		name    string
		servers string
		wantErr string
	}{
		{
			name:    "source and url both set",
			servers: "    - name: sb\n      source: docker\n      scope: session\n      url: http://localhost:1/sse\n",
			wantErr: "exactly one of",
		},
		{
			name:    "source and command both set",
			servers: "    - name: sb\n      source: docker\n      scope: session\n      command: npx\n",
			wantErr: "exactly one of",
		},
		{
			name:    "source without scope",
			servers: "    - name: sb\n      source: docker\n",
			wantErr: "scope is required when source is set",
		},
		{
			// NewRegistryFromConfig registers an entry with no command or url as-is, so
			// the loader is where an entry with no endpoint has to be refused.
			name:    "no source, url or command",
			servers: "    - name: empty\n",
			wantErr: "exactly one of",
		},
		{name: "valid source", servers: "    - name: sb\n      source: docker\n      scope: session\n"},
		{name: "valid stdio", servers: "    - name: fs\n      command: npx\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := loadMCPConfig(t, tc.servers)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid mcp_servers")
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.NotContains(t, err.Error(), "logging config")
		})
	}
}
