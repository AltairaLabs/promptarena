// Package mcp provides Model Context Protocol (MCP) configuration and registry setup for Arena.
//
// This package bridges Arena's configuration system with the runtime MCP registry,
// allowing test scenarios to use MCP servers for external tool integration.
//
// It handles:
//   - Loading MCP server configurations from Arena config files
//   - Creating and populating MCP registries
//   - Validating MCP server definitions
//
// Example usage:
//
//	registry, err := mcp.NewRegistryFromConfig(arenaConfig)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// Use registry with pipeline...
package mcp

import (
	"fmt"
	"os"
	"regexp"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/mcp"
)

// NewRegistryFromConfig creates a registry from a config object.
// It registers every static MCP server defined in the configuration — stdio
// (command) servers and HTTP (url) servers alike, with their headers and
// explicit transport.
//
// Source-backed entries (those with Source set) are not registered: they have
// no endpoint until their MCPSource opens one at a run, scenario or session
// boundary, which the arena engine does itself. Returns an empty registry if
// no static servers are configured.
func NewRegistryFromConfig(cfg *arenaconfig.Config) (*mcp.RegistryImpl, error) {
	registry := mcp.NewRegistry()

	for i := range cfg.MCPServers {
		serverCfg := &cfg.MCPServers[i]
		if serverCfg.Source != "" {
			continue
		}
		if err := registry.RegisterServer(ServerConfigFromConfig(serverCfg)); err != nil {
			return nil, fmt.Errorf("failed to register MCP server %s: %w", serverCfg.Name, err)
		}
	}

	return registry, nil
}

// ServerConfigFromConfig converts an arena MCP server entry into the runtime
// ServerConfig for a static server. Source, Scope and SourceArgs have no
// runtime counterpart: they tell the arena engine how to provision the
// endpoint, and the resulting URL and headers come from the MCPSource.
//
// `${VAR}` in env and headers values is replaced with the variable from
// Arena's environment (empty when unset), so a token can be passed as
// `GITHUB_TOKEN: ${GITHUB_TOKEN}` or `Authorization: "Bearer ${MCP_TOKEN}"`.
// Only the braced form expands; a bare `$` is passed through.
func ServerConfigFromConfig(c *config.MCPServerConfig) mcp.ServerConfig {
	sc := mcp.ServerConfig{
		Name:          c.Name,
		Command:       c.Command,
		Args:          c.Args,
		Env:           expandEnvValues(c.Env),
		WorkingDir:    c.WorkingDir,
		URL:           c.URL,
		Headers:       expandEnvValues(c.Headers),
		TransportName: mcp.Transport(c.Transport),
		TimeoutMs:     c.TimeoutMs,
	}
	if c.ToolFilter != nil {
		sc.ToolFilter = &mcp.ToolFilter{
			Allowlist: c.ToolFilter.Allowlist,
			Blocklist: c.ToolFilter.Blocklist,
		}
	}
	return sc
}

// envVarPattern matches a braced environment reference: ${NAME}.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnvValues returns a copy of m with ${VAR} references in each value
// replaced from the process environment. Nil stays nil.
func expandEnvValues(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = envVarPattern.ReplaceAllStringFunc(v, func(ref string) string {
			return os.Getenv(ref[2 : len(ref)-1]) // strip "${" and "}"
		})
	}
	return out
}
