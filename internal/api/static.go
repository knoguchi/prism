package api

import (
	"io/fs"
	"net/http"
	"strings"
)

// StaticHandler returns an http.Handler that serves static files from the given filesystem
// For SPA routing, non-file paths are served index.html
func StaticHandler(distFS fs.FS) http.Handler {
	if distFS == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Static files not available. Run 'make build' or use 'make run-web' for development.", http.StatusNotFound)
		})
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Skip API routes
		if strings.HasPrefix(path, "/api") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the file directly
		if path != "/" {
			// Check if file exists
			cleanPath := strings.TrimPrefix(path, "/")
			if f, err := distFS.Open(cleanPath); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// For SPA: serve index.html for all non-file routes
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
