package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

const spaIndexBody = "<!doctype html><div id=root></div>"

// testBundle mirrors the shape of a built frontend: an index.html at the root
// and hashed assets in a subdirectory.
func testBundle() fstest.MapFS {
	return fstest.MapFS{
		"index.html":            &fstest.MapFile{Data: []byte(spaIndexBody)},
		"assets/app-a1b2c3.js":  &fstest.MapFile{Data: []byte("console.log(1)")},
		"assets/app-a1b2c3.css": &fstest.MapFile{Data: []byte(":root{}")},
	}
}

// serveSPA drives the handler with an arbitrary request path, bypassing the
// mux so paths a ServeMux would clean or redirect still reach the handler.
func serveSPA(t *testing.T, bundle fstest.MapFS, urlPath string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/placeholder", nil)
	req.URL.Path = urlPath
	rec := httptest.NewRecorder()
	spaHandler(bundle).ServeHTTP(rec, req)
	return rec
}

func TestSPAServesBundledAsset(t *testing.T) {
	rec := serveSPA(t, testBundle(), "/assets/app-a1b2c3.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "console.log(1)" {
		t.Errorf("body = %q, want the asset contents", got)
	}
}

func TestSPARootServesIndex(t *testing.T) {
	rec := serveSPA(t, testBundle(), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != spaIndexBody {
		t.Errorf("body = %q, want index.html", got)
	}
}

// A deep link into a client-side route has no file behind it and must still
// boot the app rather than 404.
func TestSPADeepLinkFallsBackToIndex(t *testing.T) {
	rec := serveSPA(t, testBundle(), "/results/run-123")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != spaIndexBody {
		t.Errorf("body = %q, want index.html", got)
	}
}

// Traversal attempts must never escape the bundle. They are not in the asset
// set, so they take the same fallback as any unknown route.
func TestSPATraversalAttemptsFallBackToIndex(t *testing.T) {
	escapes := []string{
		"/../server.go",
		"/../../../../etc/passwd",
		"/assets/../../server.go",
		"//etc/passwd",
		"/./../../embed.go",
	}

	for _, urlPath := range escapes {
		t.Run(urlPath, func(t *testing.T) {
			rec := serveSPA(t, testBundle(), urlPath)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if got := rec.Body.String(); got != spaIndexBody {
				t.Errorf("body = %q, want index.html — path escaped the bundle", got)
			}
		})
	}
}

// Directories are not servable content: requesting one must not expose a
// listing of the bundle's layout.
func TestSPADirectoryRequestFallsBackToIndex(t *testing.T) {
	rec := serveSPA(t, testBundle(), "/assets")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != spaIndexBody {
		t.Errorf("body = %q, want index.html rather than a directory listing", got)
	}
}

func TestBundledAssetsListsFilesNotDirectories(t *testing.T) {
	assets := bundledAssets(testBundle())

	want := []string{"index.html", "assets/app-a1b2c3.js", "assets/app-a1b2c3.css"}
	for _, name := range want {
		if _, ok := assets[name]; !ok {
			t.Errorf("asset set missing %q", name)
		}
	}
	for _, name := range []string{".", "assets"} {
		if _, ok := assets[name]; ok {
			t.Errorf("asset set contains directory %q", name)
		}
	}
	if len(assets) != len(want) {
		t.Errorf("asset set size = %d, want %d: %v", len(assets), len(want), assets)
	}
}

func TestRegisterSPAFallbackMountsBundleSubdirectory(t *testing.T) {
	s := &Server{mux: http.NewServeMux()}
	s.registerSPAFallback(fstest.MapFS{
		"frontend/dist/index.html": &fstest.MapFile{Data: []byte(spaIndexBody)},
		// Outside frontend/dist — must be unreachable through the route.
		"secret.txt": &fstest.MapFile{Data: []byte("do not serve")},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != spaIndexBody {
		t.Errorf("body = %q, want index.html from frontend/dist", got)
	}
}

func TestRegisterSPAFallbackIgnoresUnreachableBundle(t *testing.T) {
	// No frontend/dist in the bundle: the route still mounts, and every
	// request falls back to an index.html that is not there.
	s := &Server{mux: http.NewServeMux()}
	s.registerSPAFallback(fstest.MapFS{"other.txt": &fstest.MapFile{Data: []byte("x")}})

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
