package state

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if !s.IsDashboardView() {
		t.Error("expected dashboard view by default")
	}
	if s.SessionCount() != 0 {
		t.Error("expected 0 sessions initially")
	}
}

func TestUpdateSessions(t *testing.T) {
	s := New()

	sessions := []*Session{
		{Name: "project-main", RepoName: "project", RepoPath: "/code/project"},
		{Name: "project-feature", RepoName: "project", RepoPath: "/code/project"},
		{Name: "other-main", RepoName: "other", RepoPath: "/code/other"},
	}

	s.UpdateSessions(sessions)

	if s.SessionCount() != 3 {
		t.Errorf("expected 3 sessions, got %d", s.SessionCount())
	}

	repos := s.GetRepositories()
	if len(repos) != 2 {
		t.Errorf("expected 2 repositories, got %d", len(repos))
	}
}

func TestSelection(t *testing.T) {
	s := New()

	sessions := []*Session{
		{Name: "a-main", RepoName: "a"},
		{Name: "b-main", RepoName: "b"},
		{Name: "c-main", RepoName: "c"},
	}

	s.UpdateSessions(sessions)
	s.SelectFirst()

	selected := s.GetSelectedSessionName()
	if selected != "a-main" {
		t.Errorf("expected first session 'a-main', got '%s'", selected)
	}

	s.SelectNext()
	selected = s.GetSelectedSessionName()
	if selected != "b-main" {
		t.Errorf("expected 'b-main' after SelectNext, got '%s'", selected)
	}

	s.SelectPrev()
	selected = s.GetSelectedSessionName()
	if selected != "a-main" {
		t.Errorf("expected 'a-main' after SelectPrev, got '%s'", selected)
	}
}

func TestSelectPrevAtStart(t *testing.T) {
	s := New()

	sessions := []*Session{
		{Name: "first", RepoName: "test"},
		{Name: "second", RepoName: "test"},
	}

	s.UpdateSessions(sessions)
	s.SelectFirst()
	s.SelectPrev() // Should stay at first

	selected := s.GetSelectedSessionName()
	if selected != "first" {
		t.Errorf("expected 'first' after SelectPrev at start, got '%s'", selected)
	}
}

func TestSelectNextAtEnd(t *testing.T) {
	s := New()

	sessions := []*Session{
		{Name: "first", RepoName: "test"},
		{Name: "second", RepoName: "test"},
	}

	s.UpdateSessions(sessions)
	s.SetSelectedSession("second")
	s.SelectNext() // Should stay at last

	selected := s.GetSelectedSessionName()
	if selected != "second" {
		t.Errorf("expected 'second' after SelectNext at end, got '%s'", selected)
	}
}

func TestToggleView(t *testing.T) {
	s := New()

	if !s.IsDashboardView() {
		t.Error("expected dashboard view initially")
	}

	s.ToggleView()
	if s.IsDashboardView() {
		t.Error("expected list view after toggle")
	}

	s.ToggleView()
	if !s.IsDashboardView() {
		t.Error("expected dashboard view after second toggle")
	}
}

func TestAttachedCount(t *testing.T) {
	s := New()

	sessions := []*Session{
		{Name: "attached1", Attached: true},
		{Name: "attached2", Attached: true},
		{Name: "idle", Attached: false},
	}

	s.UpdateSessions(sessions)

	if s.AttachedCount() != 2 {
		t.Errorf("expected 2 attached sessions, got %d", s.AttachedCount())
	}
}

func TestUpdateNote(t *testing.T) {
	s := New()

	sessions := []*Session{
		{Name: "test-session", RepoName: "test"},
	}

	s.UpdateSessions(sessions)
	s.UpdateNote("test-session", "Test note content")

	sess := s.GetSession("test-session")
	if sess == nil {
		t.Fatal("session not found")
	}
	if sess.Note != "Test note content" {
		t.Errorf("expected note 'Test note content', got '%s'", sess.Note)
	}
}

func TestSessionStatus(t *testing.T) {
	tests := []struct {
		status SessionStatus
		want   string
	}{
		{StatusIdle, "idle"},
		{StatusActive, "active"},
		{StatusTool, "tool"},
		{StatusThinking, "thinking"},
	}

	for _, tt := range tests {
		got := tt.status.String()
		if got != tt.want {
			t.Errorf("SessionStatus(%d).String() = %s, want %s", tt.status, got, tt.want)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New()

	sessions := []*Session{
		{Name: "concurrent-test", RepoName: "test"},
	}
	s.UpdateSessions(sessions)

	// Simulate concurrent access
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			s.GetSessions()
			s.GetSelectedSession()
			s.SessionCount()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestGetSessionsOrder(t *testing.T) {
	s := New()

	sessions := []*Session{
		{Name: "z-session", RepoName: "z"},
		{Name: "a-session", RepoName: "a"},
		{Name: "m-session", RepoName: "m"},
	}

	s.UpdateSessions(sessions)
	result := s.GetSessions()

	// Should be sorted by repo name
	if result[0].RepoName != "a" {
		t.Errorf("expected first repo 'a', got '%s'", result[0].RepoName)
	}
	if result[1].RepoName != "m" {
		t.Errorf("expected second repo 'm', got '%s'", result[1].RepoName)
	}
	if result[2].RepoName != "z" {
		t.Errorf("expected third repo 'z', got '%s'", result[2].RepoName)
	}
}

func TestSessionWithTimestamps(t *testing.T) {
	now := time.Now()
	sess := &Session{
		Name:       "test",
		Created:    now,
		LastActive: now,
	}

	if sess.Created.IsZero() {
		t.Error("Created timestamp should not be zero")
	}
	if sess.LastActive.IsZero() {
		t.Error("LastActive timestamp should not be zero")
	}
}
