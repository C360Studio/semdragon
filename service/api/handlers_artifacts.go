package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/storage/workspacerepo"
)

// =============================================================================
// ARTIFACT STORAGE
// =============================================================================

// getWorkspaceRepo resolves the workspacerepo component from the registry.
// Returns nil if unavailable.
func (s *Service) getWorkspaceRepo() *workspacerepo.WorkspaceRepo {
	if s.componentDeps == nil || s.componentDeps.ComponentRegistry == nil {
		return nil
	}
	return domain.ResolveWorkspaceRepo(s.componentDeps.ComponentRegistry, s.logger)
}

// =============================================================================
// HANDLERS
// =============================================================================

// handleGetQuestArtifacts serves all files for a quest as a zip archive.
// The response includes quest metadata from the graph as a manifest.json
// at the root of the archive.
//
// GET /api/game/quests/{id}/artifacts
func (s *Service) handleGetQuestArtifacts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid quest ID", http.StatusBadRequest)
		return
	}

	// Collect file paths and a reader function.
	type fileReader func(path string) ([]byte, error)
	var filePaths []string
	var readFile fileReader

	wsRepo := s.getWorkspaceRepo()
	if wsRepo == nil {
		s.writeError(w, "artifact storage not available", http.StatusServiceUnavailable)
		return
	}
	files, err := wsRepo.ListQuestFiles(id)
	if err != nil || len(files) == 0 {
		s.writeError(w, "no artifacts found for quest", http.StatusNotFound)
		return
	}
	for _, f := range files {
		filePaths = append(filePaths, f.Path)
	}
	readFile = func(path string) ([]byte, error) {
		return wsRepo.ReadFile(id, path)
	}

	// Get quest metadata from graph for the manifest.
	entity, graphErr := s.graph.GetQuest(r.Context(), domain.QuestID(id))
	var quest *domain.Quest
	if graphErr == nil {
		quest = domain.QuestFromEntityState(entity)
	}

	// Stream zip response.
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+"-artifacts.zip"))
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	defer zw.Close() //nolint:errcheck

	if quest != nil {
		// Build manifest with file paths as keys (for backward compat).
		keys := make([]string, len(filePaths))
		for i, p := range filePaths {
			keys[i] = "quests/" + id + "/" + p
		}
		manifest := buildArtifactManifest(quest, keys)
		if fw, manifestErr := zw.Create("manifest.json"); manifestErr == nil {
			fw.Write(manifest) //nolint:errcheck
		}
	}

	for _, relPath := range filePaths {
		data, readErr := readFile(relPath)
		if readErr != nil {
			s.logger.Error("skipping artifact in zip", "path", relPath, "error", readErr)
			continue
		}
		fw, createErr := zw.Create(relPath)
		if createErr != nil {
			s.logger.Error("zip create failed", "path", relPath, "error", createErr)
			continue
		}
		fw.Write(data) //nolint:errcheck
	}
}

// handleGetQuestArtifactFile serves a single artifact file.
//
// GET /api/game/quests/{id}/artifacts/{path...}
func (s *Service) handleGetQuestArtifactFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	filePath := r.PathValue("path")
	if !isValidPathID(id) || filePath == "" {
		s.writeError(w, "invalid quest ID or file path", http.StatusBadRequest)
		return
	}
	if strings.Contains(filePath, "..") {
		s.writeError(w, "invalid file path", http.StatusBadRequest)
		return
	}

	wsRepo := s.getWorkspaceRepo()
	if wsRepo == nil {
		s.writeError(w, "artifact storage not available", http.StatusServiceUnavailable)
		return
	}

	data, readErr := wsRepo.ReadFile(id, filePath)

	if readErr != nil {
		s.logger.Debug("artifact get failed", "quest_id", id, "path", filePath, "error", readErr)
		s.writeError(w, "artifact not found", http.StatusNotFound)
		return
	}

	ct := detectContentType(filePath)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Write(data) //nolint:errcheck
}

// handleListQuestArtifacts returns a JSON list of artifact files for a quest.
//
// GET /api/game/quests/{id}/artifacts/list
func (s *Service) handleListQuestArtifacts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidPathID(id) {
		s.writeError(w, "invalid quest ID", http.StatusBadRequest)
		return
	}

	wsRepo := s.getWorkspaceRepo()
	if wsRepo == nil {
		s.writeError(w, "artifact storage not available", http.StatusServiceUnavailable)
		return
	}

	var files []string
	entries, err := wsRepo.ListQuestFiles(id)
	if err != nil {
		s.writeError(w, "failed to list artifacts", http.StatusInternalServerError)
		return
	}
	for _, e := range entries {
		files = append(files, e.Path)
	}

	if files == nil {
		files = []string{}
	}

	s.writeJSON(w, map[string]any{
		"quest_id": id,
		"files":    files,
		"count":    len(files),
	})
}

// =============================================================================
// HELPERS
// =============================================================================

// buildArtifactManifest creates a JSON manifest with quest metadata.
func buildArtifactManifest(quest *domain.Quest, keys []string) []byte {
	manifest := map[string]any{
		"quest_id":    quest.ID,
		"title":       quest.Title,
		"description": quest.Description,
		"status":      quest.Status,
		"difficulty":  quest.Difficulty,
		"claimed_by":  quest.ClaimedBy,
		"file_count":  len(keys),
	}
	if quest.RequiredSkills != nil {
		manifest["required_skills"] = quest.RequiredSkills
	}

	data, _ := json.MarshalIndent(manifest, "", "  ")
	return data
}

// detectContentType returns a MIME type based on file extension.
func detectContentType(path string) string {
	switch {
	case hasExtension(path, ".go"):
		return "text/x-go; charset=utf-8"
	case hasExtension(path, ".ts", ".tsx"):
		return "text/typescript; charset=utf-8"
	case hasExtension(path, ".js", ".jsx"):
		return "text/javascript; charset=utf-8"
	case hasExtension(path, ".java"):
		return "text/x-java; charset=utf-8"
	case hasExtension(path, ".py"):
		return "text/x-python; charset=utf-8"
	case hasExtension(path, ".json"):
		return "application/json"
	case hasExtension(path, ".yaml", ".yml"):
		return "text/yaml; charset=utf-8"
	case hasExtension(path, ".md"):
		return "text/markdown; charset=utf-8"
	case hasExtension(path, ".html"):
		return "text/html; charset=utf-8"
	case hasExtension(path, ".css"):
		return "text/css; charset=utf-8"
	case hasExtension(path, ".txt", ".log"):
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// hasExtension checks if a path ends with any of the given extensions.
func hasExtension(path string, exts ...string) bool {
	for _, ext := range exts {
		if len(path) > len(ext) && path[len(path)-len(ext):] == ext {
			return true
		}
	}
	return false
}
