package inspect

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

func init() {
	if testing.Testing() {
		config.SchemaValidationDisabled.Store(true)
	}
}
