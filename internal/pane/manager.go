package pane

import (
	"sync"
)

// Manager manages a collection of panes with thread-safe operations.
type Manager struct {
	panes     []*Pane
	activeIdx int
	mu        sync.RWMutex
}

// NewManager creates a new empty pane manager.
func NewManager() *Manager {
	return &Manager{
		panes:     make([]*Pane, 0),
		activeIdx: 0,
	}
}

// Add adds a pane to the manager and returns its index.
func (m *Manager) Add(p *Pane) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.panes = append(m.panes, p)
	return len(m.panes) - 1
}

// Remove removes a pane by index and cleans up its resources.
func (m *Manager) Remove(idx int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if idx < 0 || idx >= len(m.panes) {
		return
	}

	// Close the pane's control mode
	m.panes[idx].Close()

	// Remove from slice
	m.panes = append(m.panes[:idx], m.panes[idx+1:]...)

	// Adjust active index if needed
	if m.activeIdx >= len(m.panes) && len(m.panes) > 0 {
		m.activeIdx = len(m.panes) - 1
	}
}

// Get returns the pane at the given index, or nil if out of bounds.
func (m *Manager) Get(idx int) *Pane {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if idx < 0 || idx >= len(m.panes) {
		return nil
	}
	return m.panes[idx]
}

// Active returns the currently active pane, or nil if none.
func (m *Manager) Active() *Pane {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeIdx < 0 || m.activeIdx >= len(m.panes) {
		return nil
	}
	return m.panes[m.activeIdx]
}

// ActiveIndex returns the index of the currently active pane.
func (m *Manager) ActiveIndex() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeIdx
}

// SetActive sets the active pane by index.
func (m *Manager) SetActive(idx int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if idx >= 0 && idx < len(m.panes) {
		m.activeIdx = idx
	}
}

// Count returns the number of panes.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.panes)
}

// All returns a slice of all panes. The returned slice is a copy.
func (m *Manager) All() []*Pane {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Pane, len(m.panes))
	copy(result, m.panes)
	return result
}

// ForEach executes a function for each pane. Thread-safe.
func (m *Manager) ForEach(fn func(idx int, p *Pane)) {
	m.mu.RLock()
	panes := make([]*Pane, len(m.panes))
	copy(panes, m.panes)
	m.mu.RUnlock()

	for i, p := range panes {
		fn(i, p)
	}
}

// Next moves focus to the next pane (wraps around).
func (m *Manager) Next() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.panes) == 0 {
		return
	}
	m.activeIdx = (m.activeIdx + 1) % len(m.panes)
}

// Prev moves focus to the previous pane (wraps around).
func (m *Manager) Prev() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.panes) == 0 {
		return
	}
	m.activeIdx = (m.activeIdx - 1 + len(m.panes)) % len(m.panes)
}

// CloseAll closes all panes and clears the manager.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range m.panes {
		p.Close()
	}
	m.panes = nil
	m.activeIdx = 0
}

// FocusLast sets focus to the last pane (newly added).
func (m *Manager) FocusLast() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.panes) > 0 {
		m.activeIdx = len(m.panes) - 1
	}
}
