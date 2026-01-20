// Package config handles application configuration.
package config

import (
	"os"
	"path/filepath"
)

// Config holds application configuration.
type Config struct {
	// DataDir is the directory for persistent data (notes, etc.)
	DataDir string

	// SessionPrefix is prepended to tmux session names (empty by default)
	SessionPrefix string

	// ClaudeCommand is the command to run Claude Code
	ClaudeCommand string

	// DefaultShell is the shell to use when Claude exits
	DefaultShell string

	// WorktreeDir is the subdirectory name for worktrees inside repos
	WorktreeDir string

	// RefreshInterval is how often to refresh session state (in seconds)
	RefreshInterval int
}

// Default returns a Config with default values.
func Default() *Config {
	return &Config{
		DataDir:         defaultDataDir(),
		SessionPrefix:   "",
		ClaudeCommand:   "claude",
		DefaultShell:    getDefaultShell(),
		WorktreeDir:     ".worktrees",
		RefreshInterval: 2,
	}
}

// defaultDataDir returns the default data directory.
func defaultDataDir() string {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "cmux")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cmux"
	}
	return filepath.Join(home, ".config", "cmux")
}

// getDefaultShell returns the user's default shell.
func getDefaultShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/bash"
}

// NotesFile returns the path to the notes file.
func (c *Config) NotesFile() string {
	return filepath.Join(c.DataDir, "notes.json")
}

// EnsureDataDir creates the data directory if it doesn't exist.
func (c *Config) EnsureDataDir() error {
	return os.MkdirAll(c.DataDir, 0755)
}
