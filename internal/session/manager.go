// Package session provides session lifecycle management with worktree coupling.
package session

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/abdullathedruid/cmux/internal/config"
	"github.com/abdullathedruid/cmux/internal/git"
	"github.com/abdullathedruid/cmux/internal/tmux"
)

// Manager handles session lifecycle with worktree coupling.
type Manager struct {
	tmux   *tmux.RealClient
	config *config.Config
}

// NewManager creates a new session manager.
func NewManager(t *tmux.RealClient, cfg *config.Config) *Manager {
	return &Manager{
		tmux:   t,
		config: cfg,
	}
}

// CreateSession creates a new tmux session coupled with a git worktree.
// Session naming: {repoName}/{branchName}
// For main branch sessions, uses the repo root directory.
// For other branches, creates/uses a worktree in .worktrees/.
func (m *Manager) CreateSession(repoPath, branchName string, newBranch bool) (string, error) {
	// Get repository info
	repoInfo, err := git.GetRepoInfo(repoPath)
	if err != nil {
		return "", fmt.Errorf("getting repo info: %w", err)
	}

	// Generate session name: repoName/branchName
	sessionName := GenerateSessionName(repoInfo.Name, branchName)

	// Check if session already exists
	if m.tmux.HasSession(sessionName) {
		return sessionName, nil // Already exists
	}

	// Determine working directory
	var workDir string
	mainBranch := git.GetMainBranch(repoPath)

	if branchName == mainBranch {
		// Main branch: use repo root
		workDir = repoPath
	} else {
		// Other branch: check/create worktree
		worktrees, err := git.ListWorktrees(repoPath)
		if err != nil {
			return "", fmt.Errorf("listing worktrees: %w", err)
		}

		// Check if worktree already exists for this branch
		for _, wt := range worktrees {
			if wt.Branch == branchName {
				workDir = wt.Path
				break
			}
		}

		// Create worktree if needed
		if workDir == "" {
			wtPath, err := git.CreateWorktree(repoPath, branchName, newBranch)
			if err != nil {
				return "", fmt.Errorf("creating worktree: %w", err)
			}
			workDir = wtPath
		}
	}

	// Create tmux session
	if err := m.tmux.CreateSession(sessionName, workDir, true); err != nil {
		return "", fmt.Errorf("creating tmux session: %w", err)
	}

	return sessionName, nil
}

// DeleteSession kills a tmux session and optionally removes its worktree.
func (m *Manager) DeleteSession(sessionName string, removeWorktree bool) error {
	// Parse repo and branch from session name
	repoName, branchName := ParseSessionName(sessionName)

	// Kill the tmux session first
	if m.tmux.HasSession(sessionName) {
		if err := m.tmux.KillSession(sessionName); err != nil {
			return fmt.Errorf("killing session: %w", err)
		}
	}

	if !removeWorktree || repoName == "" || branchName == "" {
		return nil
	}

	// Find the repository path from config
	repoPath := m.findRepoPath(repoName)
	if repoPath == "" {
		return nil // Can't find repo, skip worktree removal
	}

	// Check if this is the main branch - don't remove main repo
	mainBranch := git.GetMainBranch(repoPath)
	if branchName == mainBranch {
		return nil // Don't remove main branch worktree (it's the repo itself)
	}

	// Find and remove the worktree
	worktrees, err := git.ListWorktrees(repoPath)
	if err != nil {
		return nil // Ignore errors, session is already killed
	}

	for _, wt := range worktrees {
		if wt.Branch == branchName && !wt.IsMain {
			if err := git.RemoveWorktree(repoPath, wt.Path); err != nil {
				return fmt.Errorf("removing worktree: %w", err)
			}
			break
		}
	}

	return nil
}

// GetSessionInfo returns information about a session based on its name.
func (m *Manager) GetSessionInfo(sessionName string) (*SessionInfo, error) {
	repoName, branchName := ParseSessionName(sessionName)

	info := &SessionInfo{
		SessionName: sessionName,
		RepoName:    repoName,
		BranchName:  branchName,
	}

	// Try to find repo path
	repoPath := m.findRepoPath(repoName)
	if repoPath != "" {
		info.RepoPath = repoPath

		// Check if it has a worktree
		mainBranch := git.GetMainBranch(repoPath)
		info.IsMainBranch = branchName == mainBranch

		if !info.IsMainBranch {
			worktrees, _ := git.ListWorktrees(repoPath)
			for _, wt := range worktrees {
				if wt.Branch == branchName {
					info.WorktreePath = wt.Path
					info.HasWorktree = true
					break
				}
			}
		}
	}

	return info, nil
}

// findRepoPath finds the full repository path from a repo name.
func (m *Manager) findRepoPath(repoName string) string {
	for _, repo := range m.config.ExpandedRepositories() {
		if filepath.Base(repo) == repoName {
			absPath, err := filepath.Abs(repo)
			if err != nil {
				continue
			}
			return absPath
		}
	}
	return ""
}

// GenerateSessionName creates a session name from repo name and branch.
// Format: repoName/branchName
func GenerateSessionName(repoName, branchName string) string {
	// Sanitize branch name for tmux (replace / with -)
	sanitizedBranch := strings.ReplaceAll(branchName, "/", "-")
	return fmt.Sprintf("%s/%s", repoName, sanitizedBranch)
}

// ParseSessionName extracts repo name and branch from a session name.
// Expected format: repoName/branchName
func ParseSessionName(sessionName string) (repoName, branchName string) {
	parts := strings.SplitN(sessionName, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	// If no /, return the whole name as repo name
	return sessionName, ""
}

// SessionInfo contains information about a session.
type SessionInfo struct {
	SessionName  string
	RepoName     string
	RepoPath     string
	BranchName   string
	IsMainBranch bool
	HasWorktree  bool
	WorktreePath string
}
