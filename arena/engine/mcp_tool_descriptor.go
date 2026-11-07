package engine

import (
	"github.com/AltairaLabs/PromptKit/runtime/v2/mcp"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

// mcpToolDescriptor builds the registry descriptor for a discovered MCP tool.
//
// The output schema is the one the server declared for the tool's
// structuredContent (MCP tools/list outputSchema), so the runtime validates a
// structured result against what the server promised. A tool that declares
// none gets none, and its results are not validated. Arena used to give every
// MCP tool a generic schema describing the content envelope; the runtime
// applies the output schema to structuredContent, so every structured result
// failed with "(root): content is required".
func mcpToolDescriptor(name string, t *mcp.Tool) *tools.ToolDescriptor {
	return &tools.ToolDescriptor{
		Name:         name,
		Description:  t.Description,
		InputSchema:  t.InputSchema,
		OutputSchema: t.OutputSchema,
		Mode:         "mcp",
	}
}
