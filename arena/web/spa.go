package web

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// registerSPAFallback wires the catch-all route that serves the frontend bundle
// baked into the binary at build time. It is registered last so the /api/
// patterns win; everything else lands here.
func (s *Server) registerSPAFallback(bundle fs.FS) {
	sub, err := fs.Sub(bundle, "frontend/dist")
	if err != nil {
		// No bundle mounted (a build without frontend output). Leave "/"
		// unregistered so the API handlers are all the mux knows about.
		return
	}
	s.mux.Handle("/", spaHandler(sub))
}

// spaHandler serves an exact hit from the bundle and falls through to
// index.html for anything else, so client-side routes survive a hard refresh
// or a deep link.
//
// The servable files are enumerated once, up front, and the request path is
// only ever matched against that fixed set — nothing derived from the request
// reaches a file-open call. A crafted path therefore cannot address anything
// outside the bundle; it misses the set and gets index.html like any other
// unknown route. Directories miss too, so the bundle's layout is never listed.
func spaHandler(bundle fs.FS) http.Handler {
	assets := bundledAssets(bundle)
	fileServer := http.FileServer(http.FS(bundle))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if _, ok := assets[path.Clean(name)]; ok {
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: serve index.html for client-side routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// bundledAssets walks the bundle once and returns the set of file paths it
// holds, in the slash-separated, root-relative form a request path reduces to
// (e.g. "assets/index-a1b2c3.js"). Directories are omitted: they are not
// servable content.
func bundledAssets(bundle fs.FS) map[string]struct{} {
	assets := make(map[string]struct{})
	_ = fs.WalkDir(bundle, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable subtrees rather than abandoning the walk — a
			// partial set still serves what is readable.
			return nil
		}
		if !d.IsDir() {
			assets[p] = struct{}{}
		}
		return nil
	})
	return assets
}
