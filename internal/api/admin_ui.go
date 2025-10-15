package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed admin_ui/*
var adminUIFiles embed.FS

// handleAdminUI serves the admin UI
func (s *Server) handleAdminUI(w http.ResponseWriter, r *http.Request) {
	// Get the requested path
	path := strings.TrimPrefix(r.URL.Path, "/admin")

	// Default to index.html for root paths
	if path == "" || path == "/" {
		path = "/index.html"
	}

	// Remove leading slash for file system access
	filePath := "admin_ui" + path

	// Read the file from embedded filesystem
	content, err := fs.ReadFile(adminUIFiles, filePath)
	if err != nil {
		// If file not found, serve index.html for SPA routing (unless it's a static file)
		if !strings.HasSuffix(path, ".css") && !strings.HasSuffix(path, ".js") && !strings.HasSuffix(path, ".json") {
			content, err = fs.ReadFile(adminUIFiles, "admin_ui/index.html")
			if err != nil {
				http.Error(w, "Page not found", http.StatusNotFound)
				return
			}
			path = "/index.html"
		} else {
			http.Error(w, "File not found: "+path, http.StatusNotFound)
			return
		}
	}

	// Set content type based on file extension
	contentType := "text/html; charset=utf-8"
	if strings.HasSuffix(path, ".css") {
		contentType = "text/css; charset=utf-8"
	} else if strings.HasSuffix(path, ".js") {
		contentType = "application/javascript; charset=utf-8"
	} else if strings.HasSuffix(path, ".json") {
		contentType = "application/json"
	} else if strings.HasSuffix(path, ".png") {
		contentType = "image/png"
	} else if strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".jpeg") {
		contentType = "image/jpeg"
	} else if strings.HasSuffix(path, ".svg") {
		contentType = "image/svg+xml"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(content)
}
