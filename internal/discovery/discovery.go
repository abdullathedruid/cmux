// Package discovery provides session discovery with repository grouping.
package discovery

import (
	"path/filepath"
	"time"

	"github.com/abdullathedruid/cmux/internal/config"
	"github.com/abdullathedruid/cmux/internal/git"
	"github.com/abdullathedruid/cmux/internal/state"
	"github.com/abdullathedruid/cmux/internal/tmux"
)

// Service discovers tmux sessions and groups them by repository.
type Service struct {
	tmux   *tmux.RealClient
	config *config.Config
}

// NewService creates a new discovery service.
func NewService(t *tmux.RealClient, cfg *config.Config) *Service {
	return &Service{
		tmux:   t,
		config: cfg,
	}
}

// DiscoverSessions discovers all tmux sessions and returns those that belong
// to configured repositories. Sessions are enriched with repository info.
func (s *Service) DiscoverSessions() ([]*state.Session, error) {
	// Get all Claude sessions from tmux
	tmuxSessions, err := s.tmux.DiscoverClaudeSessions()
	if err != nil {
		return nil, err
	}

	// Get expanded repository paths for comparison
	configuredRepos := make(map[string]bool)
	for _, repo := range s.config.ExpandedRepositories() {
		absPath, err := filepath.Abs(repo)
		if err != nil {
			continue
		}
		configuredRepos[absPath] = true
	}

	var sessions []*state.Session

	for _, ts := range tmuxSessions {
		// Get working directory for this session
		workDir, err := s.tmux.GetSessionWorkingDir(ts.Name)
		if err != nil || workDir == "" {
			continue
		}

		// Find the repository root
		repoRoot, err := git.FindRepoRoot(workDir)
		if err != nil {
			// Not a git repo, skip
			continue
		}

		absRepoRoot, err := filepath.Abs(repoRoot)
		if err != nil {
			continue
		}

		// Filter: only include sessions from configured repositories
		if !configuredRepos[absRepoRoot] {
			continue
		}

		// Get branch info
		branch, err := git.GetCurrentBranch(workDir)
		if err != nil {
			branch = "unknown"
		}

		// Determine if this is a worktree
		isWorktree := git.IsWorktreePath(workDir)

		sess := &state.Session{
			Name:       ts.Name,
			RepoPath:   repoRoot,
			RepoName:   filepath.Base(repoRoot),
			Worktree:   workDir,
			Branch:     branch,
			Attached:   ts.Attached,
			Created:    ts.Created,
			LastActive: time.Now(),
		}

		// Mark if it's a worktree path vs main repo
		if !isWorktree {
			sess.Worktree = "" // Main repo, not a worktree
		}

		sessions = append(sessions, sess)
	}

	return sessions, nil
}

// DiscoverAllSessions discovers all Claude sessions regardless of repository configuration.
// This is useful for the sidebar view which may show sessions outside configured repos.
func (s *Service) DiscoverAllSessions() ([]*state.Session, error) {
	tmuxSessions, err := s.tmux.DiscoverClaudeSessions()
	if err != nil {
		return nil, err
	}

	var sessions []*state.Session

	for _, ts := range tmuxSessions {
		sess := &state.Session{
			Name:       ts.Name,
			Attached:   ts.Attached,
			Created:    ts.Created,
			LastActive: time.Now(),
		}

		// Try to get working directory and repo info
		workDir, err := s.tmux.GetSessionWorkingDir(ts.Name)
		if err == nil && workDir != "" {
			sess.Worktree = workDir

			// Try to find repo root
			if repoRoot, err := git.FindRepoRoot(workDir); err == nil {
				sess.RepoPath = repoRoot
				sess.RepoName = filepath.Base(repoRoot)

				// Get branch
				if branch, err := git.GetCurrentBranch(workDir); err == nil {
					sess.Branch = branch
				}

				// Determine if worktree
				if !git.IsWorktreePath(workDir) {
					sess.Worktree = ""
				}
			}
		}

		sessions = append(sessions, sess)
	}

	return sessions, nil
}

// GetConfiguredRepositories returns all configured repository paths with their info.
func (s *Service) GetConfiguredRepositories() []RepositoryInfo {
	var repos []RepositoryInfo

	for _, repoPath := range s.config.ExpandedRepositories() {
		absPath, err := filepath.Abs(repoPath)
		if err != nil {
			continue
		}

		// Verify it's a git repo
		if _, err := git.FindRepoRoot(absPath); err != nil {
			continue
		}

		repos = append(repos, RepositoryInfo{
			Path: absPath,
			Name: filepath.Base(absPath),
		})
	}

	return repos
}

// RepositoryInfo contains basic repository information.
type RepositoryInfo struct {
	Path string
	Name string
}
