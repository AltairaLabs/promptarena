package web

import (
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/configschema"
)

// init disables schema validation for tests so fixture configs that predate
// required schema fields (e.g. spec.description) still load cleanly.
func init() {
	if testing.Testing() {
		configschema.Disabled.Store(true)
	}
}
