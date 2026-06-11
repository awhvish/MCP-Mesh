package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxReadBytes = 1 << 20 // 1 MB

func (r *Registry) isAllowed(path string) bool {
	if len(r.cfg.Agent.AllowedPaths) == 0 {
		return true
	}
	clean := filepath.Clean(path)
	for _, allowed := range r.cfg.Agent.AllowedPaths {
		a := filepath.Clean(allowed)
		// exact match or proper subdirectory (the separator prevents /home matching /homeuser)
		if clean == a || strings.HasPrefix(clean, a+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func (r *Registry) listFiles(params map[string]any) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path param required")
	}
	if !r.isAllowed(path) {
		return "", fmt.Errorf("path %q is outside allowed directories", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("reading directory: %w", err)
	}

	type fileInfo struct {
		Name  string `json:"name"`
		Size  int64  `json:"size"`
		IsDir bool   `json:"is_dir"`
	}

	files := make([]fileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			Name:  e.Name(),
			Size:  info.Size(),
			IsDir: e.IsDir(),
		})
	}

	out, err := json.Marshal(files)
	if err != nil {
		return "", fmt.Errorf("encoding result: %w", err)
	}
	return string(out), nil
}

func (r *Registry) readFile(params map[string]any) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path param required")
	}
	if !r.isAllowed(path) {
		return "", fmt.Errorf("path %q is outside allowed directories", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat: %w", err)
	}
	if info.Size() > maxReadBytes {
		return "", fmt.Errorf("file too large (%d bytes, max 1 MB)", info.Size())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}
	return string(data), nil
}
