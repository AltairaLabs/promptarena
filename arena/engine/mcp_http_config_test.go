package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/mcpsource"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/mcp"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

// TestBuildMCPRegistry_HTTPServerSendsHeadersEndToEnd drives a url-configured
// MCP server from arena config through to a live HTTP MCP server: the explicit
// transport must be kept and the configured headers must reach the wire.
func TestBuildMCPRegistry_HTTPServerSendsHeadersEndToEnd(t *testing.T) {
	srv := newFakeSSEMCPServer(t,
		mcp.Tool{Name: "Read", InputSchema: json.RawMessage(`{"type":"object"}`)},
		mcp.ToolCallResponse{Content: []mcp.Content{{Type: "text", Text: "ok"}}},
	)

	cfg := &arenaconfig.Config{
		MCPServers: []config.MCPServerConfig{{
			Name:      "remote",
			URL:       srv.URL,
			Headers:   map[string]string{"Authorization": "Bearer e2e-token"},
			Transport: "sse",
			TimeoutMs: 2000,
		}},
	}

	registry, err := buildMCPRegistry(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = registry.Close() })

	sc, ok := registry.GetServerConfig("remote")
	require.True(t, ok)
	assert.Equal(t, mcp.TransportSSE, sc.TransportName, "explicit transport must be carried from config")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	all, err := registry.ListAllTools(ctx)
	require.NoError(t, err)
	require.Len(t, all["remote"], 1)
	assert.Equal(t, "Read", all["remote"][0].Name)

	headers := srv.sseRequestHeaders()
	require.NotEmpty(t, headers, "the MCP client should have connected to /sse")
	assert.Equal(t, "Bearer e2e-token", headers[0].Get("Authorization"))
}

// TestMCPSourceScope_PassesExplicitTransport checks that a source-backed
// entry's transport reaches the runtime config registered after Open.
func TestMCPSourceScope_PassesExplicitTransport(t *testing.T) {
	srv := newFakeSSEMCPServer(t,
		mcp.Tool{Name: "Read", InputSchema: json.RawMessage(`{"type":"object"}`)},
		mcp.ToolCallResponse{Content: []mcp.Content{{Type: "text", Text: "ok"}}},
	)
	sourceName := registerTestSource(t, srv.URL)

	mcpReg := mcp.NewRegistry()
	t.Cleanup(func() { _ = mcpReg.Close() })
	toolReg := tools.NewRegistry()
	toolReg.RegisterExecutor(tools.NewMCPExecutor(mcpReg))
	scope := newMCPSourceScopeWithTools(mcpReg, toolReg)

	cfg := []config.MCPServerConfig{{
		Name:      "sandbox",
		Source:    sourceName,
		Scope:     string(mcpsource.ScopeSession),
		Transport: "sse",
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, scope.OpenAll(ctx, mcpsource.ScopeSession, "session-1", nil, nil, cfg))
	t.Cleanup(func() { _ = scope.CloseAll(mcpsource.ScopeSession, "session-1") })

	sc, ok := mcpReg.GetServerConfig("sandbox")
	require.True(t, ok)
	assert.Equal(t, mcp.TransportSSE, sc.TransportName)
	assert.Equal(t, srv.URL, sc.URL)
}
