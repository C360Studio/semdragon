package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/storage/workspacerepo"
)

// =============================================================================
// WORKSPACE — Artifact browser backed by workspace repo (git worktrees) or
// the legacy filestore component.
//
// Prefers git worktrees when workspace repo is available; falls back to
// filestore snapshot.
//
//   GET /workspace           — list quests that have artifacts
//   GET /workspace/tree      — file tree for a single quest's artifacts
//   GET /workspace/file      — serve a single artifact file's content
// =============================================================================

// workspaceEntry represents a file or directory in the artifact tree.
type workspaceEntry struct {
	Name     string            `json:"name"`
	Path     string            `json:"path"`
	IsDir    bool              `json:"is_dir"`
	Size     int64             `json:"size"`
	Children []*workspaceEntry `json:"children,omitempty"`
}

// workspaceQuestInfo describes a quest with artifacts.
type workspaceQuestInfo struct {
	QuestID     string `json:"quest_id"`
	Title       string `json:"title,omitempty"`
	Status      string `json:"status,omitempty"`
	Agent       string `json:"agent,omitempty"`
	AgentName   string `json:"agent_name,omitempty"`
	ParentQuest string `json:"parent_quest,omitempty"`
	FileCount   int    `json:"file_count"`
}

// handleWorkspaceQuests returns a list of quests that have artifacts.
//
// GET /game/workspace
func (s *Service) handleWorkspaceQuests(w http.ResponseWriter, r *http.Request) {
	questFiles := make(map[string]int)

	// Try workspace repo (git worktrees) first.
	if wsRepo := s.getWorkspaceRepo(); wsRepo != nil {
		ids, err := wsRepo.ListWorktreeIDs()
		if err == nil {
			for _, instanceID := range ids {
				files, listErr := wsRepo.ListQuestFiles(instanceID)
				if listErr == nil {
					questFiles[instanceID] = len(files)
				}
			}
		}
	} else if store := s.getArtifactStore(); store != nil {
		// Fall back to filestore.
		keys, err := store.List(r.Context(), "quests/")
		if err != nil {
			s.writeError(w, "failed to list artifacts", http.StatusInternalServerError)
			s.logger.Error("workspace: artifact list failed", "error", err)
			return
		}
		for _, key := range keys {
			rest := strings.TrimPrefix(key, "quests/")
			parts := strings.SplitN(rest, "/", 2)
			if len(parts) < 2 {
				continue
			}
			questFiles[parts[0]]++
		}
	} else {
		s.writeError(w, "artifact storage not available", http.StatusServiceUnavailable)
		return
	}

	result := s.enrichQuestInfos(r, questFiles)
	s.writeJSON(w, result)
}

// enrichQuestInfos builds workspaceQuestInfo list from a quest ID → file count map.
func (s *Service) enrichQuestInfos(r *http.Request, questFiles map[string]int) []workspaceQuestInfo {
	result := make([]workspaceQuestInfo, 0, len(questFiles))
	for qid, count := range questFiles {
		info := workspaceQuestInfo{
			QuestID:   qid,
			FileCount: count,
		}

		entity, err := s.graph.GetQuest(r.Context(), domain.QuestID(qid))
		if err == nil {
			quest := domain.QuestFromEntityState(entity)
			if quest != nil {
				info.Title = quest.Title
				info.Status = string(quest.Status)
				if quest.ClaimedBy != nil {
					info.Agent = string(*quest.ClaimedBy)
				}
				if quest.ParentQuest != nil {
					info.ParentQuest = string(*quest.ParentQuest)
				}
			}
		}

		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].QuestID < result[j].QuestID
	})
	return result
}

