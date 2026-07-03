package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AltairaLabs/promptarena/arena/engine"
	"github.com/AltairaLabs/promptarena/arena/statestore"
)

func TestHandleRunOptions_NoEngine(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/run-options", nil)
	rec := httptest.NewRecorder()

	srv.handleRunOptions(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleRunOptions_Success(t *testing.T) {
	mock := &mockEngine{
		providers: []engine.ProviderInfo{{ID: "mock", Type: "mock"}, {ID: "openai", Type: "openai", Model: "gpt-4o"}},
		scenarios: []engine.ScenarioInfo{{ID: "greeting", Description: "say hi"}},
	}
	srv := newServerWithRunner(NewEventAdapter(), mock, nil, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/run-options") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /api/run-options: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got runOptionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Providers) != 2 {
		t.Errorf("got %d providers, want 2", len(got.Providers))
	}
	if len(got.Scenarios) != 1 || got.Scenarios[0].ID != "greeting" {
		t.Errorf("scenarios = %+v, want [greeting]", got.Scenarios)
	}
}

func TestHandleMedia_NotConfigured(t *testing.T) {
	srv := &Server{} // outputDir == ""
	req := httptest.NewRequest(http.MethodGet, "/api/media/foo.wav", nil)
	req.SetPathValue("path", "foo.wav")
	rec := httptest.NewRecorder()

	srv.handleMedia(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleMedia_MissingPath(t *testing.T) {
	srv := &Server{outputDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/media/", nil)
	req.SetPathValue("path", "")
	rec := httptest.NewRecorder()

	srv.handleMedia(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleMedia_Traversal(t *testing.T) {
	srv := &Server{outputDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/media/x", nil)
	// SetPathValue bypasses the http client path cleaning so we can exercise
	// the traversal guard directly.
	req.SetPathValue("path", "../../../etc/passwd")
	rec := httptest.NewRecorder()

	srv.handleMedia(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d (body %q)", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestHandleMedia_ServesFile(t *testing.T) {
	dir := t.TempDir()
	want := []byte("RIFFfake-wav-bytes")
	if err := os.WriteFile(filepath.Join(dir, "clip.wav"), want, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	srv := &Server{outputDir: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/media/clip.wav", nil)
	req.SetPathValue("path", "clip.wav")
	rec := httptest.NewRecorder()

	srv.handleMedia(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.Bytes(); string(got) != string(want) {
		t.Errorf("body = %q, want %q", got, want)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, want immutable directive", cc)
	}
}

// TestHandleMedia_StripsOutPrefix confirms a storage_reference saved as
// "out/<rel>" resolves against outputDir after the leading "out/" is stripped.
func TestHandleMedia_StripsOutPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clip.wav"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	srv := &Server{outputDir: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/media/out/clip.wav", nil)
	req.SetPathValue("path", "out/clip.wav")
	rec := httptest.NewRecorder()

	srv.handleMedia(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestLoadOneResult_ErrorPaths exercises the silent-skip branches of
// loadOneResult: unreadable file, malformed JSON, and an empty RunID all
// report false without hydrating the store.
func TestLoadOneResult_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	store := statestore.NewArenaStateStore()
	dir := t.TempDir()

	t.Run("missing file", func(t *testing.T) {
		if loadOneResult(ctx, store, filepath.Join(dir, "nope.json")) {
			t.Error("expected false for missing file")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		p := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if loadOneResult(ctx, store, p) {
			t.Error("expected false for malformed JSON")
		}
	})

	t.Run("empty RunID", func(t *testing.T) {
		p := filepath.Join(dir, "empty.json")
		if err := os.WriteFile(p, []byte(`{"RunID":""}`), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if loadOneResult(ctx, store, p) {
			t.Error("expected false for empty RunID")
		}
	})

	t.Run("valid result", func(t *testing.T) {
		p := filepath.Join(dir, "ok.json")
		body := `{"RunID":"run-ok","ScenarioID":"s1","ProviderID":"p1","Messages":[]}`
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if !loadOneResult(ctx, store, p) {
			t.Fatal("expected true for valid result")
		}
		ids, _ := store.ListRunIDs(ctx)
		found := false
		for _, id := range ids {
			if id == "run-ok" {
				found = true
			}
		}
		if !found {
			t.Errorf("store missing run-ok, has %v", ids)
		}
	})
}
