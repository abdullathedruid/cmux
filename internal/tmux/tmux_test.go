package tmux

import (
	"testing"
	"time"
)

func TestParseSessions(t *testing.T) {
	output := `session1	/home/user/project1	1704067200	0	1
session2	/home/user/project2	1704067300	1	2
`

	sessions := parseSessions(output)

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// First session
	if sessions[0].Name != "session1" {
		t.Errorf("session[0].Name = %q, want 'session1'", sessions[0].Name)
	}
	if sessions[0].Path != "/home/user/project1" {
		t.Errorf("session[0].Path = %q, want '/home/user/project1'", sessions[0].Path)
	}
	if sessions[0].Attached {
		t.Error("session[0] should not be attached")
	}
	if sessions[0].WindowCount != 1 {
		t.Errorf("session[0].WindowCount = %d, want 1", sessions[0].WindowCount)
	}

	// Second session
	if sessions[1].Name != "session2" {
		t.Errorf("session[1].Name = %q, want 'session2'", sessions[1].Name)
	}
	if !sessions[1].Attached {
		t.Error("session[1] should be attached")
	}
	if sessions[1].WindowCount != 2 {
		t.Errorf("session[1].WindowCount = %d, want 2", sessions[1].WindowCount)
	}
}

func TestParseSessionsEmpty(t *testing.T) {
	sessions := parseSessions("")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for empty input, got %d", len(sessions))
	}
}

func TestParseSessionsPartialLine(t *testing.T) {
	// Less than 5 tab-separated fields should be skipped
	output := `incomplete	line
`
	sessions := parseSessions(output)
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for incomplete line, got %d", len(sessions))
	}
}

func TestParseUnixTimestamp(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1704067200", 1704067200},
		{"0", 0},
		{"1234567890", 1234567890},
	}

	for _, tt := range tests {
		got, err := parseUnixTimestamp(tt.input)
		if err != nil {
			t.Errorf("parseUnixTimestamp(%q) error: %v", tt.input, err)
			continue
		}
		if got.Unix() != tt.want {
			t.Errorf("parseUnixTimestamp(%q).Unix() = %d, want %d", tt.input, got.Unix(), tt.want)
		}
	}
}

func TestParseUnixTimestampInvalid(t *testing.T) {
	_, err := parseUnixTimestamp("not-a-number")
	if err == nil {
		t.Error("expected error for invalid timestamp")
	}
}

func TestSessionStruct(t *testing.T) {
	now := time.Now()
	sess := Session{
		Name:        "test-session",
		Path:        "/test/path",
		Created:     now,
		Attached:    true,
		WindowCount: 3,
	}

	if sess.Name != "test-session" {
		t.Errorf("Name = %q, want 'test-session'", sess.Name)
	}
	if sess.Path != "/test/path" {
		t.Errorf("Path = %q, want '/test/path'", sess.Path)
	}
	if !sess.Attached {
		t.Error("expected Attached to be true")
	}
	if sess.WindowCount != 3 {
		t.Errorf("WindowCount = %d, want 3", sess.WindowCount)
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("claude")
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestClientInterface(t *testing.T) {
	// Ensure RealClient implements Client interface
	var _ Client = (*RealClient)(nil)
}

func TestParseSessionsWithDateFormat(t *testing.T) {
	// Test with ISO date format
	output := `session1	/path	2024-01-01T10:00:00	0	1
`
	sessions := parseSessions(output)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// Should have parsed date or fallen back to time.Now()
	if sessions[0].Created.IsZero() {
		t.Error("Created time should not be zero")
	}
}

func TestParseSessionsMultiple(t *testing.T) {
	output := `alpha	/alpha	1704067200	0	1
beta	/beta	1704067300	0	1
gamma	/gamma	1704067400	1	1
delta	/delta	1704067500	0	2
`
	sessions := parseSessions(output)

	if len(sessions) != 4 {
		t.Fatalf("expected 4 sessions, got %d", len(sessions))
	}

	names := []string{"alpha", "beta", "gamma", "delta"}
	for i, want := range names {
		if sessions[i].Name != want {
			t.Errorf("session[%d].Name = %q, want %q", i, sessions[i].Name, want)
		}
	}

	// Only gamma should be attached
	attachedCount := 0
	for _, s := range sessions {
		if s.Attached {
			attachedCount++
			if s.Name != "gamma" {
				t.Errorf("expected 'gamma' to be attached, got %q", s.Name)
			}
		}
	}
	if attachedCount != 1 {
		t.Errorf("expected 1 attached session, got %d", attachedCount)
	}
}

func TestParseVersionSupportsPopup(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		// Versions that support popup (>= 3.2)
		{"tmux 3.2", true},
		{"tmux 3.2a", true},
		{"tmux 3.3", true},
		{"tmux 3.3a", true},
		{"tmux 3.4", true},
		{"tmux 4.0", true},
		{"tmux next-3.4", true},

		// Versions that don't support popup (< 3.2)
		{"tmux 3.1", false},
		{"tmux 3.1c", false},
		{"tmux 3.0", false},
		{"tmux 3.0a", false},
		{"tmux 2.9", false},
		{"tmux 2.9a", false},
		{"tmux 2.8", false},
		{"tmux 1.8", false},

		// Invalid versions
		{"", false},
		{"tmux", false},
		{"invalid", false},
		{"tmux abc", false},
	}

	for _, tt := range tests {
		got := parseVersionSupportsPopup(tt.version)
		if got != tt.want {
			t.Errorf("parseVersionSupportsPopup(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}
