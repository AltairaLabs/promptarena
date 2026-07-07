package flow

import (
	"context"
	"testing"
)

func TestCheckPreflight_ConfigMissing(t *testing.T) {
	pf := CheckPreflight(context.Background(), Options{ProjectDir: t.TempDir(), ConfigPath: t.TempDir() + "/nope.yaml"})
	if pf.ConfigErr == nil {
		t.Fatal("expected ConfigErr for missing config")
	}
	if pf.Ready() {
		t.Fatal("Ready() must be false when config is missing")
	}
}
