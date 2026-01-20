// Package state manages application state for cmux.
package state

import (
	"sort"
	"sync"
	"time"
)

// SessionStatus represents the current status of a Claude session.
type SessionStatus int

const (
	StatusIdle SessionStatus = iota
	StatusActive
	StatusTool
	StatusThinking
	StatusNeedsInput
)

func (s SessionStatus) String() string {
	switch s {
	case StatusActive:
		return "active"
	case StatusTool:
		return "tool"
	case StatusThinking:
		return "thinking"
	case StatusNeedsInput:
		return "needs_input"
	default:
		return "idle"
	}
}

// ToolHistoryEntry represents a single tool execution in the history.
type ToolHistoryEntry struct {
	Tool      string
	Summary   string
	Result    string
	Timestamp time.Time
}

// Session represents a Claude session managed by cmux.
type Session struct {
	Name        string        // tmux session name (e.g., "myproject-feature-auth")
	RepoPath    string        // git repo root (empty if standalone)
	RepoName    string        // derived repo name
	Worktree    string        // worktree path (may equal RepoPath)
	Branch      string        // current branch
	Attached    bool          // currently attached
	Status      SessionStatus // idle, active, tool, thinking, needs_input
	CurrentTool string        // current tool being used
	ToolSummary string        // one-line summary of what the tool is doing
	Created     time.Time
	LastActive  time.Time
	Note        string

	// Extended status from transcript
	SessionID   string             // Claude session ID
	LastPrompt  string             // last user prompt submitted
	ToolHistory []ToolHistoryEntry // recent tool execution history
}

// Repository represents a git repository with associated sessions.
type Repository struct {
	Path     string
	Name     string
	Sessions []*Session
}

// State holds the application state.
type State struct {
	mu sync.RWMutex

	sessions     map[string]*Session // keyed by session name
	repositories map[string]*Repository // keyed by repo path

	// Selection state
	selectedSession string
	selectedRepo    string

	// View state
	dashboardView bool // true = dashboard, false = list
}

// New creates a new State.
func New() *State {
	return &State{
		sessions:      make(map[string]*Session),
		repositories:  make(map[string]*Repository),
		dashboardView: true,
	}
}

// UpdateSessions replaces all sessions with the provided list.
func (s *State) UpdateSessions(sessions []*Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear existing
	s.sessions = make(map[string]*Session)
	s.repositories = make(map[string]*Repository)

	// Add new sessions
	for _, sess := range sessions {
		s.sessions[sess.Name] = sess
		s.addToRepo(sess)
	}

	// Validate selection
	if _, ok := s.sessions[s.selectedSession]; !ok {
		s.selectedSession = ""
	}
}

// addToRepo adds a session to its repository (must be called with lock held).
func (s *State) addToRepo(sess *Session) {
	repoPath := sess.RepoPath
	if repoPath == "" {
		repoPath = "standalone"
	}

	repo, ok := s.repositories[repoPath]
	if !ok {
		repo = &Repository{
			Path: repoPath,
			Name: sess.RepoName,
		}
		if repo.Name == "" {
			repo.Name = "standalone"
		}
		s.repositories[repoPath] = repo
	}
	repo.Sessions = append(repo.Sessions, sess)
}

// GetSessions returns all sessions.
func (s *State) GetSessions() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}

	// Sort by repo, then created time
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].RepoName != sessions[j].RepoName {
			return sessions[i].RepoName < sessions[j].RepoName
		}
		return sessions[i].Created.Before(sessions[j].Created)
	})

	return sessions
}

// GetSession returns a specific session by name.
func (s *State) GetSession(name string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[name]
}

// GetRepositories returns all repositories with their sessions.
func (s *State) GetRepositories() []*Repository {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repos := make([]*Repository, 0, len(s.repositories))
	for _, repo := range s.repositories {
		repos = append(repos, repo)
	}

	// Sort by name
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Name < repos[j].Name
	})

	return repos
}

// GetSelectedSession returns the currently selected session.
func (s *State) GetSelectedSession() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[s.selectedSession]
}

// GetSelectedSessionName returns the name of the currently selected session.
func (s *State) GetSelectedSessionName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selectedSession
}

// SetSelectedSession sets the selected session by name.
func (s *State) SetSelectedSession(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selectedSession = name
}

// SelectFirst selects the first session if none selected.
func (s *State) SelectFirst() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.selectedSession != "" {
		if _, ok := s.sessions[s.selectedSession]; ok {
			return
		}
	}

	// Find first session
	sessions := s.getSortedSessionsLocked()
	if len(sessions) > 0 {
		s.selectedSession = sessions[0].Name
	}
}

// SelectNext moves selection to the next session.
func (s *State) SelectNext() {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := s.getSortedSessionsLocked()
	if len(sessions) == 0 {
		return
	}

	idx := s.findSessionIndex(sessions, s.selectedSession)
	if idx < len(sessions)-1 {
		s.selectedSession = sessions[idx+1].Name
	}
}

// SelectPrev moves selection to the previous session.
func (s *State) SelectPrev() {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := s.getSortedSessionsLocked()
	if len(sessions) == 0 {
		return
	}

	idx := s.findSessionIndex(sessions, s.selectedSession)
	if idx > 0 {
		s.selectedSession = sessions[idx-1].Name
	}
}

// getSortedSessionsLocked returns sessions sorted by repo then created time (must be called with lock held).
func (s *State) getSortedSessionsLocked() []*Session {
	sessions := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].RepoName != sessions[j].RepoName {
			return sessions[i].RepoName < sessions[j].RepoName
		}
		return sessions[i].Created.Before(sessions[j].Created)
	})

	return sessions
}

// findSessionIndex finds the index of a session by name.
func (s *State) findSessionIndex(sessions []*Session, name string) int {
	for i, sess := range sessions {
		if sess.Name == name {
			return i
		}
	}
	return 0
}

// IsDashboardView returns true if showing dashboard view.
func (s *State) IsDashboardView() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dashboardView
}

// SetDashboardView sets whether to show dashboard view.
func (s *State) SetDashboardView(dashboard bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dashboardView = dashboard
}

// ToggleView toggles between dashboard and list view.
func (s *State) ToggleView() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dashboardView = !s.dashboardView
}

// SessionCount returns the total number of sessions.
func (s *State) SessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// AttachedCount returns the number of attached sessions.
func (s *State) AttachedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, sess := range s.sessions {
		if sess.Attached {
			count++
		}
	}
	return count
}

// ActiveCount returns the number of non-attached sessions that are active (not idle).
func (s *State) ActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, sess := range s.sessions {
		if !sess.Attached && sess.Status != StatusIdle {
			count++
		}
	}
	return count
}

// UpdateNote updates a session's note.
func (s *State) UpdateNote(name, note string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[name]; ok {
		sess.Note = note
	}
}
