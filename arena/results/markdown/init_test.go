package markdown

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

// init disables schema validation for tests since schemas may not be published yet.
func init() {
	if testing.Testing() {
		config.SchemaValidationDisabled.Store(true)
	}
}
