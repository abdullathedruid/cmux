// Package git provides git and worktree operations.
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RepoInfo contains information about a git repository.
type RepoInfo struct {
	Root   string // absolute path to repo root
	Name   string // derived name (directory name)
	Branch string // current branch
}

// Worktree represents a git worktree.
type Worktree struct {
	Path   string
	Branch string
	IsMain bool // true if this is the main worktree
}

// FindRepoRoot finds the main git repository root from the given path.
// For worktrees, this returns the main repository root, not the worktree path.
func FindRepoRoot(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("getting absolute path: %w", err)
	}

	// Use --git-common-dir to get the common .git directory
	// This returns the main repo's .git dir even when in a worktree
	cmd := exec.Command("git", "-C", absPath, "rev-parse", "--git-common-dir")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("not a git repository: %s", absPath)
	}

	gitDir := strings.TrimSpace(stdout.String())

	// If gitDir is relative (like ".git"), we need to make it absolute
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(absPath, gitDir)
	}

	// Clean the path to resolve any ".." components
	gitDir = filepath.Clean(gitDir)

	// The repo root is the parent of the .git directory
	repoRoot := filepath.Dir(gitDir)

	return repoRoot, nil
}

// GetRepoInfo returns information about the repository containing the given path.
func GetRepoInfo(path string) (*RepoInfo, error) {
	root, err := FindRepoRoot(path)
	if err != nil {
		return nil, err
	}

	// Get branch from the original path (not root) to get the correct branch for worktrees
	branch, err := GetCurrentBranch(path)
	if err != nil {
		branch = "unknown"
	}

	return &RepoInfo{
		Root:   root,
		Name:   filepath.Base(root),
		Branch: branch,
	}, nil
}

// GetCurrentBranch returns the current branch name for the given path.
func GetCurrentBranch(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "branch", "--show-current")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		// Detached HEAD
		cmd = exec.Command("git", "-C", path, "rev-parse", "--short", "HEAD")
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			return "", err
		}
	}

	branch := strings.TrimSpace(stdout.String())
	if branch == "" {
		return "HEAD", nil
	}
	return branch, nil
}

// ListWorktrees returns all worktrees for the repository at the given path.
func ListWorktrees(repoPath string) ([]Worktree, error) {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git worktree list: %w: %s", err, stderr.String())
	}

	return parseWorktrees(stdout.String()), nil
}

// parseWorktrees parses git worktree list --porcelain output.
func parseWorktrees(output string) []Worktree {
	var worktrees []Worktree
	var current Worktree

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = Worktree{}
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branch := strings.TrimPrefix(line, "branch ")
			// Remove refs/heads/ prefix
			current.Branch = strings.TrimPrefix(branch, "refs/heads/")
		} else if line == "bare" {
			// Skip bare worktrees
			current = Worktree{}
		}
	}

	// Mark the first worktree as main
	if len(worktrees) > 0 {
		worktrees[0].IsMain = true
	}

	return worktrees
}

// CreateWorktree creates a new worktree in the repository's .worktrees directory.
func CreateWorktree(repoPath, branchName string, createBranch bool) (string, error) {
	worktreeDir := filepath.Join(repoPath, ".worktrees")

	// Ensure .worktrees directory exists
	if err := os.MkdirAll(worktreeDir, 0755); err != nil {
		return "", fmt.Errorf("creating worktrees dir: %w", err)
	}

	// Sanitize branch name for directory
	dirName := sanitizeBranchName(branchName)
	worktreePath := filepath.Join(worktreeDir, dirName)

	args := []string{"-C", repoPath, "worktree", "add"}
	if createBranch {
		args = append(args, "-b", branchName)
	}
	args = append(args, worktreePath)
	if !createBranch {
		args = append(args, branchName)
	}

	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git worktree add: %w: %s", err, stderr.String())
	}

	return worktreePath, nil
}

// RemoveWorktree removes a worktree.
func RemoveWorktree(repoPath, worktreePath string) error {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "remove", worktreePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree remove: %w: %s", err, stderr.String())
	}
	return nil
}

// sanitizeBranchName converts a branch name to a safe directory name.
func sanitizeBranchName(branch string) string {
	// Replace / with -
	name := strings.ReplaceAll(branch, "/", "-")
	// Remove any other problematic characters
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, name)
	return name
}

// ShortenPath shortens a path for display by replacing home dir with ~.
func ShortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// IsWorktreePath checks if the given path is inside a .worktrees directory.
func IsWorktreePath(path string) bool {
	return strings.Contains(path, "/.worktrees/")
}

// ExtractWorktreeName extracts the worktree name from a path.
func ExtractWorktreeName(path string) string {
	if !IsWorktreePath(path) {
		return ""
	}

	// Find .worktrees/ and get the next component
	parts := strings.Split(path, "/.worktrees/")
	if len(parts) < 2 {
		return ""
	}

	// Get first component after .worktrees/
	remaining := parts[1]
	if idx := strings.Index(remaining, "/"); idx != -1 {
		return remaining[:idx]
	}
	return remaining
}

// ListBranches returns all local branches for the repository.
func ListBranches(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoPath, "branch", "--format=%(refname:short)")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git branch: %w: %s", err, stderr.String())
	}

	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

// GetMainBranch returns the main branch name for the repository.
// It first tries to get it from the remote HEAD, then falls back to main/master.
func GetMainBranch(repoPath string) string {
	// Try to get the default branch from remote HEAD
	cmd := exec.Command("git", "-C", repoPath, "symbolic-ref", "refs/remotes/origin/HEAD")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err == nil {
		ref := strings.TrimSpace(stdout.String())
		// refs/remotes/origin/main -> main
		if parts := strings.Split(ref, "/"); len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Fallback: check if main or master exists
	branches, err := ListBranches(repoPath)
	if err != nil {
		return "main"
	}

	for _, b := range branches {
		if b == "main" {
			return "main"
		}
	}
	for _, b := range branches {
		if b == "master" {
			return "master"
		}
	}
	return "main"
}

// IsBranchMerged checks if branch is merged to the main branch.
func IsBranchMerged(repoPath, branchName string) (bool, error) {
	mainBranch := GetMainBranch(repoPath)

	cmd := exec.Command("git", "-C", repoPath, "branch", "--merged", mainBranch)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("git branch --merged: %w: %s", err, stderr.String())
	}

	mergedBranches := stdout.String()
	for _, line := range strings.Split(mergedBranches, "\n") {
		// Lines may start with "* " or "  "
		branch := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if branch == branchName {
			return true, nil
		}
	}
	return false, nil
}

// GetLastCommitTime returns the last commit timestamp for a worktree.
func GetLastCommitTime(worktreePath string) (time.Time, error) {
	cmd := exec.Command("git", "-C", worktreePath, "log", "-1", "--format=%ct")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return time.Time{}, fmt.Errorf("git log: %w: %s", err, stderr.String())
	}

	timestamp, err := strconv.ParseInt(strings.TrimSpace(stdout.String()), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing timestamp: %w", err)
	}

	return time.Unix(timestamp, 0), nil
}
