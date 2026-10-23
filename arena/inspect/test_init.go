package inspect

import (
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/configschema"
)

func init() {
	if testing.Testing() {
		configschema.Disabled.Store(true)
	}
}
