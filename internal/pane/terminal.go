// Package pane provides pane management with terminal emulation for tmux control mode.
package pane

import (
	"strings"
	"sync"

	"github.com/vito/midterm"
)

// SafeTerminal wraps midterm.Terminal with a mutex for thread-safe access.
// All reads and writes to the terminal must go through this wrapper.
type SafeTerminal struct {
	*midterm.Terminal
	mu sync.Mutex
}

// NewSafeTerminal creates a new thread-safe terminal with the given dimensions.
func NewSafeTerminal(rows, cols int) *SafeTerminal {
	return &SafeTerminal{
		Terminal: midterm.NewTerminal(rows, cols),
	}
}

// Write writes data to the terminal buffer. Thread-safe.
func (t *SafeTerminal) Write(data []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Terminal.Write(data)
}

// Resize changes the terminal dimensions. Thread-safe.
func (t *SafeTerminal) Resize(rows, cols int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Terminal.Resize(rows, cols)
}

// Render writes the terminal content to a strings.Builder. Thread-safe.
func (t *SafeTerminal) Render(w *strings.Builder) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Height <= 0 || t.Width <= 0 {
		return nil
	}
	return t.Terminal.Render(w)
}

// Cursor returns the current cursor position. Thread-safe.
func (t *SafeTerminal) Cursor() (x, y int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Terminal.Cursor.X, t.Terminal.Cursor.Y
}

// Dimensions returns the terminal size. Thread-safe.
func (t *SafeTerminal) Dimensions() (rows, cols int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Height, t.Width
}

// CursorVisible returns whether the cursor should be visible. Thread-safe.
// Applications like Claude hide the cursor while working.
func (t *SafeTerminal) CursorVisible() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Terminal.CursorVisible
}
