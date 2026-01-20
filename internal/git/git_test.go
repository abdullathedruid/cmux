package git

import (
	"strings"
	"testing"
)

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"feature/auth", "feature-auth"},
		{"bugfix-123", "bugfix-123"},
		{"main", "main"},
		{"feature/user/login", "feature-user-login"},
		{"feat!special@chars", "feat-special-chars"},
		{"under_score", "under_score"},
		{"CamelCase", "CamelCase"},
	}

	for _, tt := range tests {
		got := sanitizeBranchName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeBranchName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestShortenPath(t *testing.T) {
	// This test depends on the HOME environment variable
	path := "/some/absolute/path"
	result := ShortenPath(path)

	// Should not panic and return something
	if result == "" {
		t.Error("ShortenPath returned empty string")
	}
}

func TestIsWorktreePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/repo/.worktrees/feature", true},
		{"/repo/.worktrees/feature/src", true},
		{"/repo/src", false},
		{"/repo/.git", false},
		{"/home/user/.worktrees/test", true},
	}

	for _, tt := range tests {
		got := IsWorktreePath(tt.path)
		if got != tt.want {
			t.Errorf("IsWorktreePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestExtractWorktreeName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/repo/.worktrees/feature", "feature"},
		{"/repo/.worktrees/feature-auth/src", "feature-auth"},
		{"/repo/src", ""},
		{"/repo/.worktrees/", ""},
		{"/repo/.worktrees/test", "test"},
	}

	for _, tt := range tests {
		got := ExtractWorktreeName(tt.path)
		if got != tt.want {
			t.Errorf("ExtractWorktreeName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestParseWorktrees(t *testing.T) {
	output := `worktree /Users/test/project
branch refs/heads/main

worktree /Users/test/project/.worktrees/feature
branch refs/heads/feature/auth

`

	worktrees := parseWorktrees(output)

	if len(worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(worktrees))
	}

	// First worktree should be marked as main
	if !worktrees[0].IsMain {
		t.Error("first worktree should be marked as main")
	}
	if worktrees[0].Branch != "main" {
		t.Errorf("expected branch 'main', got '%s'", worktrees[0].Branch)
	}

	// Second worktree
	if worktrees[1].IsMain {
		t.Error("second worktree should not be marked as main")
	}
	if worktrees[1].Branch != "feature/auth" {
		t.Errorf("expected branch 'feature/auth', got '%s'", worktrees[1].Branch)
	}
}

func TestParseWorktreesEmpty(t *testing.T) {
	worktrees := parseWorktrees("")
	if len(worktrees) != 0 {
		t.Errorf("expected 0 worktrees for empty input, got %d", len(worktrees))
	}
}

func TestParseWorktreesBare(t *testing.T) {
	output := `worktree /Users/test/project.git
bare

worktree /Users/test/project
branch refs/heads/main

`

	worktrees := parseWorktrees(output)

	// Bare worktree should be skipped
	if len(worktrees) != 1 {
		t.Fatalf("expected 1 worktree (bare skipped), got %d", len(worktrees))
	}
}

func TestWorktreeStruct(t *testing.T) {
	wt := Worktree{
		Path:   "/test/path",
		Branch: "main",
		IsMain: true,
	}

	if wt.Path != "/test/path" {
		t.Errorf("expected path '/test/path', got '%s'", wt.Path)
	}
	if wt.Branch != "main" {
		t.Errorf("expected branch 'main', got '%s'", wt.Branch)
	}
	if !wt.IsMain {
		t.Error("expected IsMain to be true")
	}
}

func TestRepoInfoStruct(t *testing.T) {
	info := &RepoInfo{
		Root:   "/test/repo",
		Name:   "repo",
		Branch: "main",
	}

	if info.Root != "/test/repo" {
		t.Errorf("expected root '/test/repo', got '%s'", info.Root)
	}
	if info.Name != "repo" {
		t.Errorf("expected name 'repo', got '%s'", info.Name)
	}
}

func TestShortenPathWithHome(t *testing.T) {
	// Test that paths with home directory get shortened
	// This is a behavioral test - actual result depends on HOME env var
	path := ShortenPath("/absolute/path/no/home")
	if strings.HasPrefix(path, "~") && !strings.Contains("/absolute/path/no/home", "~") {
		t.Error("path without home should not start with ~")
	}
}
