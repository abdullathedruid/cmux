package controller

import (
	"testing"
)

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		text    string
		pattern string
		want    bool
	}{
		// Exact matches
		{"hello", "hello", true},
		{"HELLO", "hello", true},
		{"hello", "HELLO", true},

		// Substring matches
		{"hello world", "world", true},
		{"my-project-feature", "project", true},
		{"my-project-feature", "feat", true},

		// Fuzzy character matches
		{"my-project", "mprj", true},
		{"feature-auth", "fauth", true},
		{"hello", "hlo", true},

		// No matches
		{"hello", "xyz", false},
		{"abc", "abcd", false},
		{"short", "longerpattern", false},

		// Empty cases
		{"", "", true},
		{"hello", "", true},
		{"", "hello", false},

		// Case insensitive
		{"MyProject", "myproject", true},
		{"UPPERCASE", "upper", true},
	}

	for _, tt := range tests {
		got := fuzzyMatch(tt.text, tt.pattern)
		if got != tt.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tt.text, tt.pattern, got, tt.want)
		}
	}
}

func TestSanitizeBranch(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"main", "main"},
		{"feature/auth", "feature-auth"},
		{"bugfix-123", "bugfix-123"},
		{"feature/user/login", "feature-user-login"},
		{"special@chars!", "special-chars-"},
		{"under_score", "under_score"},
	}

	for _, tt := range tests {
		got := sanitizeBranch(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeBranch(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
