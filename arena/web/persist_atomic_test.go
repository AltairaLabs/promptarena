package web

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AltairaLabs/promptarena/arena/statestore"
)

// TestPersistOneRun_NeverExposesPartialJSON locks in the invariant that a run's
// JSON file is only ever observable complete. persistOneRun rewrites the same
// path every time a run finishes, and readers scan that directory concurrently
// — LoadResultsIntoStore does exactly this when `promptarena serve` starts, and
// so does anything watching the output dir during a batch.
//
// A plain os.WriteFile truncates the file and then writes it, leaving a window
// where a reader observes zero bytes or a prefix. The symptom is an unmarshal
// failure ("unexpected end of JSON input") against a file that plainly exists,
// which is what made TestStartRun_PersistsResultsToDisk flaky in CI.
func TestPersistOneRun_NeverExposesPartialJSON(t *testing.T) {
	tmpDir := t.TempDir()
	store := statestore.NewArenaStateStore()
	ctx := context.Background()
	const runID = "run-atomic-1"

	// Seed enough metadata that the marshalled payload is not a single trivial
	// write — a wider file widens the truncate/write window a reader can land in.
	longID := ""
	for i := 0; i < 512; i++ {
		longID += "scenario-with-a-deliberately-long-identifier/"
	}
	if err := store.SaveMetadata(ctx, runID, &statestore.RunMetadata{
		RunID:      runID,
		ScenarioID: longID,
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("SaveMetadata: %v", err)
	}

	srv := newServerWithRunner(NewEventAdapter(), nil, store, tmpDir)
	path := filepath.Join(tmpDir, runID+".json")

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		corrupt  []byte
		stopping = make(chan struct{})
	)

	// Reader: whenever the file is readable, its contents MUST parse. A file
	// that exists but does not parse is the defect.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopping:
				return
			default:
			}
			b, err := os.ReadFile(path)
			if err != nil {
				continue // not created yet — legitimate
			}
			var res statestore.RunResult
			if json.Unmarshal(b, &res) != nil {
				mu.Lock()
				if corrupt == nil {
					corrupt = b
				}
				mu.Unlock()
				return
			}
		}
	}()

	for i := 0; i < 300; i++ {
		srv.persistOneRun(ctx, runID)
	}
	close(stopping)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if corrupt != nil {
		t.Fatalf("reader observed a partially-written %s (%d bytes); "+
			"the file must be swapped into place atomically so readers never see a prefix",
			path, len(corrupt))
	}
}

// TestWriteFileAtomic_WritesContentAndMode covers the success path: the data
// lands intact under the destination name, with the requested mode rather than
// the 0600 os.CreateTemp gives the staging file.
func TestWriteFileAtomic_WritesContentAndMode(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "result.json")
	want := []byte(`{"runID":"abc"}`)

	if err := writeFileAtomic(dest, want, 0o640); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("contents = %q, want %q", got, want)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want %v (staging file's 0600 must not leak through)",
			info.Mode().Perm(), os.FileMode(0o640))
	}
}

// TestWriteFileAtomic_ReplacesExistingFile covers the overwrite path — the one
// persistOneRun actually takes on every run after the first.
func TestWriteFileAtomic_ReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "result.json")
	if err := os.WriteFile(dest, []byte("stale"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := writeFileAtomic(dest, []byte("fresh"), 0o600); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "fresh" {
		t.Errorf("contents = %q, want %q", got, "fresh")
	}
}

// TestWriteFileAtomic_StagingFailureReturnsError covers the branch where the
// staging file cannot be created at all — here because the destination's
// directory does not exist.
func TestWriteFileAtomic_StagingFailureReturnsError(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "no-such-dir", "result.json")

	if err := writeFileAtomic(dest, []byte("x"), 0o600); err == nil {
		t.Fatal("expected an error when the destination directory does not exist")
	}
}

// TestWriteFileAtomic_RenameFailureLeavesNoStagingFile covers the failure
// cleanup: when the swap into place fails, the staging file must not be left
// behind in the results directory, where a later scan would trip over it.
// A destination that is a directory makes the rename fail.
func TestWriteFileAtomic_RenameFailureLeavesNoStagingFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "result.json")
	if err := os.Mkdir(dest, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A non-empty directory cannot be replaced by a rename on any platform.
	if err := os.WriteFile(filepath.Join(dest, "occupant"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed occupant: %v", err)
	}

	if err := writeFileAtomic(dest, []byte("x"), 0o600); err == nil {
		t.Fatal("expected an error when the destination cannot be replaced")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("staging file %q was left behind after a failed write", e.Name())
		}
	}
}
