package web

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
