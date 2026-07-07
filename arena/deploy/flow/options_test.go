package flow

import (
	"os"
	"testing"
)

func TestOptions_ConfigDefaultsToArenaYAML(t *testing.T) {
	if got := (Options{}).config(); got != "arena.yaml" {
		t.Fatalf("config() = %q, want arena.yaml", got)
	}
	if got := (Options{ConfigPath: "custom.yaml"}).config(); got != "custom.yaml" {
		t.Fatalf("config() = %q, want custom.yaml", got)
	}
}

func TestOptions_DirDefaultsToCwd(t *testing.T) {
	wd, _ := os.Getwd()
	got, err := (Options{}).dir()
	if err != nil {
		t.Fatal(err)
	}
	if got != wd {
		t.Fatalf("dir() = %q, want %q", got, wd)
	}
	if got, _ := (Options{ProjectDir: "/tmp/x"}).dir(); got != "/tmp/x" {
		t.Fatalf("dir() = %q, want /tmp/x", got)
	}
}
