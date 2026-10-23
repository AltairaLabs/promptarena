package main

import (
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/configschema"
)

// init is called before any tests run in this package
// Disable schema validation for tests since schemas may not be published yet
func init() {
	// Disable schema validation when running tests
	if testing.Testing() {
		configschema.Disabled.Store(true)
	}
}
