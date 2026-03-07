package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// maxFileSize caps file content responses at 1 MB.
const maxFileSize = 1 << 20

// maxTreeDepth limits recursive directory traversal to prevent stack exhaustion.
const maxTreeDepth = 10

// maxTreeEntries caps the total number of entries returned by buildTree.
const maxTreeEntries = 5000

// skipDirs lists directories that are always excluded from the workspace tree
// because they are large, generated, or not meaningful to the user.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	"dist":         true,
	"build":        true,
}

// workspaceEntry represents a file or directory in the workspace tree.
type workspaceEntry struct {
	Name     string            `json:"name"`
	Path     string            `json:"path"`
	IsDir    bool              `json:"is_dir"`
	Size     int64             `json:"size"`
	Modified time.Time         `json:"modified"`
	Children []*workspaceEntry `json:"children,omitempty"`
}

// resolveRoot converts a workspace directory path to an absolute, symlink-resolved
// path suitable for use as a safe containment boundary. Both filepath.Abs and
// filepath.EvalSymlinks are required: Abs handles relative paths, EvalSymlinks
// ensures that child paths resolved through EvalSymlinks share the same prefix
// (critical when WorkspaceDir itself is a symlink, e.g. in Docker environments).
func resolveRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

// handleWorkspaceTree returns a recursive JSON tree of the workspace directory.
func (s *Service) handleWorkspaceTree(w http.ResponseWriter, r *http.Request) {
	root := s.config.WorkspaceDir
	if root == "" {
		s.writeError(w, "workspace not configured", http.StatusNotFound)
		return
	}

	// Resolve to absolute, symlink-resolved path so that child EvalSymlinks
	// results share the same prefix. See resolveRoot for rationale.
	absRoot, err := resolveRoot(root)
	if err != nil {
		s.writeError(w, "invalid workspace path", http.StatusInternalServerError)
		return
	}

	info, err := os.Stat(absRoot)
	if err != nil || !info.IsDir() {
		s.writeError(w, "workspace directory not found", http.StatusNotFound)
		return
	}

	count := 0
	tree, err := buildTree(r.Context(), absRoot, absRoot, 0, &count)
	if err != nil {
		s.logger.Error("failed to walk workspace", "error", err)
		s.writeError(w, "failed to read workspace", http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, tree)
}

// buildTree walks a directory recursively and returns a slice of workspaceEntry.
// depth tracks the current nesting level and is capped at maxTreeDepth.
// count is a shared pointer tracking total entries across all recursive calls,
// capped at maxTreeEntries to prevent unbounded responses on large workspaces.
func buildTree(ctx context.Context, root, dir string, depth int, count *int) ([]*workspaceEntry, error) {
	if depth > maxTreeDepth {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var result []*workspaceEntry
	for _, e := range entries {
		// Honour context cancellation between iterations so that a slow
		// filesystem walk doesn't block request teardown.
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		if *count >= maxTreeEntries {
			break
		}

		// Skip hidden files/directories.
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}

		// Skip common large generated/dependency directories.
		if e.IsDir() && skipDirs[e.Name()] {
			continue
		}

		fullPath := filepath.Join(dir, e.Name())

		// Resolve symlinks and verify still within workspace root.
		resolved, err := filepath.EvalSymlinks(fullPath)
		if err != nil {
			continue // skip broken symlinks
		}
		if !strings.HasPrefix(resolved, root+string(filepath.Separator)) && resolved != root {
			continue // symlink escapes workspace
		}

		info, err := os.Stat(resolved)
		if err != nil {
			continue
		}

		relPath, err := filepath.Rel(root, fullPath)
		if err != nil {
			continue
		}

		entry := &workspaceEntry{
			Name:     e.Name(),
			Path:     filepath.ToSlash(relPath),
			IsDir:    info.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime().UTC(),
		}

		*count++

		if info.IsDir() {
			children, err := buildTree(ctx, root, fullPath, depth+1, count)
			if err != nil {
				continue
			}
			entry.Children = children
		}

		result = append(result, entry)
	}

	return result, nil
}

// handleWorkspaceFile returns the content of a single file from the workspace.
func (s *Service) handleWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	root := s.config.WorkspaceDir
	if root == "" {
		s.writeError(w, "workspace not configured", http.StatusNotFound)
		return
	}

	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		s.writeError(w, "path parameter required", http.StatusBadRequest)
		return
	}

	// Security: reject absolute paths and parent traversals.
	if filepath.IsAbs(relPath) || strings.Contains(relPath, "..") {
		s.writeError(w, "invalid path", http.StatusBadRequest)
		return
	}

	// Resolve to absolute, symlink-resolved root. See resolveRoot for rationale.
	absRoot, err := resolveRoot(root)
	if err != nil {
		s.writeError(w, "invalid workspace path", http.StatusInternalServerError)
		return
	}

	// Clean and resolve the full path.
	fullPath := filepath.Join(absRoot, filepath.Clean(relPath))

	// TOCTOU mitigation: Lstat the path before following any symlink. If the
	// entry is itself a symlink, reject it outright for direct file access.
	// This avoids a race between Lstat and EvalSymlinks where an attacker
	// could swap a regular file for a symlink pointing outside the workspace.
	linfo, err := os.Lstat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.writeError(w, "file not found", http.StatusNotFound)
			return
		}
		s.writeError(w, "invalid path", http.StatusBadRequest)
		return
	}
	if linfo.Mode()&os.ModeSymlink != 0 {
		s.writeError(w, "symlinks not allowed for direct file access", http.StatusBadRequest)
		return
	}

	// Resolve symlinks before prefix check.
	resolved, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.writeError(w, "file not found", http.StatusNotFound)
			return
		}
		s.writeError(w, "invalid path", http.StatusBadRequest)
		return
	}

	// Verify resolved path is within workspace root.
	if !strings.HasPrefix(resolved, absRoot+string(filepath.Separator)) && resolved != absRoot {
		s.writeError(w, "path outside workspace", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(resolved)
	if err != nil {
		s.writeError(w, "file not found", http.StatusNotFound)
		return
	}
	if info.IsDir() {
		s.writeError(w, "path is a directory", http.StatusBadRequest)
		return
	}
	if info.Size() > maxFileSize {
		s.writeError(w, fmt.Sprintf("file too large (%d bytes, max %d)", info.Size(), maxFileSize), http.StatusRequestEntityTooLarge)
		return
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		if pathErr, ok := err.(*os.PathError); ok && os.IsPermission(pathErr.Err) {
			s.writeError(w, "permission denied", http.StatusForbidden)
			return
		}
		s.writeError(w, "failed to read file", http.StatusInternalServerError)
		return
	}

	// Check if content is valid UTF-8 text.
	if !utf8.Valid(data) {
		s.writeError(w, "binary file not supported", http.StatusUnsupportedMediaType)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := w.Write(data); err != nil {
		s.logger.Error("failed to write workspace file response", "error", err)
	}
}
