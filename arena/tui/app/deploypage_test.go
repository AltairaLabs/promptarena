package app

import "testing"

// TestNewDeployPage_TitleAndInitialState verifies that NewDeployPage builds a
// *DeployPage titled "Deploy" that starts in the preflight state.
func TestNewDeployPage_TitleAndInitialState(t *testing.T) {
	ctx := newMenuTestCtx(t)
	p := NewDeployPage(ctx)
	if p.Title() != "Deploy" {
		t.Fatalf("Title() = %q, want Deploy", p.Title())
	}
	dp, ok := p.(*DeployPage)
	if !ok {
		t.Fatal("NewDeployPage did not return *DeployPage")
	}
	if dp.state != deployStatePreflight {
		t.Fatalf("initial state = %d, want preflight", dp.state)
	}
}
