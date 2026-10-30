package arenaconfig

import (
	"fmt"

	"github.com/AltairaLabs/PromptKit/runtime/v2/composition"
	"github.com/AltairaLabs/PromptKit/runtime/v2/workflow"
)

// ResolveDeprecatedFields carries values set through the deprecated untyped
// Workflow and Compositions fields over to WorkflowSpec and CompositionSpecs,
// when those are unset. Loading a config from YAML never sets the untyped
// fields; this is for Go callers that build a Config by hand. The engine calls
// it on construction, so callers need not.
func (c *Config) ResolveDeprecatedFields() error {
	if c.WorkflowSpec == nil && c.Workflow != nil {
		spec, err := workflow.ParseConfig(c.Workflow)
		if err != nil {
			return fmt.Errorf("parsing workflow config: %w", err)
		}
		c.WorkflowSpec = spec
	}
	if len(c.CompositionSpecs) == 0 && c.Compositions != nil {
		comps, err := composition.ParseConfig(c.Compositions)
		if err != nil {
			return fmt.Errorf("parsing compositions config: %w", err)
		}
		c.CompositionSpecs = comps
	}
	return nil
}
