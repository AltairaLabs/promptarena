package arenaconfig

import (
	"errors"
	"fmt"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

// checkMCPServers applies PromptKit's mcp_servers rules to the Arena config:
// a name, exactly one of command / url / source, a valid scope when source is
// set, and a transport that matches the endpoint. Without it an entry with
// both source and url passed (source silently won) and a source entry with no
// scope passed and was never opened.
func (c *Config) checkMCPServers() error {
	if len(c.MCPServers) == 0 {
		return nil
	}
	spec := config.RuntimeConfigSpec{MCPServers: c.MCPServers}
	err := spec.Validate()
	if err == nil {
		return nil
	}
	// config.ValidationError's own message is prefixed "logging config
	// validation error" whatever the field, so report its parts directly.
	var ve *config.ValidationError
	if errors.As(err, &ve) {
		return fmt.Errorf("invalid mcp_servers: %s: %s", ve.Field, ve.Message)
	}
	return fmt.Errorf("invalid mcp_servers: %w", err)
}
