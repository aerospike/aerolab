// Package webui provides the embedded web UI assets and HTTP handlers for serving
// the AeroLab Web UI. The assets are built from web/webui/ and embedded at compile time.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:generate bash -c "cd ../../../web && ./build.sh"

// WebUIFS contains the embedded web UI assets built from web/webui/
// The dist/ directory contains the Vite build output.
//
//go:embed dist/*
var WebUIFS embed.FS

// GetFileSystem returns an fs.FS rooted at dist/
// This removes the "dist/" prefix from paths.
func GetFileSystem() (fs.FS, error) {
	return fs.Sub(WebUIFS, "dist")
}

// GetHTTPFileSystem returns an http.FileSystem rooted at dist/
func GetHTTPFileSystem() (http.FileSystem, error) {
	subFS, err := GetFileSystem()
	if err != nil {
		return nil, err
	}
	return http.FS(subFS), nil
}

// NewFileServer creates an http.Handler that serves the embedded files.
func NewFileServer() (http.Handler, error) {
	fsys, err := GetHTTPFileSystem()
	if err != nil {
		return nil, err
	}
	return http.FileServer(fsys), nil
}

// SPAHandler wraps a file server to handle SPA routing.
// - Serves static files if they exist (js, css, images, etc.)
// - Falls back to index.html for all other routes (SPA client routing)
type SPAHandler struct {
	FileServer http.Handler
	FileSystem http.FileSystem
}

// ServeHTTP implements http.Handler for SPA routing.
func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Clean the path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Try to serve the file directly
	if f, err := h.FileSystem.Open(path); err == nil {
		f.Close()
		h.FileServer.ServeHTTP(w, r)
		return
	}

	// Fall back to index.html for SPA routing
	r.URL.Path = "/"
	h.FileServer.ServeHTTP(w, r)
}

// NewSPAHandler creates a handler for serving the SPA with client-side routing.
func NewSPAHandler() (*SPAHandler, error) {
	fsys, err := GetHTTPFileSystem()
	if err != nil {
		return nil, err
	}
	return &SPAHandler{
		FileServer: http.FileServer(fsys),
		FileSystem: fsys,
	}, nil
}

// ReadIndexHTML reads the index.html file from the embedded filesystem.
func ReadIndexHTML() ([]byte, error) {
	return WebUIFS.ReadFile("dist/index.html")
}
