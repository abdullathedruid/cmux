// Package notes provides session notes persistence for cmux.
package notes

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

// Store manages session notes persistence.
type Store struct {
	mu       sync.RWMutex
	filePath string
	notes    map[string]string // session name -> note
}

// NewStore creates a new notes store.
func NewStore(filePath string) *Store {
	return &Store{
		filePath: filePath,
		notes:    make(map[string]string),
	}
}

// Load loads notes from the file.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No notes file yet
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &s.notes)
}

// Save saves notes to the file.
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.notes, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// Get returns the note for a session.
func (s *Store) Get(sessionName string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.notes[sessionName]
}

// Set sets the note for a session.
func (s *Store) Set(sessionName, note string) error {
	s.mu.Lock()
	s.notes[sessionName] = note
	s.mu.Unlock()

	return s.Save()
}

// Delete removes the note for a session.
func (s *Store) Delete(sessionName string) error {
	s.mu.Lock()
	delete(s.notes, sessionName)
	s.mu.Unlock()

	return s.Save()
}

// GetAll returns all notes.
func (s *Store) GetAll() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy
	result := make(map[string]string, len(s.notes))
	for k, v := range s.notes {
		result[k] = v
	}
	return result
}

// FirstLine returns the first line of a note for preview.
func FirstLine(note string) string {
	if note == "" {
		return ""
	}
	idx := strings.Index(note, "\n")
	if idx == -1 {
		return note
	}
	return note[:idx]
}

// Cleanup removes notes for sessions that no longer exist.
func (s *Store) Cleanup(existingSessionNames []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build set of existing names
	exists := make(map[string]bool, len(existingSessionNames))
	for _, name := range existingSessionNames {
		exists[name] = true
	}

	// Remove notes for non-existent sessions
	for name := range s.notes {
		if !exists[name] {
			delete(s.notes, name)
		}
	}

	// Save if we removed anything
	data, err := json.MarshalIndent(s.notes, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}
