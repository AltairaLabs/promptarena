package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AltairaLabs/promptarena/v2/arena/mcpsource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/mcp"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

const graphOutputSchema = `{"type":"object","properties":{"entities":{"type":"array"}},"required":["entities"]}`

func TestMCPToolDescriptor_UsesServerOutputSchema(t *testing.T) {
	desc := mcpToolDescriptor("mcp__memory__read_graph", &mcp.Tool{
		Name: "read_graph", Description: "Read", InputSchema: json.RawMessage(`{"type":"object"}`),
		OutputSchema: json.RawMessage(graphOutputSchema),
	})
	assert.Equal(t, "mcp__memory__read_graph", desc.Name)
	assert.Equal(t, "mcp", desc.Mode)
	assert.JSONEq(t, graphOutputSchema, string(desc.OutputSchema))
}

func TestMCPToolDescriptor_NoServerSchemaMeansNoOutputSchema(t *testing.T) {
	desc := mcpToolDescriptor("Read", &mcp.Tool{
		Name: "Read", Description: "Read", InputSchema: json.RawMessage(`{"type":"object"}`),
	})
	assert.Empty(t, desc.OutputSchema)
}

// openStructuredTool serves one tool returning structuredContent through a
// source-backed scope and returns the tool registry it was discovered into.
func openStructuredTool(t *testing.T, tool mcp.Tool, resp mcp.ToolCallResponse) *tools.Registry {
	t.Helper()
	srv := newFakeSSEMCPServer(t, tool, resp)
	sourceName := registerTestSource(t, srv.URL)
	mcpReg := mcp.NewRegistry()
	t.Cleanup(func() { _ = mcpReg.Close() })
	toolReg := tools.NewRegistry()
	toolReg.RegisterExecutor(tools.NewMCPExecutor(mcpReg))
	scope := newMCPSourceScopeWithTools(mcpReg, toolReg)
	cfg := []config.MCPServerConfig{{Name: "sandbox", Source: sourceName, Scope: string(mcpsource.ScopeSession)}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, scope.OpenAll(ctx, mcpsource.ScopeSession, "s1", nil, nil, cfg))
	t.Cleanup(func() { _ = scope.CloseAll(mcpsource.ScopeSession, "s1") })
	return toolReg
}

// A server that returns structuredContent used to fail every call with
// "(root): content is required", because Arena gave every MCP tool a generic
// output schema describing the content envelope.
func TestMCPTool_StructuredContentWithoutDeclaredSchemaSucceeds(t *testing.T) {
	toolReg := openStructuredTool(t,
		mcp.Tool{Name: "read_graph", Description: "Read", InputSchema: json.RawMessage(`{"type":"object"}`)},
		mcp.ToolCallResponse{StructuredContent: json.RawMessage(`{"entities":[]}`)},
	)
	result, err := toolReg.Execute(context.Background(), "read_graph", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Empty(t, result.Error)
	assert.JSONEq(t, `{"entities":[]}`, string(result.Result))
}

func TestMCPTool_StructuredContentIsValidatedAgainstDeclaredSchema(t *testing.T) {
	tool := mcp.Tool{
		Name: "read_graph", Description: "Read", InputSchema: json.RawMessage(`{"type":"object"}`),
		OutputSchema: json.RawMessage(graphOutputSchema),
	}

	ok := openStructuredTool(t, tool, mcp.ToolCallResponse{StructuredContent: json.RawMessage(`{"entities":[]}`)})
	result, err := ok.Execute(context.Background(), "read_graph", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Empty(t, result.Error)

	bad := openStructuredTool(t, tool, mcp.ToolCallResponse{StructuredContent: json.RawMessage(`{"nodes":[]}`)})
	result, err = bad.Execute(context.Background(), "read_graph", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Contains(t, result.Error, "does not match its output schema")
}
