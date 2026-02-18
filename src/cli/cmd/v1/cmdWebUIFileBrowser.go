package cmd

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// handleFSHomedir handles GET /api/fs/homedir
// Returns the user's home directory. Query param 'path' can override for validation.
func (c *WebUICmd) handleFSHomedir(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	if !c.allowServerBrowse(r) {
		http.Error(w, "Server file browsing is not allowed from this origin", http.StatusForbidden)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParam := r.URL.Query().Get("path")
	var resultPath string

	if pathParam != "" {
		// Validate that the path exists and is a directory
		absPath, err := filepath.Abs(pathParam)
		if err != nil {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "Path does not exist", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to access path", http.StatusInternalServerError)
			return
		}
		if !info.IsDir() {
			resultPath = filepath.Dir(absPath)
		} else {
			resultPath = absPath
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			http.Error(w, "Failed to get home directory", http.StatusInternalServerError)
			return
		}
		resultPath = home
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]string{"path": resultPath})
}

// handleFSLs handles GET /api/fs/ls
// Lists directory contents. Query param 'path' is required.
func (c *WebUICmd) handleFSLs(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	if !c.allowServerBrowse(r) {
		http.Error(w, "Server file browsing is not allowed from this origin", http.StatusForbidden)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParam := r.URL.Query().Get("path")
	if pathParam == "" {
		http.Error(w, "path query parameter is required", http.StatusBadRequest)
		return
	}

	absPath, err := filepath.Abs(pathParam)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Path does not exist", http.StatusNotFound)
			return
		}
		if os.IsPermission(err) {
			http.Error(w, "Permission denied", http.StatusForbidden)
			return
		}
		http.Error(w, "Failed to list directory", http.StatusInternalServerError)
		return
	}

	dirs := []string{}
	files := []string{}
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || name[0] == '.' {
			// Skip hidden files by default, or include them - including for now
		}
		if entry.IsDir() {
			dirs = append(dirs, name)
		} else {
			files = append(files, name)
		}
	}

	result := map[string]any{
		"path":  absPath,
		"dirs":  dirs,
		"files": files,
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		c.logError("Failed to encode fs/ls response: %s", err)
	}
}
