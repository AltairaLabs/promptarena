package markdown

import (
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/configschema"
)

// init disables schema validation for tests since schemas may not be published yet.
func init() {
	if testing.Testing() {
		configschema.Disabled.Store(true)
	}
}
