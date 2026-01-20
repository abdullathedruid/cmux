package controller

import (
	"testing"
)

func TestSanitizeBranchForSession(t *testing.T) {
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
		{"CamelCase", "CamelCase"},
		{"with spaces", "with-spaces"},
	}

	for _, tt := range tests {
		got := sanitizeBranchForSession(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeBranchForSession(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
