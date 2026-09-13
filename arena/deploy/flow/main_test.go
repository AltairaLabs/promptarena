package flow

import (
	"fmt"
	"os"
	"testing"
)

// TestMain stops the login flow from launching a real browser.
//
// LoginHooks.OpenBrowser falls back to browser.OpenURL when a caller leaves it
// nil, which is right in production and wrong in a test: a case that reaches
// the authorize step without stubbing the hook opens an actual browser window
// on whoever is running the suite. That happened — TestRunLoginFlow_Timeout
// passed LoginHooks{} and every `go test ./...` popped Chrome at the fake
// authorize URL.
//
// Stubbing it here rather than only fixing that one test means a future test
// cannot reintroduce the problem by forgetting. Tests that care about the
// browser call still set LoginHooks.OpenBrowser explicitly and are unaffected.
func TestMain(m *testing.M) {
	defaultOpenBrowser = func(url string) error {
		return fmt.Errorf("test attempted to open a browser at %s: "+
			"set LoginHooks.OpenBrowser in the test instead", url)
	}
	os.Exit(m.Run())
}
