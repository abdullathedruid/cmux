package pane

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// Scrollback manages scrollback state and tmux history capture for a pane.
type Scrollback struct {
	mu         sync.Mutex
	session    string
	scrollPos  int      // 0 = live view, >0 = lines scrolled up from bottom
	cache      []string // Cached scrollback lines
	cacheValid bool     // Whether cache is still valid
}

// NewScrollback creates a new scrollback manager for the given tmux session.
func NewScrollback(session string) *Scrollback {
	return &Scrollback{
		session: session,
	}
}

// ScrollPos returns the current scroll position (0 = live, >0 = scrolled up).
func (s *Scrollback) ScrollPos() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scrollPos
}

// IsScrolled returns true if the view is scrolled (not showing live output).
func (s *Scrollback) IsScrolled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scrollPos > 0
}

// ScrollUp moves the viewport up by the given number of lines.
// Returns the new scroll position.
func (s *Scrollback) ScrollUp(lines int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scrollPos += lines
	s.cacheValid = false
	return s.scrollPos
}

// ScrollDown moves the viewport down by the given number of lines.
// Returns the new scroll position (minimum 0).
func (s *Scrollback) ScrollDown(lines int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scrollPos -= lines
	if s.scrollPos < 0 {
		s.scrollPos = 0
	}
	s.cacheValid = false
	return s.scrollPos
}

// ScrollToBottom resets scroll position to show live output.
func (s *Scrollback) ScrollToBottom() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scrollPos = 0
	s.cacheValid = false
}

// InvalidateCache marks the cache as invalid (call when new output arrives).
func (s *Scrollback) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheValid = false
}

// CaptureHistory fetches scrollback history from tmux for the given viewport.
// height is the number of visible lines in the pane.
// Returns the lines to display and any error.
func (s *Scrollback) CaptureHistory(height int) ([]string, error) {
	s.mu.Lock()
	scrollPos := s.scrollPos
	session := s.session
	cacheValid := s.cacheValid
	cache := s.cache
	s.mu.Unlock()

	// Return cached content if still valid
	if cacheValid && len(cache) > 0 {
		return cache, nil
	}

	// Calculate line range
	// scrollPos=10, height=24 means show lines -34 to -10 (relative to visible bottom)
	// -S (start) = -(scrollPos + height)
	// -E (end) = -scrollPos - 1 (or -1 if scrollPos is small)
	startLine := -(scrollPos + height)
	endLine := -scrollPos
	if endLine == 0 {
		endLine = -1 // -E 0 would be first visible line, we want history only
	}

	lines, err := capturePane(session, startLine, endLine)
	if err != nil {
		return nil, err
	}

	// Update cache
	s.mu.Lock()
	s.cache = lines
	s.cacheValid = true
	s.mu.Unlock()

	return lines, nil
}

// capturePane runs tmux capture-pane and returns the output lines.
func capturePane(session string, startLine, endLine int) ([]string, error) {
	cmd := exec.Command("tmux", "capture-pane",
		"-t", session,
		"-p",  // Print to stdout
		"-J",  // Join wrapped lines
		"-S", strconv.Itoa(startLine),
		"-E", strconv.Itoa(endLine),
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Split into lines, preserving empty lines for proper rendering
	content := string(output)
	if content == "" {
		return []string{}, nil
	}

	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	return lines, nil
}
