package engine

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestEngine_HeadlessBuildIsStatic builds a program that imports arena/engine
// with CGO_ENABLED=0 for Linux, and requires a static binary.
//
// Headless workers (Omnia's arena-worker) run on distroless/static, which has
// no dynamic loader. A dependency that links libc dynamically even without cgo
// (purego, through the oto audio backend) makes the binary exit "no such file
// or directory" before main. Audio backends belong in arena/audio/otobackend,
// which only the CLI imports.
func TestEngine_HeadlessBuildIsStatic(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	out := filepath.Join(t.TempDir(), "headless")
	cmd := exec.Command("go", "build", "-o", out, "./testdata/headlessmain")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}

	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("open binary: %v", err)
	}
	defer f.Close()
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			libs, _ := f.ImportedLibraries()
			t.Fatalf("binary is dynamically linked (needs %v); something in arena/engine's imports links libc", libs)
		}
	}
}