// handleWorkspaceTree returns a nested file tree for a quest's artifacts.
//
// GET /game/workspace/tree?quest={id}
func (s *Service) handleWorkspaceTree(w http.ResponseWriter, r *http.Request) {
	questID := r.URL.Query().Get("quest")
	if questID == "" || !isValidPathID(questID) {
		s.writeError(w, "quest parameter required", http.StatusBadRequest)
		return
	}

	// Try workspace repo first.
	if wsRepo := s.getWorkspaceRepo(); wsRepo != nil {
		tree, err := wsRepo.FileTree(questID)
		if err != nil {
			s.writeJSON(w, []*workspaceEntry{})
			return
		}
		s.writeJSON(w, convertTreeEntries(tree))
		return
	}

	// Fall back to filestore.
	store := s.getArtifactStore()
	if store == nil {
		s.writeError(w, "artifact storage not available", http.StatusServiceUnavailable)
		return
	}

	prefix := "quests/" + questID + "/"
	keys, err := store.List(r.Context(), prefix)
	if err != nil {
		s.writeError(w, "failed to list artifacts", http.StatusInternalServerError)
		return
	}

	if len(keys) == 0 {
		s.writeJSON(w, []*workspaceEntry{})
		return
	}

	tree := buildArtifactTree(keys, prefix)
	s.writeJSON(w, tree)
}

// handleWorkspaceFile serves a single artifact file's content.
//
// GET /game/workspace/file?quest={id}&path={path}
func (s *Service) handleWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	questID := r.URL.Query().Get("quest")
	filePath := r.URL.Query().Get("path")

	if questID == "" || !isValidPathID(questID) {
		s.writeError(w, "quest parameter required", http.StatusBadRequest)
		return
	}
	if filePath == "" {
		s.writeError(w, "path parameter required", http.StatusBadRequest)
		return
	}
	if strings.Contains(filePath, "..") {
		s.writeError(w, "invalid path", http.StatusBadRequest)
		return
	}

	// Try workspace repo first.
	if wsRepo := s.getWorkspaceRepo(); wsRepo != nil {
		data, err := wsRepo.ReadFile(questID, filePath)
		if err != nil {
			s.writeError(w, "artifact not found", http.StatusNotFound)
			return
		}
		ct := detectContentType(filePath)
		w.Header().Set("Content-Type", ct)
		w.Write(data) //nolint:errcheck
		return
	}

	// Fall back to filestore.
	store := s.getArtifactStore()
	if store == nil {
		s.writeError(w, "artifact storage not available", http.StatusServiceUnavailable)
		return
	}

	key := "quests/" + questID + "/" + filePath
	data, err := store.Get(r.Context(), key)
	if err != nil {
		s.writeError(w, "artifact not found", http.StatusNotFound)
		return
	}

	ct := detectContentType(filePath)
	w.Header().Set("Content-Type", ct)
	w.Write(data) //nolint:errcheck
}

// =============================================================================
// HELPERS
// =============================================================================

// dirNode is used internally by buildArtifactTree to accumulate the tree.
type dirNode struct {
	entry    *workspaceEntry
	children map[string]*dirNode
}

// buildArtifactTree converts a flat list of storage keys into a nested tree.
func buildArtifactTree(keys []string, prefix string) []*workspaceEntry {
	root := &dirNode{children: make(map[string]*dirNode)}

	for _, key := range keys {
		relPath := strings.TrimPrefix(key, prefix)
		if relPath == "" {
			continue
		}

		parts := strings.Split(relPath, "/")
		current := root

		for i, part := range parts {
			isFile := i == len(parts)-1

			if _, exists := current.children[part]; !exists {
				entry := &workspaceEntry{
					Name:  part,
					Path:  strings.Join(parts[:i+1], "/"),
					IsDir: !isFile,
				}
				current.children[part] = &dirNode{
					entry:    entry,
					children: make(map[string]*dirNode),
				}
			}

			current = current.children[part]
		}
	}

	return collectTree(root)
}

// collectTree flattens a dirNode tree into sorted workspaceEntry slices.
func collectTree(node *dirNode) []*workspaceEntry {
	if len(node.children) == 0 {
		return nil
	}

	entries := make([]*workspaceEntry, 0, len(node.children))
	for _, child := range node.children {
		entry := child.entry
		if entry.IsDir {
			entry.Children = collectTree(child)
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	return entries
}

// convertTreeEntries converts workspacerepo.TreeEntry to workspaceEntry.
func convertTreeEntries(entries []*workspacerepo.TreeEntry) []*workspaceEntry {
	if len(entries) == 0 {
		return nil
	}
	result := make([]*workspaceEntry, len(entries))
	for i, e := range entries {
		result[i] = &workspaceEntry{
			Name:     e.Name,
			Path:     e.Path,
			IsDir:    e.IsDir,
			Size:     e.Size,
			Children: convertTreeEntries(e.Children),
		}
	}
	return result
}
