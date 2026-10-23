package app

import (
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/configschema"
)

// init disables schema validation when running tests so that unit tests do not
// make network requests to fetch remote JSON schemas.
func init() {
	if testing.Testing() {
		configschema.Disabled.Store(true)
	}
}
