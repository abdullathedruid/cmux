package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("Default() returned nil")
	}

	if cfg.ClaudeCommand != "claude" {
		t.Errorf("ClaudeCommand = %q, want 'claude'", cfg.ClaudeCommand)
	}

	if cfg.WorktreeDir != ".worktrees" {
		t.Errorf("WorktreeDir = %q, want '.worktrees'", cfg.WorktreeDir)
	}

	if cfg.RefreshInterval != 2 {
		t.Errorf("RefreshInterval = %d, want 2", cfg.RefreshInterval)
	}

	if cfg.SessionPrefix != "" {
		t.Errorf("SessionPrefix = %q, want empty string", cfg.SessionPrefix)
	}
}

func TestDefaultDataDir(t *testing.T) {
	// Save and restore XDG_CONFIG_HOME
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	// Test with XDG_CONFIG_HOME set
	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	dir := defaultDataDir()
	if dir != "/custom/config/cmux" {
		t.Errorf("with XDG_CONFIG_HOME: got %q, want '/custom/config/cmux'", dir)
	}

	// Test without XDG_CONFIG_HOME
	os.Unsetenv("XDG_CONFIG_HOME")
	dir = defaultDataDir()
	if !strings.HasSuffix(dir, ".config/cmux") {
		t.Errorf("without XDG_CONFIG_HOME: got %q, expected to end with '.config/cmux'", dir)
	}
}

func TestGetDefaultShell(t *testing.T) {
	shell := getDefaultShell()
	if shell == "" {
		t.Error("getDefaultShell() returned empty string")
	}
	// Should be a valid path or contain 'sh'
	if !strings.Contains(shell, "sh") && shell != "" {
		t.Logf("shell = %q (might be fine)", shell)
	}
}

func TestNotesFile(t *testing.T) {
	cfg := &Config{
		DataDir: "/test/data",
	}

	notesFile := cfg.NotesFile()
	expected := "/test/data/notes.json"
	if notesFile != expected {
		t.Errorf("NotesFile() = %q, want %q", notesFile, expected)
	}
}

func TestEnsureDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "cmux-test", "data")

	cfg := &Config{
		DataDir: dataDir,
	}

	if err := cfg.EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir() error: %v", err)
	}

	// Directory should exist
	info, err := os.Stat(dataDir)
	if err != nil {
		t.Fatalf("data dir does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("data dir is not a directory")
	}

	// Should be idempotent
	if err := cfg.EnsureDataDir(); err != nil {
		t.Errorf("second EnsureDataDir() error: %v", err)
	}
}

func TestConfigFields(t *testing.T) {
	cfg := &Config{
		DataDir:         "/data",
		SessionPrefix:   "test-",
		ClaudeCommand:   "claude-test",
		DefaultShell:    "/bin/zsh",
		WorktreeDir:     ".wt",
		RefreshInterval: 5,
	}

	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %q, want '/data'", cfg.DataDir)
	}
	if cfg.SessionPrefix != "test-" {
		t.Errorf("SessionPrefix = %q, want 'test-'", cfg.SessionPrefix)
	}
	if cfg.ClaudeCommand != "claude-test" {
		t.Errorf("ClaudeCommand = %q, want 'claude-test'", cfg.ClaudeCommand)
	}
	if cfg.DefaultShell != "/bin/zsh" {
		t.Errorf("DefaultShell = %q, want '/bin/zsh'", cfg.DefaultShell)
	}
	if cfg.WorktreeDir != ".wt" {
		t.Errorf("WorktreeDir = %q, want '.wt'", cfg.WorktreeDir)
	}
	if cfg.RefreshInterval != 5 {
		t.Errorf("RefreshInterval = %d, want 5", cfg.RefreshInterval)
	}
}

func TestDefaultShellWithEnv(t *testing.T) {
	// Save and restore SHELL
	oldShell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", oldShell)

	// Test with SHELL set
	os.Setenv("SHELL", "/bin/custom-shell")
	shell := getDefaultShell()
	if shell != "/bin/custom-shell" {
		t.Errorf("with SHELL env: got %q, want '/bin/custom-shell'", shell)
	}

	// Test without SHELL
	os.Unsetenv("SHELL")
	shell = getDefaultShell()
	if shell != "/bin/bash" {
		t.Errorf("without SHELL env: got %q, want '/bin/bash'", shell)
	}
}

func TestValidateRepositories(t *testing.T) {
	// Create temp directories for testing
	tmpDir := t.TempDir()
	existingRepo := filepath.Join(tmpDir, "existing-repo")
	if err := os.MkdirAll(existingRepo, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	tests := []struct {
		name    string
		repos   []string
		wantErr bool
	}{
		{
			name:    "empty list",
			repos:   []string{},
			wantErr: false,
		},
		{
			name:    "existing path",
			repos:   []string{existingRepo},
			wantErr: false,
		},
		{
			name:    "non-existing path",
			repos:   []string{"/path/that/does/not/exist"},
			wantErr: true,
		},
		{
			name:    "mixed paths",
			repos:   []string{existingRepo, "/path/that/does/not/exist"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRepositories(tt.repos)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRepositories() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"~/test", filepath.Join(home, "test")},
		{"~/", home},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := expandPath(tt.input)
			if result != tt.expected {
				t.Errorf("expandPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExpandedRepositories(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	cfg := &Config{
		Repositories: []string{
			"~/Coding/project1",
			"/absolute/path",
			"~/another/repo",
		},
	}

	expanded := cfg.ExpandedRepositories()

	if len(expanded) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(expanded))
	}

	expectedPaths := []string{
		filepath.Join(home, "Coding/project1"),
		"/absolute/path",
		filepath.Join(home, "another/repo"),
	}

	for i, expected := range expectedPaths {
		if expanded[i] != expected {
			t.Errorf("expanded[%d] = %q, want %q", i, expanded[i], expected)
		}
	}
}
