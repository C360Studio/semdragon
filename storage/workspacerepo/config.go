package workspacerepo

import (
	"errors"
)

// Config holds configuration for the workspace repo component.
type Config struct {
	// RepoDir is the path to the bare git repository. Auto-initialized
	// on first Start if it does not exist.
	RepoDir string `json:"repo_dir" schema:"type:string,description:Path to the bare git repository,category:basic,required:true"`

	// WorktreesDir is the directory under which per-quest worktrees are created.
	WorktreesDir string `json:"worktrees_dir" schema:"type:string,description:Directory for per-quest git worktrees,category:basic,required:true"`

	// MainWorktreeDir is the path where a persistent checkout of the main branch
	// is maintained. Updated after each MergeToMain. Semsource watches this
	// directory (RO) for AST/doc/config indexing.
	MainWorktreeDir string `json:"main_worktree_dir" schema:"type:string,description:Persistent main branch checkout for semsource watching,category:advanced"`

	// RetentionDays controls how long completed quest worktrees are kept
	// before pruning. Zero disables automatic pruning.
	RetentionDays int `json:"retention_days" schema:"type:int,description:Days to retain completed quest worktrees (0=no pruning),category:advanced,default:30"`
}

// Validate checks that the configuration is complete and consistent.
func (c *Config) Validate() error {
	if c.RepoDir == "" {
		return errors.New("repo_dir is required")
	}
	if c.WorktreesDir == "" {
		return errors.New("worktrees_dir is required")
	}
	return nil
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		RepoDir:         "/var/semdragons/workspace.git",
		WorktreesDir:    "/var/semdragons/quest-worktrees",
		MainWorktreeDir: "/var/semdragons/workspace-main",
		RetentionDays:   30,
	}
}
