package flow

import (
	"os"
	"testing"
)

// TestOptions_ConfigDefaultMatchesRun pins deploy's default config to the one
// `run` and `init` use, with arena.yaml still honoured when it is the only
// config in the directory (projects set up for deploy's old default).
func TestOptions_ConfigDefaultMatchesRun(t *testing.T) {
	t.Chdir(t.TempDir())

	if got := (Options{}).Config(); got != DefaultConfigFile {
		t.Fatalf("empty dir: Config() = %q, want %q", got, DefaultConfigFile)
	}
	if got := (Options{ConfigPath: "custom.yaml"}).config(); got != "custom.yaml" {
		t.Fatalf("config() = %q, want custom.yaml", got)
	}

	if err := os.WriteFile(LegacyConfigFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := (Options{}).Config(); got != LegacyConfigFile {
		t.Fatalf("only arena.yaml: Config() = %q, want the legacy fallback", got)
	}

	if err := os.WriteFile(DefaultConfigFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := (Options{}).Config(); got != DefaultConfigFile {
		t.Fatalf("both present: Config() = %q, want %q", got, DefaultConfigFile)
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
